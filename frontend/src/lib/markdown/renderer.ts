import mermaid from 'mermaid'
import type { ColorScheme } from '../style/styleDef'

// 閲覧モードの mermaid 描画と、リモート画像の簡易検出のみを担う薄いモジュール。
// Markdown → 安全 HTML の変換（旧 markdown-it / highlight.js / DOMPurify パイプライン）は
// Go の render パッケージへ移行した（バインディング RenderHTML / api.renderHTML）。
// 設計: docs/Go中心化移行設計.md §4

let currentMermaidTheme = ''
function ensureMermaid(theme: 'default' | 'dark'): void {
  if (currentMermaidTheme === theme) return
  // securityLevel: 'strict' で未知ソースでも安全側に倒す
  mermaid.initialize({ startOnLoad: false, theme, securityLevel: 'strict' })
  currentMermaidTheme = theme
}

/**
 * Markdown テキストにリモート画像（http/https/プロトコル相対）が含まれるか簡易判定する。
 * セッション復元時の事前確認用（完全な判定は Go 側 render が行う）。
 */
export function contentHasRemoteImages(content: string): boolean {
  // Markdown 画像構文: ![alt](https://...) / ![alt](//...)
  if (/!\[[^\]]*\]\(\s*(?:https?:)?\/\//i.test(content)) return true
  // 生 HTML: <img src="https://...">
  if (/<img\b[^>]*\bsrc\s*=\s*["'](?:https?:)?\/\//i.test(content)) return true
  return false
}

/**
 * コンテナ内の mermaid プレースホルダ（pre.mermaid）を SVG に描画する。HTML 反映後に呼ぶ。
 * colorScheme に応じてテーマ（light=default / dark=dark）を切り替える。
 */
export async function runMermaid(container: HTMLElement, scheme: ColorScheme = 'light'): Promise<void> {
  const nodes = Array.from(container.querySelectorAll<HTMLElement>('pre.mermaid'))
  if (nodes.length === 0) return
  ensureMermaid(scheme === 'dark' ? 'dark' : 'default')
  try {
    await mermaid.run({ nodes })
  } catch (err) {
    console.error('mermaid render error:', err)
  }
}
