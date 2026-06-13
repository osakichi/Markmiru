// 閲覧モードのページ内検索バーの開閉状態。
// 検索ロジック自体は Preview.svelte が持つ（本文 .markdown-body を対象に検索・ハイライトする）。
// 開く操作は Ctrl+F（Preview）とコンテキストメニュー「検索…」から。

class ViewFindStore {
  open = $state(false)

  show(): void {
    this.open = true
  }

  close(): void {
    this.open = false
  }
}

export const viewFind = new ViewFindStore()
