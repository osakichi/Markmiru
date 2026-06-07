import type { Tab, EditorMode } from './tabs'

// タブ（＝開いているドキュメント）の状態ストア（Svelte 5 runes）。
// 設計: docs/アーキテクチャ・画面設計.md §2, §5。
// step3 時点ではメモリ上のみ。ファイル読み書き（Go バインディング）は step4 で接続する。

let counter = 0
function nextId(): string {
  counter += 1
  return `tab-${counter}`
}

export interface OpenParams {
  filePath?: string | null
  fileName: string
  dirHint?: string
  content: string
  mode?: EditorMode
}

class TabsStore {
  tabs = $state<Tab[]>([])
  activeId = $state<string | null>(null)

  get active(): Tab | undefined {
    return this.tabs.find((t) => t.id === this.activeId)
  }

  /** ファイル/ドキュメントを開く。同一 filePath が既にあればアクティブ化する。 */
  open(params: OpenParams): Tab {
    if (params.filePath) {
      const existing = this.tabs.find((t) => t.filePath === params.filePath)
      if (existing) {
        this.activeId = existing.id
        return existing
      }
    }
    const tab: Tab = {
      id: nextId(),
      filePath: params.filePath ?? null,
      fileName: params.fileName,
      dirHint: params.dirHint ?? '',
      content: params.content,
      savedContent: params.content,
      mode: params.mode ?? 'view'
    }
    this.tabs.push(tab)
    this.activeId = tab.id
    return tab
  }

  /** 無題の新規ドキュメント（編集が目的なので編集モードで開く）。 */
  newUntitled(): Tab {
    return this.open({ filePath: null, fileName: '無題', content: '', mode: 'source' })
  }

  close(id: string): void {
    const idx = this.tabs.findIndex((t) => t.id === id)
    if (idx < 0) return
    this.tabs.splice(idx, 1)
    if (this.activeId === id) {
      const next = this.tabs[idx] ?? this.tabs[idx - 1]
      this.activeId = next ? next.id : null
    }
  }

  activate(id: string): void {
    this.activeId = id
  }

  setContent(id: string, content: string): void {
    const t = this.tabs.find((t) => t.id === id)
    if (t) t.content = content
  }

  setMode(id: string, mode: EditorMode): void {
    const t = this.tabs.find((t) => t.id === id)
    if (t) t.mode = mode
  }

  toggleMode(id: string): void {
    const t = this.tabs.find((t) => t.id === id)
    if (t) t.mode = t.mode === 'view' ? 'source' : 'view'
  }

  /** リモート画像の表示ポリシーを設定する（確認ダイアログの結果を反映）。 */
  setRemoteImagePolicy(id: string, policy: 'allow' | 'block'): void {
    const t = this.tabs.find((t) => t.id === id)
    if (t) t.remoteImagePolicy = policy
  }

  /** ファイル（Go から受け取った FileDoc 相当）を開く。同一パスがあれば再読込してアクティブ化。 */
  openFromDoc(doc: { path: string; name: string; dir: string; content: string }): Tab {
    const existing = this.tabs.find((t) => t.filePath === doc.path)
    if (existing) {
      existing.content = doc.content
      existing.savedContent = doc.content
      existing.fileName = doc.name
      existing.dirHint = doc.dir
      this.activeId = existing.id
      return existing
    }
    return this.open({
      filePath: doc.path,
      fileName: doc.name,
      dirHint: doc.dir,
      content: doc.content,
      mode: 'view'
    })
  }

  /** 保存完了後にパス・名前・保存済み内容を更新する。 */
  markSavedDoc(id: string, doc: { path: string; name: string; dir: string }): void {
    const t = this.tabs.find((t) => t.id === id)
    if (!t) return
    t.filePath = doc.path
    t.fileName = doc.name
    t.dirHint = doc.dir
    t.savedContent = t.content
  }
}

export const tabsStore = new TabsStore()
