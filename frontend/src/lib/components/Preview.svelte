<script lang="ts">
  import { tick, onMount } from 'svelte'
  import { renderMarkdown, runMermaid, resolveLocalImages } from '../markdown/renderer'
  import { styleStore } from '../style/style.svelte'
  import { styleToVars, varsToStyleString } from '../style/styleDef'
  import { tabsStore } from '../stores/tabs.svelte'
  import { uiStore } from '../stores/ui.svelte'
  import { riskyDialog } from '../riskyDialog.svelte'
  import { viewFind } from '../viewFind.svelte'
  import '../styles/markdown.css'

  // 閲覧モードのプレビュー。source（Markdown 文字列）を描画する。
  // fileDir: 開いているファイルの親ディレクトリ（相対パス画像の解決に使用）。空なら画像解決スキップ。
  // remoteImagePolicy: リモート画像の表示可否（未確認なら遮断してレンダリングし、初回に確認する）。
  let {
    source = '',
    tabId = '',
    fileName = '',
    fileDir = '',
    remoteImagePolicy
  }: {
    source?: string
    tabId?: string
    fileName?: string
    fileDir?: string
    remoteImagePolicy?: 'allow' | 'block'
  } = $props()

  let container = $state<HTMLElement | undefined>(undefined)
  let html = $state('')

  // 同一タブで確認ダイアログが多重に開かないようにするガード。
  let confirmingId: string | null = null

  // アクティブスタイルから CSS 変数を生成して適用（背景・配色・フォント等）
  const varsStyle = $derived(varsToStyleString(styleToVars(styleStore.active)))

  // 本文のみを選択する（右クリックメニュー「すべて選択」および Ctrl+A と共通）。
  function selectAllBody(): void {
    if (!container) return
    const range = document.createRange()
    range.selectNodeContents(container)
    const sel = window.getSelection()
    sel?.removeAllRanges()
    sel?.addRange(range)
  }

  // 閲覧モードのコピーは書式を捨てプレーンテキストのみにする（メニューのコピーと挙動を揃える）。
  function onCopy(e: ClipboardEvent): void {
    const selection = window.getSelection()
    const text = selection?.toString() ?? ''
    if (!text) return
    // 選択が本文内のときのみ介入する（他所の選択には触れない）。
    if (container && selection?.anchorNode && !container.contains(selection.anchorNode)) return
    e.preventDefault()
    e.clipboardData?.setData('text/plain', text)
  }

  function onKeydown(e: KeyboardEvent): void {
    if (!(e.ctrlKey || e.metaKey) || e.altKey) return
    // Ctrl+F（閲覧モードのページ内検索）。
    if (!e.shiftKey && (e.key === 'f' || e.key === 'F')) {
      e.preventDefault()
      const wasOpen = viewFind.open
      viewFind.show()
      if (wasOpen) {
        findInput?.focus()
        findInput?.select()
      }
      return
    }
    // Ctrl+A は本文のみを対象にする（ブラウザ既定のページ全体選択ではなく）。
    if (!e.shiftKey && (e.key === 'a' || e.key === 'A')) {
      const t = e.target as HTMLElement | null
      if (t?.closest('input, textarea, [contenteditable="true"]')) return // 入力欄では既定動作
      e.preventDefault()
      selectAllBody()
    }
  }

  // 検索バー表示中の Esc で閉じる（入力欄にフォーカスがあっても拾う）。
  function onFindEsc(e: KeyboardEvent): void {
    if (viewFind.open && e.key === 'Escape') {
      e.preventDefault()
      closeFind()
    }
  }

  // Preview は閲覧モードのアクティブタブのみマウントされるため、購読はその間だけ有効。
  onMount(() => {
    window.addEventListener('copy', onCopy)
    window.addEventListener('keydown', onKeydown)
    window.addEventListener('keydown', onFindEsc)
    return () => {
      window.removeEventListener('copy', onCopy)
      window.removeEventListener('keydown', onKeydown)
      window.removeEventListener('keydown', onFindEsc)
      viewFind.close()
      clearFindHighlights()
    }
  })

  // --- ページ内検索（本文 .markdown-body を対象。設計: docs/アーキテクチャ・画面設計.md §5.12） ---
  const HL_ALL = 'mm-find'
  const HL_CUR = 'mm-find-current'
  const highlightSupported = typeof CSS !== 'undefined' && 'highlights' in CSS

  let findInput = $state<HTMLInputElement | undefined>(undefined)
  let findQuery = $state('')
  let matches: Range[] = []
  let findCount = $state(0)
  let findIndex = $state(-1)

  function clearFindHighlights(): void {
    if (!highlightSupported) return
    CSS.highlights.delete(HL_ALL)
    CSS.highlights.delete(HL_CUR)
  }

  function applyFindHighlights(): void {
    if (!highlightSupported) return
    if (matches.length === 0) {
      clearFindHighlights()
      return
    }
    CSS.highlights.set(HL_ALL, new Highlight(...matches))
    const cur = matches[findIndex]
    CSS.highlights.set(HL_CUR, new Highlight(...(cur ? [cur] : [])))
  }

  // 本文テキストノードを走査し、現在のクエリの一致範囲を Range として集める（大文字小文字は無視）。
  function computeMatches(): void {
    matches = []
    const root = container
    const q = findQuery
    if (root && q) {
      const needle = q.toLowerCase()
      const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT)
      let node = walker.nextNode()
      while (node) {
        const text = node.nodeValue ?? ''
        const hay = text.toLowerCase()
        let idx = hay.indexOf(needle)
        while (idx !== -1) {
          const r = document.createRange()
          r.setStart(node, idx)
          r.setEnd(node, idx + q.length)
          matches.push(r)
          idx = hay.indexOf(needle, idx + q.length)
        }
        node = walker.nextNode()
      }
    }
    findCount = matches.length
    if (matches.length === 0) findIndex = -1
    else if (findIndex < 0 || findIndex >= matches.length) findIndex = 0
    applyFindHighlights()
  }

  function scrollToCurrent(): void {
    const r = matches[findIndex]
    r?.startContainer.parentElement?.scrollIntoView({ block: 'center', behavior: 'smooth' })
  }

  function runFind(): void {
    findIndex = findQuery ? 0 : -1
    computeMatches()
    scrollToCurrent()
  }

  function gotoMatch(delta: number): void {
    if (matches.length === 0) return
    findIndex = (findIndex + delta + matches.length) % matches.length
    applyFindHighlights()
    scrollToCurrent()
  }

  function closeFind(): void {
    viewFind.close()
    clearFindHighlights()
  }

  function onFindInputKeydown(e: KeyboardEvent): void {
    if (e.key === 'Enter') {
      e.preventDefault()
      gotoMatch(e.shiftKey ? -1 : 1)
    }
  }

  // 検索バーが開いたら入力へフォーカスし、保持中のクエリで再計算する（Ctrl+F・メニューの両経路に対応）。
  let findWasOpen = false
  $effect(() => {
    const open = viewFind.open
    if (open && !findWasOpen) {
      requestAnimationFrame(() => {
        findInput?.focus()
        findInput?.select()
      })
      computeMatches()
    }
    findWasOpen = open
  })

  // source・テーマ・リモート画像ポリシーに依存して再描画。
  $effect(() => {
    const src = source
    const scheme = styleStore.active.colorScheme
    const policy = remoteImagePolicy
    const id = tabId
    const dir = fileDir
    // 未確認（undefined）または 'block' の間はリモート画像を読み込まない。
    const result = renderMarkdown(src, { allowRemoteImages: policy === 'allow' })
    // colorScheme が変わったら HTML 文字列も必ず変化させ、{@html} の再描画
    // → mermaid プレースホルダ再生成 → 新テーマで再描画 を確実に行う。
    html = result.html + `<!--markmiru-scheme:${scheme}-->`
    tick().then(() => {
      if (!container) return
      void runMermaid(container, scheme)
      void resolveLocalImages(container, dir)
      // 再描画で旧 Range は無効になるため、検索中なら作り直す。
      if (viewFind.open && findQuery) computeMatches()
    })
    // リモート画像を含み未確認の場合、ファイルごとに一度だけ表示可否を確認する。
    // セッション復元中（restoring）は commands.restoreSession が一括処理するためスキップ。
    if (result.hasRemoteImages && policy === undefined && id && !uiStore.restoring) {
      void confirmRemoteImages(id)
    }
  })

  async function confirmRemoteImages(id: string): Promise<void> {
    if (confirmingId === id) return
    confirmingId = id
    try {
      const ok = await riskyDialog.confirm({
        title: '外部画像の読み込み確認',
        message: `「${fileName || '無題'}」に外部画像が含まれています。読み込みますか？`,
        acceptLabel: '表示する',
        rejectLabel: '表示しない'
      })
      tabsStore.setRemoteImagePolicy(id, ok ? 'allow' : 'block')
    } finally {
      confirmingId = null
    }
  }
</script>

<!-- スクロールは全幅の外側コンテナ1か所に集約。CSS 変数はここに適用（子に継承）。 -->
<div class="preview-area">
  <div class="preview-scroll" style={varsStyle}>
    <div class="markdown-body" bind:this={container}>
      {@html html}
    </div>
  </div>

  {#if viewFind.open}
    <!-- フローティング検索バー（Chrome/Edge 風・右上）。本文はずれない。 -->
    <div class="find-bar" role="search">
      <input
        type="text"
        placeholder="検索"
        aria-label="ページ内検索"
        bind:value={findQuery}
        bind:this={findInput}
        oninput={() => runFind()}
        onkeydown={onFindInputKeydown}
      />
      <span class="find-count">{findQuery ? `${findIndex + 1}/${findCount}` : ''}</span>
      <button class="find-nav" title="前へ (Shift+Enter)" disabled={findCount === 0} onclick={() => gotoMatch(-1)}>▲</button>
      <button class="find-nav" title="次へ (Enter)" disabled={findCount === 0} onclick={() => gotoMatch(1)}>▼</button>
      <button class="find-close" title="閉じる (Esc)" onclick={() => closeFind()}>✕</button>
    </div>
  {/if}
</div>

<style>
  .preview-area {
    position: relative;
    height: 100%;
    min-height: 0;
  }
  .find-bar {
    position: absolute;
    top: 0.5rem;
    right: 1rem;
    z-index: 50;
    display: flex;
    align-items: center;
    gap: 0.25rem;
    background: #ffffff;
    color: #24292f;
    border: 1px solid #d0d7de;
    border-radius: 8px;
    box-shadow: 0 4px 14px rgba(0, 0, 0, 0.18);
    padding: 0.25rem 0.4rem;
  }
  .find-bar input {
    border: 1px solid #d0d7de;
    border-radius: 4px;
    padding: 0.2rem 0.45rem;
    font-size: 0.85rem;
    width: 12rem;
  }
  .find-count {
    font-size: 0.78rem;
    color: #57606a;
    min-width: 3rem;
    text-align: center;
  }
  .find-bar button {
    border: 1px solid transparent;
    background: transparent;
    color: #24292f;
    cursor: pointer;
    border-radius: 4px;
    padding: 0.15rem 0.4rem;
    font-size: 0.8rem;
    line-height: 1;
  }
  .find-bar button:hover:not(:disabled) {
    background: #eef1f4;
  }
  .find-bar button:disabled {
    color: #8b949e;
    cursor: default;
  }
  /* ページ内検索のハイライト（CSS Custom Highlight API）。全一致は淡色、現在の一致は濃色。 */
  :global(::highlight(mm-find)) {
    background-color: #fff3a3;
    color: #24292f;
  }
  :global(::highlight(mm-find-current)) {
    background-color: #ffb454;
    color: #24292f;
  }
</style>
