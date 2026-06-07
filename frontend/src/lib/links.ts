// 外部リンクの遷移制御（防御レビュー 2026-06-06 反映）。
//
// 目的: WebView 内で <a> をクリックしても WebView 自体が外部 URL へ遷移し、
//       アプリの UI が置き換わって操作不能になる（戻れない）のを防ぐ。
//   - http/https/mailto → 既定遷移を抑止し、BrowserOpenURL で OS の既定ブラウザ/メーラへ。
//   - ページ内アンカー（#...）→ WebView 遷移にならないため既定動作を許可。
//   - それ以外のスキーム → 念のため遷移を抑止（何も開かない）。
import { BrowserOpenURL } from '../../wailsjs/runtime/runtime'

// 外部ブラウザで開いてよいスキーム。
const EXTERNAL_SCHEME_RE = /^(https?|mailto):/i

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

  // ページ内アンカーは WebView 遷移を起こさないので既定動作のまま許可。
  if (rawHref.startsWith('#')) return

  // 解決済みの絶対 URL でスキームを判定。
  const href = a.href
  if (EXTERNAL_SCHEME_RE.test(href)) {
    e.preventDefault()
    BrowserOpenURL(href)
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
