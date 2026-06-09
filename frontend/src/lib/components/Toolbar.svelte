<script lang="ts">
  import { tabsStore } from '../stores/tabs.svelte'
  import { uiStore } from '../stores/ui.svelte'
  import { styleStore } from '../style/style.svelte'
  import { printDocument } from '../commands'

  // 左: サイドバー開閉。右: テーマ（プロファイル）選択＋全画面トグル（閲覧 ⇄ 編集）。
  // ファイル操作はネイティブメニュー。PDF・設定パネル等は step5-4/5-6。
  const active = $derived(tabsStore.active)

  function onThemeChange(e: Event): void {
    styleStore.setActive((e.currentTarget as HTMLSelectElement).value)
  }
</script>

<div class="toolbar">
  <button
    class="icon-btn"
    title="サイドバー (Ctrl+B)"
    aria-label="サイドバーの表示切替"
    onclick={() => uiStore.toggleSidebar()}
  >☰</button>

  <div class="spacer"></div>

  <label class="theme">
    テーマ
    <select value={styleStore.activeId} onchange={onThemeChange}>
      {#each styleStore.profiles as p (p.id)}
        <option value={p.id}>{p.name}</option>
      {/each}
    </select>
  </label>

  <button
    class="icon-btn"
    title="スタイル設定"
    aria-label="スタイル設定"
    onclick={() => uiStore.openSettings()}
  >⚙</button>

  <button
    class="icon-btn"
    title="PDF 出力 / 印刷 (Ctrl+P)"
    aria-label="PDF出力/印刷"
    onclick={() => printDocument()}
  >PDF</button>

  <div class="mode-toggle" role="group" aria-label="表示モード">
    <button
      class:selected={active?.mode === 'view'}
      disabled={!active}
      onclick={() => active && tabsStore.setMode(active.id, 'view')}
    >閲覧</button>
    <button
      class:selected={active?.mode === 'source'}
      disabled={!active || active.readOnly}
      onclick={() => active && tabsStore.setMode(active.id, 'source')}
    >編集</button>
  </div>
</div>

<style>
  .toolbar {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    padding: 0.3rem 0.6rem;
    background: #f6f8fa;
    border-bottom: 1px solid #d0d7de;
  }
  .spacer {
    flex: 1;
  }
  .icon-btn {
    border: 1px solid #d0d7de;
    background: #ffffff;
    color: #24292f;
    padding: 0.2rem 0.55rem;
    font-size: 0.9rem;
    line-height: 1;
    border-radius: 6px;
    cursor: pointer;
  }
  .icon-btn:hover {
    background: #f3f4f6;
  }
  .theme {
    display: inline-flex;
    align-items: center;
    gap: 0.3rem;
    font-size: 0.78rem;
    color: #57606a;
  }
  .theme select {
    font-size: 0.8rem;
    padding: 0.15rem 0.3rem;
    border: 1px solid #d0d7de;
    border-radius: 6px;
    background: #ffffff;
  }
  .mode-toggle {
    display: inline-flex;
    border: 1px solid #d0d7de;
    border-radius: 6px;
    overflow: hidden;
  }
  .mode-toggle button {
    border: none;
    background: #ffffff;
    color: #57606a;
    padding: 0.2rem 0.9rem;
    font-size: 0.8rem;
    cursor: pointer;
  }
  .mode-toggle button + button {
    border-left: 1px solid #d0d7de;
  }
  .mode-toggle button.selected {
    background: #0969da;
    color: #ffffff;
  }
  .mode-toggle button:disabled {
    opacity: 0.5;
    cursor: default;
  }
</style>
