<script lang="ts">
  import { dialogStore } from '../dialog.svelte'
  import { FocusWindow } from '../../../wailsjs/go/main/App'

  let boxEl = $state<HTMLDivElement | undefined>(undefined)

  // ダイアログが開いたら WebView にキーボードフォーカスを与え、ダイアログ内へ移す。
  // - 文書内（WebView）にフォーカスが無いと <svelte:window onkeydown> がキーを受け取れない。
  // - 起動直後、Windows の WebView2 は子ウィンドウで動くためクリックするまでキーボード
  //   フォーカスが入らない。FocusWindow()（Go）がメインウィンドウへ WM_SETFOCUS を送り、
  //   Wails の chromium.Focus() を誘発して WebView へフォーカスを渡す（他 OS は no-op）。
  // - requestAnimationFrame で次フレームに遅延し、プライマリボタンへフォーカスを当てる。
  $effect(() => {
    if (!dialogStore.open) return
    void FocusWindow()
    requestAnimationFrame(() => {
      const primary = boxEl?.querySelector<HTMLButtonElement>('button.primary')
      ;(primary ?? boxEl)?.focus()
    })
  })

  function onKeydown(e: KeyboardEvent): void {
    if (!dialogStore.open) return
    if (e.key === 'Escape') {
      e.preventDefault()
      dialogStore.choose(dialogStore.escValue)
    } else if (e.key === 'Enter') {
      e.preventDefault()
      dialogStore.choose(dialogStore.enterValue)
    }
  }
</script>

<svelte:window onkeydown={onKeydown} />

{#if dialogStore.open}
  <div class="overlay">
    <!-- tabindex="-1": プログラム的にフォーカス可能にする（Tab キーでは選択されない） -->
    <div class="box" role="dialog" aria-modal="true" aria-label={dialogStore.title}
         tabindex="-1" bind:this={boxEl}>
      <div class="title">{dialogStore.title}</div>
      <div class="message">{dialogStore.message}</div>
      <div class="buttons">
        {#each dialogStore.buttons as btn (btn.value)}
          <button class:primary={btn.primary} onclick={() => dialogStore.choose(btn.value)}>
            {btn.label}
          </button>
        {/each}
      </div>
    </div>
  </div>
{/if}

<style>
  .overlay {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.35);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 1000;
  }
  .box {
    background: #ffffff;
    color: #24292f;
    width: min(460px, 90vw);
    border-radius: 8px;
    box-shadow: 0 8px 28px rgba(0, 0, 0, 0.25);
    padding: 1.2rem 1.4rem;
    outline: none;
  }
  .title {
    font-weight: 600;
    font-size: 1rem;
    margin-bottom: 0.6rem;
  }
  .message {
    font-size: 0.9rem;
    line-height: 1.6;
    margin-bottom: 1.2rem;
    white-space: pre-wrap;
    word-break: break-all;
  }
  .buttons {
    display: flex;
    justify-content: flex-end;
    gap: 0.5rem;
  }
  .buttons button {
    border: 1px solid #d0d7de;
    background: #ffffff;
    color: #24292f;
    padding: 0.35rem 0.9rem;
    font-size: 0.85rem;
    border-radius: 6px;
    cursor: pointer;
  }
  .buttons button:hover {
    background: #f3f4f6;
  }
  .buttons button.primary {
    background: #0969da;
    border-color: #0969da;
    color: #ffffff;
  }
  .buttons button.primary:hover {
    background: #0a5fc2;
  }
</style>
