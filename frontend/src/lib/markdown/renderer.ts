import MarkdownIt from 'markdown-it'
import taskLists from 'markdown-it-task-lists'
import hljs from 'highlight.js'
import DOMPurify from 'dompurify'
import mermaid from 'mermaid'
import type { ColorScheme } from '../style/profile'
import { readImageAsDataURL } from '../api/wails'

// 閲覧モードの Markdown レンダリングパイプライン
// 設計: docs/アーキテクチャ・画面設計.md §3
//   markdown-it（GFM）→ highlight.js / mermaid プレースホルダ
//   → DOMPurify でサニタイズ → DOM 反映後に mermaid.run() で図を描画
//
// セキュリティ（防御レビュー 2026-06-06 反映）:
//   - 外部リンクには rel="noopener noreferrer" を付与（target は付けない。
//     クリックは links.ts のハンドラが BrowserOpenURL で外部ブラウザへ委譲する）。
//   - リモート（http/https）画像は既定で遮断し、ファイルごとの許可があれば表示する。
//     遮断時はリクエストを飛ばさないよう src/srcset を剥がす（alt は残す）。

// http/https/プロトコル相対（//）の URL を外部リソースとみなす正規表現。
const REMOTE_URL_RE = /^(https?:)?\/\//i

let currentMermaidTheme = ''
function ensureMermaid(theme: 'default' | 'dark'): void {
  if (currentMermaidTheme === theme) return
  // securityLevel: 'strict' で未知ソースでも安全側に倒す
  mermaid.initialize({ startOnLoad: false, theme, securityLevel: 'strict' })
  currentMermaidTheme = theme
}

const md: MarkdownIt = new MarkdownIt({
  html: true, // 生 HTML は許可するが、後段の DOMPurify で無害化する
  linkify: true,
  typographer: false,
  highlight(code: string, lang: string): string {
    if (lang && hljs.getLanguage(lang)) {
      try {
        return `<pre class="hljs"><code>${hljs.highlight(code, { language: lang }).value}</code></pre>`
      } catch {
        /* 失敗時はエスケープのみ */
      }
    }
    return `<pre class="hljs"><code>${md.utils.escapeHtml(code)}</code></pre>`
  }
})

// GFM チェックリスト
md.use(taskLists, { enabled: true, label: true })

// ```mermaid フェンスは描画用プレースホルダとして出力（描画は DOM 反映後）
const defaultFence = md.renderer.rules.fence!
md.renderer.rules.fence = (tokens, idx, options, env, self) => {
  const info = tokens[idx].info.trim().toLowerCase()
  if (info === 'mermaid') {
    return `<pre class="mermaid">${md.utils.escapeHtml(tokens[idx].content)}</pre>`
  }
  return defaultFence(tokens, idx, options, env, self)
}

// DOMPurify フック用の状態（sanitize は同期実行のため、呼び出し前後で安全に共有できる）。
let allowRemoteImages = false // 現在のレンダリングでリモート画像を許可するか
let sawRemoteImage = false // 現在のレンダリングでリモート画像を検出したか

// uponSanitizeAttribute フック: ローカル絶対パスの src を DOMPurify による除去から保護する。
//
// 問題: DOMPurify は `C:\...` の `C:` を未知 URL スキームと解釈して src を除去する。
//       さらに markdown-it が URL エンコードを施すため `C:\` → `C:%5C` になる場合もある。
// 対応: Windows 絶対パス（[A-Za-z]: 始まり）を forceKeepAttr で強制保持し、
//       hookEvent.attrValue にデコード済みの値を設定して後続処理に渡す。
//       Unix 絶対パス（/ 始まり、// 除く）は DOMPurify が通常許容するため対象外。
DOMPurify.addHook('uponSanitizeAttribute', (node, hookEvent) => {
  if (hookEvent.attrName !== 'src') return
  if ((node as Element).tagName?.toLowerCase() !== 'img') return
  const val = hookEvent.attrValue
  // URL デコードして判定（markdown-it が \ → %5C、日本語等 → %xx にエンコードする場合を考慮）
  let decoded = val
  try { decoded = decodeURIComponent(val) } catch { /* 不正なエンコードはそのまま使用 */ }
  // Windows 絶対パス: "C:\" "C:/" 形式（デコード後）
  if (/^[a-zA-Z]:[/\\]/.test(decoded)) {
    hookEvent.forceKeepAttr = true
    hookEvent.attrValue = decoded // デコード済みの値を属性として保存
  }
})

// afterSanitizeAttributes フック:
//   - <a> 外部リンクへ rel="noopener noreferrer" を付与（target は付けない）。
//   - リモート画像を検出し、未許可なら src/srcset を剥がして読み込みを止める。
DOMPurify.addHook('afterSanitizeAttributes', (node) => {
  if (!(node instanceof Element)) return
  const tag = node.tagName.toLowerCase()

  if (tag === 'a') {
    const href = node.getAttribute('href') ?? ''
    if (REMOTE_URL_RE.test(href)) {
      node.setAttribute('rel', 'noopener noreferrer')
    }
    return
  }

  if (tag === 'img' || tag === 'source') {
    const src = node.getAttribute('src') ?? ''
    const srcset = node.getAttribute('srcset') ?? ''
    const isRemote = REMOTE_URL_RE.test(src) || REMOTE_URL_RE.test(srcset)
    if (!isRemote) return
    sawRemoteImage = true
    if (!allowRemoteImages) {
      // 遮断: 退避してから src/srcset を除去（読み込みを発生させない）。
      if (src) node.setAttribute('data-blocked-src', src)
      node.removeAttribute('src')
      node.removeAttribute('srcset')
      node.setAttribute('data-remote-blocked', '')
    }
  }
})

/**
 * Markdown テキストにリモート画像（http/https/プロトコル相対）が含まれるか簡易判定する。
 * セッション復元時の事前確認用。完全な判定は renderMarkdown の DOMPurify フックで行う。
 */
export function contentHasRemoteImages(content: string): boolean {
  // Markdown 画像構文: ![alt](https://...) / ![alt](//...)
  if (/!\[[^\]]*\]\(\s*(?:https?:)?\/\//i.test(content)) return true
  // 生 HTML: <img src="https://...">
  if (/<img\b[^>]*\bsrc\s*=\s*["'](?:https?:)?\/\//i.test(content)) return true
  return false
}

/** renderMarkdown の戻り値。hasRemoteImages はリモート画像が含まれていたか。 */
export interface RenderResult {
  html: string
  hasRemoteImages: boolean
}

/**
 * Markdown を安全な HTML に変換する（mermaid はプレースホルダのまま返る）。
 * opts.allowRemoteImages が false（既定）の場合、リモート画像は読み込まれない。
 */
export function renderMarkdown(src: string, opts: { allowRemoteImages?: boolean } = {}): RenderResult {
  const rawHtml = md.render(src ?? '')
  allowRemoteImages = opts.allowRemoteImages ?? false
  sawRemoteImage = false
  // 安全な HTML は許可しつつ、script 等の危険要素のみ除去
  const html = DOMPurify.sanitize(rawHtml)
  return { html, hasRemoteImages: sawRemoteImage }
}

/**
 * コンテナ内のローカル画像（相対パスまたは絶対パス）を Go 経由で読み込み、
 * data URI に差し替える。HTML 反映後に呼ぶ。
 * リモート URL・data URI・遮断済み画像はスキップする。
 * fileDir が空（無題ファイル等）の場合は何もしない。
 */
export async function resolveLocalImages(container: HTMLElement, fileDir: string): Promise<void> {
  if (!fileDir) return
  const imgs = Array.from(
    container.querySelectorAll<HTMLImageElement>('img:not([data-remote-blocked])')
  )
  await Promise.all(
    imgs.map(async (img) => {
      const rawSrc = img.getAttribute('src') ?? ''
      if (!rawSrc || rawSrc.startsWith('data:') || REMOTE_URL_RE.test(rawSrc)) return
      // URL デコード: uponSanitizeAttribute フックが効かない環境の保険と
      // Unix 絶対パスに含まれる日本語等の % エンコードへの対応。
      let srcPath = rawSrc
      try { srcPath = decodeURIComponent(rawSrc) } catch { /* invalid → as-is */ }
      if (!srcPath) return
      try {
        const dataUrl = await readImageAsDataURL(fileDir, srcPath)
        if (dataUrl) img.setAttribute('src', dataUrl)
      } catch {
        // 読み込み失敗 → 壊れた画像のまま（alt テキスト表示）
      }
    })
  )
}

/** コンテナ内の mermaid プレースホルダ（pre.mermaid）を SVG に描画する。HTML 反映後に呼ぶ。
 *  colorScheme に応じてテーマ（light=default / dark=dark）を切り替える。 */
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
