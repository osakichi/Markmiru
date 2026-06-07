// タブ（＝開いているファイル）の状態モデル。
// データモデルの仕様は docs/アーキテクチャ・画面設計.md §2 を参照。
// タブ集合のストア（追加/閉じる/アクティブ切替）は step3 で実装する。

export type EditorMode = 'view' | 'source'

export interface Tab {
  id: string
  /** 任意ディレクトリの絶対パス。null = 無題（新規） */
  filePath: string | null
  /** 表示名（例: a.md） */
  fileName: string
  /** 同名ファイル区別用の親ディレクトリ等 */
  dirHint: string
  /** 現在の編集内容 */
  content: string
  /** 最後に保存した内容（dirty 判定用） */
  savedContent: string
  /** 既定は 'view'（閲覧重視） */
  mode: EditorMode
  /**
   * リモート（外部サーバー）画像の表示ポリシー。
   * undefined = 未確認（既定では遮断してレンダリング）/ 'allow' = 表示許可 / 'block' = 表示しない。
   * ファイルごとに開いた時に一度だけ確認する。セッションには永続化しない（次回起動時に再確認）。
   */
  remoteImagePolicy?: 'allow' | 'block'
}

/** 未保存（dirty）かどうか。 */
export function isDirty(tab: Tab): boolean {
  return tab.content !== tab.savedContent
}
