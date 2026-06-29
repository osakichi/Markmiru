// 外部リンクの遷移制御（防御レビュー 2026-06-06 反映）。
//
// 目的: WebView 内で <a> をクリックしても WebView 自体が外部 URL へ遷移し、
//       アプリの UI が置き換わって操作不能になる（戻れない）のを防ぐ。
//   - http/https/mailto → 既定遷移を抑止し、BrowserOpenURL で OS の既定ブラウザ/メーラへ。
//   - ページ内アンカー（#...）→ 既定遷移は使わず、見出し id へ手動でスクロールする。
//   - それ以外のスキーム → 念のため遷移を抑止（何も開かない）。
import { BrowserOpenURL } from '../../wailsjs/runtime/runtime'
import { riskyDialog } from './riskyDialog.svelte'

// 外部ブラウザで開いてよいスキーム。
const EXTERNAL_SCHEME_RE = /^(https?|mailto):/i

// 外部リンクは確認ダイアログ（危険な選択）を挟んでから OS の既定アプリで開く。
async function confirmAndOpen(url: string): Promise<void> {
  const ok = await riskyDialog.confirm({
    title: '外部リンクを開きますか？',
    message: '次のリンクを既定のアプリで開きます。',
    detail: url,
    acceptLabel: 'はい',
    rejectLabel: 'いいえ'
  })
  if (ok) BrowserOpenURL(url)
}

/** クリックされた要素から最も近い祖先 <a> を返す（composedPath 経由でシャドウ越えも考慮）。 */
function findAnchor(e: MouseEvent): HTMLAnchorElement | null {
  const path = e.composedPath?.() ?? []
  for (const el of path) {
    if (el instanceof HTMLAnchorElement) return el
  }
  // フォールバック（composedPath 非対応環境）
  let node = e.target as Node | null
  while (node) {
    if (node instanceof HTMLAnchorElement) return node
    node = node.parentNode
  }
  return null
}

function onClick(e: MouseEvent): void {
  // 修飾キー併用や左クリック以外は通常処理に委ねる（ただし下で preventDefault する）
  if (e.defaultPrevented) return
  const a = findAnchor(e)
  if (!a) return

  // href 属性の生値（解決前）。空・未設定は無視。
  const rawHref = a.getAttribute('href')
  if (!rawHref) return

  // 文書内リンク（#...）はジャンプ先の見出し id へ手動でスクロールする。
  // 既定の遷移に任せると location.hash を書き換えてしまう＆同じリンク再クリックで
  // 再スクロールされないため、preventDefault して scrollIntoView で制御する。
  if (rawHref.startsWith('#')) {
    e.preventDefault()
    // href は Markdown 描画（goldmark）により % エンコードされ得る（例: #mermaid-%E5%9B%B3）。
    // 見出し id は素のスラッグ（例: mermaid-図）なのでデコードして突き合わせる。
    let id = rawHref.slice(1)
    try { id = decodeURIComponent(id) } catch { /* 不正なエンコードはそのまま使用 */ }
    if (!id) return
    document.getElementById(id)?.scrollIntoView({ behavior: 'smooth', block: 'start' })
    return
  }

  // 解決済みの絶対 URL でスキームを判定。
  const href = a.href
  if (EXTERNAL_SCHEME_RE.test(href)) {
    e.preventDefault()
    void confirmAndOpen(href)
    return
  }

  // その他（file: 等の未知スキーム、相対パス）は WebView 遷移を防ぐためブロック。
  e.preventDefault()
}

/** グローバルな外部リンクハンドラを登録する。戻り値は解除関数。 */
export function installLinkHandler(): () => void {
  // capture フェーズで先取りし、各所のリンクを一括で制御する。
  document.addEventListener('click', onClick, true)
  return () => document.removeEventListener('click', onClick, true)
}
