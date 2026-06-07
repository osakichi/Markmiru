import {defineConfig, type Plugin} from 'vite'
import {svelte} from '@sveltejs/vite-plugin-svelte'

// 本番ビルドの index.html にのみ CSP（コンテンツセキュリティポリシー）を注入する。
// dev（wails dev）では Vite の HMR が inline script / WebSocket を使うため適用しない。
// 防御レビュー 2026-06-06 反映。
//   - script-src 'self': 注入された inline script を遮断（XSS の多層防御。DOMPurify と二重化）
//   - style-src 'unsafe-inline': Svelte/CSS 変数/hljs テーマ/カスタム CSS のため必須
//   - img-src ... https:: リモート画像は許可ユーザー操作後に表示（遮断はアプリ側で制御）
//   - object/base/frame 系を 'none' で固める
const CSP = [
  "default-src 'self'",
  "script-src 'self'",
  "style-src 'self' 'unsafe-inline'",
  "img-src 'self' data: https:",
  "font-src 'self' data:",
  "connect-src 'self'",
  "object-src 'none'",
  "base-uri 'none'",
  "frame-src 'none'",
  "frame-ancestors 'none'",
  "form-action 'none'"
].join('; ')

function cspPlugin(): Plugin {
  return {
    name: 'markmiru-csp',
    apply: 'build',
    transformIndexHtml(html) {
      const tag = `    <meta http-equiv="Content-Security-Policy" content="${CSP}"/>\n`
      return html.replace('</head>', `${tag}</head>`)
    }
  }
}

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [svelte(), cspPlugin()]
})
