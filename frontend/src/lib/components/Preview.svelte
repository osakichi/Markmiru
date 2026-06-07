<script lang="ts">
  import { tick } from 'svelte'
  import { renderMarkdown, runMermaid, resolveLocalImages } from '../markdown/renderer'
  import { styleStore } from '../style/style.svelte'
  import { profileToVars, varsToStyleString } from '../style/profile'
  import { tabsStore } from '../stores/tabs.svelte'
  import { dialogStore } from '../dialog.svelte'
  import '../styles/markdown.css'

  // 閲覧モードのプレビュー。source（Markdown 文字列）を描画する。
  // fileDir: 開いているファイルの親ディレクトリ（相対パス画像の解決に使用）。空なら画像解決スキップ。
  // remoteImagePolicy: リモート画像の表示可否（未確認なら遮断してレンダリングし、初回に確認する）。
  let {
    source = '',
    tabId = '',
    fileName = '',
    fileDir = '',
    remoteImagePolicy
  }: {
    source?: string
    tabId?: string
    fileName?: string
    fileDir?: string
    remoteImagePolicy?: 'allow' | 'block'
  } = $props()

  let container = $state<HTMLElement | undefined>(undefined)
  let html = $state('')

  // 同一タブで確認ダイアログが多重に開かないようにするガード。
  let confirmingId: string | null = null

  // アクティブプロファイルから CSS 変数を生成して適用（背景・配色・フォント等）
  const varsStyle = $derived(varsToStyleString(profileToVars(styleStore.active)))

  // source・テーマ・リモート画像ポリシーに依存して再描画。
  $effect(() => {
    const src = source
    const scheme = styleStore.active.colorScheme
    const policy = remoteImagePolicy
    const id = tabId
    const dir = fileDir
    // 未確認（undefined）または 'block' の間はリモート画像を読み込まない。
    const result = renderMarkdown(src, { allowRemoteImages: policy === 'allow' })
    // colorScheme が変わったら HTML 文字列も必ず変化させ、{@html} の再描画
    // → mermaid プレースホルダ再生成 → 新テーマで再描画 を確実に行う。
    html = result.html + `<!--markmiru-scheme:${scheme}-->`
    tick().then(() => {
      if (!container) return
      void runMermaid(container, scheme)
      void resolveLocalImages(container, dir)
    })
    // リモート画像を含み、まだ未確認なら、ファイルごとに一度だけ表示可否を確認する。
    if (result.hasRemoteImages && policy === undefined && id) {
      void confirmRemoteImages(id)
    }
  })

  async function confirmRemoteImages(id: string): Promise<void> {
    if (confirmingId === id) return
    confirmingId = id
    try {
      const choice = await dialogStore.confirmRemoteImages(fileName || '無題')
      tabsStore.setRemoteImagePolicy(id, choice)
    } finally {
      confirmingId = null
    }
  }
</script>

<!-- スクロールは全幅の外側コンテナ1か所に集約。CSS 変数はここに適用（子に継承）。 -->
<div class="preview-scroll" style={varsStyle}>
  <div class="markdown-body" bind:this={container}>
    {@html html}
  </div>
</div>
