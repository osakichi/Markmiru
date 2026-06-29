<script lang="ts">
  import { onMount } from 'svelte'
  import TabBar from './lib/components/TabBar.svelte'
  import Toolbar from './lib/components/Toolbar.svelte'
  import OpenFilesList from './lib/components/OpenFilesList.svelte'
  import ConfirmDialog from './lib/components/ConfirmDialog.svelte'
  import ExportPickerDialog from './lib/components/ExportPickerDialog.svelte'
  import RiskyConfirmDialog from './lib/components/RiskyConfirmDialog.svelte'
  import ContextMenu from './lib/components/ContextMenu.svelte'
  import SettingsPanel from './lib/components/SettingsPanel.svelte'
  import Preview from './lib/components/Preview.svelte'
  import Editor from './lib/components/Editor.svelte'
  import { tabsStore } from './lib/stores/tabs.svelte'
  import { isDirty } from './lib/stores/tabs'
  import { uiStore } from './lib/stores/ui.svelte'
  import { registerMenuHandlers } from './lib/menu'
  import { installLinkHandler } from './lib/links'
  import { setDirtyState, setEditMenuEnabled, getPendingFiles } from './lib/api/wails'
  import { restoreSession, currentConfig, schedulePersist, openFileByPath } from './lib/commands'
  import { styleStore } from './lib/style/style.svelte'
  import { applyHighlightTheme } from './lib/style/highlight'

  const active = $derived(tabsStore.active)

  // ネイティブメニュー/ショートカットのイベント購読（解除は onMount の戻り値で）
  onMount(() => registerMenuHandlers())

  // 外部リンクのクリックを外部ブラウザへ委譲（WebView 自体の遷移を防ぐ）
  onMount(() => installLinkHandler())

  // 起動時に前回セッションを復元し、その後に起動引数・IPC 早期受信ファイルを開く。
  // getPendingFiles() の呼出で Go 側の frontReady フラグが立ち、以降の IPC ファイルはイベントで配信される。
  onMount(async () => {
    uiStore.restoring = true
    try {
      await restoreSession()
      const pending = await getPendingFiles()
      for (const path of pending) {
        await openFileByPath(path)
      }
    } finally {
      uiStore.restoring = false
    }
  })

  // 未保存の有無を Go に通知（終了時の確認ループ起動判定に使用）
  $effect(() => {
    const hasUnsaved = tabsStore.tabs.some((t) => isDirty(t))
    void setDirtyState(hasUnsaved)
  })

  // アクティブタブのモードを Go に通知し、ネイティブ「編集」メニュー（Windows / Linux）の
  // 編集専用項目（取り消し/やり直し/切り取り/貼り付け）の有効・無効を切り替える。
  $effect(() => {
    const canEdit = tabsStore.active?.mode === 'source'
    void setEditMenuEnabled(canEdit)
  })

  // 設定（セッション・サイドバー状態・スタイル）を永続化。復元中・終了中は抑止。デバウンス保存。
  $effect(() => {
    if (uiStore.restoring || uiStore.closing) return
    schedulePersist(currentConfig())
  })

  // コードブロックのハイライトテーマをアクティブスタイルの colorScheme に連動（CSS は Go の chroma が生成）
  $effect(() => {
    void applyHighlightTheme(styleStore.active.colorScheme)
  })

  // カスタム CSS（スタイル）を style 要素へ反映
  $effect(() => {
    const css = styleStore.active.customCSS ?? ''
    let el = document.getElementById('markmiru-custom-css') as HTMLStyleElement | null
    if (!el) {
      el = document.createElement('style')
      el.id = 'markmiru-custom-css'
      document.head.appendChild(el)
    }
    el.textContent = css
  })

  // 印刷モード（表示通り=配色再現 / 既定=印刷向け変換）を body クラスへ反映
  $effect(() => {
    document.body.classList.toggle('print-as-displayed', uiStore.printAsDisplayed)
  })
</script>

<div class="app">
  {#if uiStore.sidebarOpen}
    <OpenFilesList />
  {/if}
  <div class="main">
    <TabBar />
    <Toolbar />
    <div class="content">
      {#if active}
        {#if active.mode === 'view'}
          <Preview
            source={active.content}
            tabId={active.id}
            fileName={active.fileName}
            fileDir={active.dirHint}
            remoteImagePolicy={active.remoteImagePolicy}
          />
        {:else}
          {#key active.id}
            <Editor
              value={active.content}
              onChange={(v) => active && tabsStore.setContent(active.id, v)}
            />
          {/key}
        {/if}
      {:else}
        <div class="empty">タブがありません。＋ または Ctrl+N で新規作成できます。</div>
      {/if}
    </div>
  </div>
</div>

<ConfirmDialog />
<ExportPickerDialog />
<RiskyConfirmDialog />
<ContextMenu />
<SettingsPanel />

<style>
  .app {
    height: 100%;
    display: flex;
    flex-direction: row;
    overflow: hidden;
  }
  .main {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    overflow: hidden;
  }
  .content {
    flex: 1;
    min-height: 0;
    overflow: hidden;
  }
  .empty {
    padding: 2rem;
    color: #8b949e;
  }
</style>
