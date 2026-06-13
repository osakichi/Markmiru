<script lang="ts">
  import { onMount } from 'svelte'
  import { selectAll, undo, redo, undoDepth, redoDepth } from '@codemirror/commands'
  import { openSearchPanel } from '@codemirror/search'
  import { tabsStore } from '../stores/tabs.svelte'
  import { getEditorView } from '../editorBridge'
  import { viewFind } from '../viewFind.svelte'
  import { clipboardGetText, clipboardSetText } from '../api/wails'

  // 両モード（閲覧 .markdown-body / 編集 .cm-editor）の右クリックメニュー。
  // 既定の WebView コンテキストメニューは無効のため、最小限の標準項目を自前で出す。
  // クリップボードは OS（Go runtime）経由で扱い、ブラウザ API の制限を回避する。
  interface MenuItem {
    label: string
    shortcut?: string
    disabled?: boolean
    action: () => void
  }

  let open = $state(false)
  let x = $state(0)
  let y = $state(0)
  let items = $state<MenuItem[]>([])
  let menuEl = $state<HTMLDivElement | undefined>(undefined)

  function close(): void {
    open = false
  }

  async function copyText(text: string): Promise<void> {
    if (text) await clipboardSetText(text)
  }

  // 選択中の全範囲（複数選択を含む）を CodeMirror ネイティブと同様に改行で連結して返す。
  function editorSelectionText(): string {
    const view = getEditorView()
    if (!view) return ''
    const { state } = view
    return state.selection.ranges
      .filter((r) => !r.empty)
      .map((r) => state.sliceDoc(r.from, r.to))
      .join(state.lineBreak)
  }

  async function pasteIntoEditor(): Promise<void> {
    const view = getEditorView()
    if (!view) return
    const text = await clipboardGetText()
    if (text) view.dispatch(view.state.replaceSelection(text))
    view.focus()
  }

  async function cutFromEditor(text: string): Promise<void> {
    const view = getEditorView()
    if (!view) return
    if (text) {
      await clipboardSetText(text)
      view.dispatch(view.state.replaceSelection(''))
    }
    view.focus()
  }

  function selectAllEditor(): void {
    const view = getEditorView()
    if (!view) return
    selectAll(view) // CodeMirror 組み込みコマンド
    view.focus()
  }

  function searchEditor(): void {
    const view = getEditorView()
    if (!view) return
    openSearchPanel(view) // CodeMirror の検索パネル（Ctrl+F と同じ）
  }

  function undoEditor(): void {
    const view = getEditorView()
    if (!view) return
    undo(view)
    view.focus()
  }

  function redoEditor(): void {
    const view = getEditorView()
    if (!view) return
    redo(view)
    view.focus()
  }

  function selectAllPreview(): void {
    const el = document.querySelector('.markdown-body')
    if (!el) return
    const range = document.createRange()
    range.selectNodeContents(el)
    const sel = window.getSelection()
    sel?.removeAllRanges()
    sel?.addRange(range)
  }

  function buildItems(e: MouseEvent): MenuItem[] {
    const tab = tabsStore.active
    if (!tab) return []
    if (tab.mode === 'source') {
      // 編集モード（CodeMirror）。項目は固定で、可否のみ状況に応じて変える。
      const view = getEditorView()
      const sel = editorSelectionText()
      const canUndo = view ? undoDepth(view.state) > 0 : false
      const canRedo = view ? redoDepth(view.state) > 0 : false
      return [
        { label: '取り消し', shortcut: 'Ctrl+Z', disabled: !canUndo, action: () => undoEditor() },
        { label: '再実行', shortcut: 'Ctrl+Y', disabled: !canRedo, action: () => redoEditor() },
        { label: '切り取り', shortcut: 'Ctrl+X', disabled: !sel, action: () => void cutFromEditor(sel) },
        { label: 'コピー', shortcut: 'Ctrl+C', disabled: !sel, action: () => void copyText(sel) },
        { label: '貼り付け', shortcut: 'Ctrl+V', action: () => void pasteIntoEditor() },
        { label: 'すべて選択', shortcut: 'Ctrl+A', action: () => selectAllEditor() },
        { label: '検索…', shortcut: 'Ctrl+F', action: () => searchEditor() }
      ]
    }
    // 閲覧モード（貼り付けなし）。項目は常に固定で、リンク以外では「リンクのコピー」を無効にする。
    const target = e.target as Element | null
    const anchor = target?.closest?.('a[href]') as HTMLAnchorElement | null
    const href = anchor?.getAttribute('href') ?? ''
    const sel = window.getSelection()?.toString() ?? ''
    return [
      { label: 'コピー', shortcut: 'Ctrl+C', disabled: !sel, action: () => void copyText(sel) },
      { label: 'リンクのコピー', disabled: !anchor, action: () => void copyText(href) },
      { label: 'すべて選択', shortcut: 'Ctrl+A', action: () => selectAllPreview() },
      { label: '検索…', shortcut: 'Ctrl+F', action: () => viewFind.show() }
    ]
  }

  function onContextMenu(e: MouseEvent): void {
    // コンテンツ領域（閲覧/編集）でのみ自前メニューを出す。ツールバー・ダイアログ等では出さない。
    const target = e.target as Element | null
    if (!target?.closest?.('.markdown-body, .cm-editor')) {
      close()
      return
    }
    const built = buildItems(e)
    if (built.length === 0) return
    e.preventDefault()
    items = built
    x = e.clientX
    y = e.clientY
    open = true
    // 表示後に画面外へはみ出さないよう位置を補正
    queueMicrotask(clampPosition)
  }

  function clampPosition(): void {
    const el = menuEl
    if (!el) return
    const rect = el.getBoundingClientRect()
    if (x + rect.width > window.innerWidth) x = Math.max(0, window.innerWidth - rect.width - 4)
    if (y + rect.height > window.innerHeight) y = Math.max(0, window.innerHeight - rect.height - 4)
  }

  function onItem(item: MenuItem): void {
    if (item.disabled) return
    close()
    item.action()
  }

  function onKeydown(e: KeyboardEvent): void {
    if (open && e.key === 'Escape') {
      e.preventDefault()
      close()
    }
  }

  // メニュー外をクリックしたら閉じる（右クリックでの開き直しは onContextMenu が処理する）。
  function onPointerDown(e: PointerEvent): void {
    if (!open) return
    const target = e.target as Node | null
    if (menuEl && target && !menuEl.contains(target)) close()
  }

  onMount(() => {
    window.addEventListener('contextmenu', onContextMenu)
    return () => window.removeEventListener('contextmenu', onContextMenu)
  })
</script>

<svelte:window onkeydown={onKeydown} onpointerdown={onPointerDown} onresize={close} onblur={close} />

{#if open}
  <div class="cm-menu" bind:this={menuEl} style="left:{x}px; top:{y}px" role="menu">
    {#each items as item (item.label)}
      <button class="cm-item" role="menuitem" disabled={item.disabled} onclick={() => onItem(item)}>
        <span class="cm-label">{item.label}</span>
        {#if item.shortcut}<span class="cm-shortcut">{item.shortcut}</span>{/if}
      </button>
    {/each}
  </div>
{/if}

<style>
  .cm-menu {
    position: fixed;
    z-index: 1200;
    min-width: 160px;
    padding: 0.25rem;
    background: #ffffff;
    border: 1px solid #d0d7de;
    border-radius: 6px;
    box-shadow: 0 6px 20px rgba(0, 0, 0, 0.18);
  }
  .cm-item {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 1.5rem;
    width: 100%;
    text-align: left;
    border: none;
    background: transparent;
    color: #24292f;
    padding: 0.32rem 0.7rem;
    font-size: 0.85rem;
    border-radius: 4px;
    cursor: pointer;
  }
  .cm-item:hover:not(:disabled) {
    background: #eef1f4;
  }
  .cm-item:disabled {
    color: #8b949e;
    cursor: default;
  }
  .cm-shortcut {
    color: #8b949e;
    font-size: 0.78rem;
    white-space: nowrap;
  }
</style>
