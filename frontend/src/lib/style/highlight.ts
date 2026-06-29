// コードブロックのシンタックスハイライト用 CSS を colorScheme に連動させる。
// 設計: docs/スタイル設定設計.md §5.1 / docs/Go中心化移行設計.md §4
// ハイライトは Go（chroma）が生成し、その CSS（chroma クラス。github / github-dark 相当）を
// アクティブな1つだけ <style> に注入して切り替える。
import { highlightCSS } from '../api/wails'
import type { ColorScheme } from './styleDef'

const STYLE_ID = 'markmiru-code-theme'

export async function applyHighlightTheme(scheme: ColorScheme): Promise<void> {
  let el = document.getElementById(STYLE_ID) as HTMLStyleElement | null
  if (!el) {
    el = document.createElement('style')
    el.id = STYLE_ID
    document.head.appendChild(el)
  }
  el.textContent = await highlightCSS(scheme)
}
