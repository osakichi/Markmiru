// 編集アクション（取り消し/やり直し/切り取り/コピー/貼り付け/すべて選択）。
// 右クリックメニュー（ContextMenu.svelte）と、ネイティブ編集メニュー（Go 側の menu:* イベント）の
// 双方から共有する。クリップボードは OS（Go runtime）経由で扱い、ブラウザ API の制限
// （特に貼り付けのプログラム的呼び出し制限）を回避する。
// 設計: docs/アーキテクチャ・画面設計.md §9
import { selectAll, undo, redo } from '@codemirror/commands'
import { tabsStore } from './stores/tabs.svelte'
import { getEditorView } from './editorBridge'
import { selectElementContents } from './dom'
import { clipboardGetText, clipboardSetText } from './api/wails'

/** 任意のテキストをクリップボードへ書き込む（空文字は無視）。 */
export async function copyText(text: string): Promise<void> {
  if (text) await clipboardSetText(text)
}

/** 編集モードの選択範囲（複数選択を含む）を、CodeMirror ネイティブ同様に改行連結して返す。 */
export function editorSelectionText(): string {
  const view = getEditorView()
  if (!view) return ''
  const { state } = view
  return state.selection.ranges
    .filter((r) => !r.empty)
    .map((r) => state.sliceDoc(r.from, r.to))
    .join(state.lineBreak)
}

export async function pasteIntoEditor(): Promise<void> {
  const view = getEditorView()
  if (!view) return
  const text = await clipboardGetText()
  if (text) view.dispatch(view.state.replaceSelection(text))
  view.focus()
}

export async function cutFromEditor(text: string): Promise<void> {
  const view = getEditorView()
  if (!view) return
  if (text) {
    await clipboardSetText(text)
    view.dispatch(view.state.replaceSelection(''))
  }
  view.focus()
}

export function selectAllEditor(): void {
  const view = getEditorView()
  if (!view) return
  selectAll(view) // CodeMirror 組み込みコマンド
  view.focus()
}

export function selectAllPreview(): void {
  selectElementContents(document.querySelector('.markdown-body'))
}

export function undoEditor(): void {
  const view = getEditorView()
  if (!view) return
  undo(view)
  view.focus()
}

export function redoEditor(): void {
  const view = getEditorView()
  if (!view) return
  redo(view)
  view.focus()
}

// --- ネイティブ編集メニュー（menu:* イベント）用：現在のモードに応じて実行先を振り分ける ---
// 右クリックと違いクリック地点を持たないため、アクティブタブのモードで対象（編集/閲覧）を決める。

function isSourceMode(): boolean {
  return tabsStore.active?.mode === 'source'
}

export function menuUndo(): void {
  if (isSourceMode()) undoEditor()
}

export function menuRedo(): void {
  if (isSourceMode()) redoEditor()
}

export async function menuCut(): Promise<void> {
  if (isSourceMode()) await cutFromEditor(editorSelectionText())
}

export async function menuCopy(): Promise<void> {
  if (isSourceMode()) {
    await copyText(editorSelectionText())
  } else {
    await copyText(window.getSelection()?.toString() ?? '')
  }
}

export async function menuPaste(): Promise<void> {
  // 貼り付けは編集モードのみ（閲覧モードは読み取り専用）。
  if (isSourceMode()) await pasteIntoEditor()
}

export function menuSelectAll(): void {
  if (isSourceMode()) selectAllEditor()
  else selectAllPreview()
}
