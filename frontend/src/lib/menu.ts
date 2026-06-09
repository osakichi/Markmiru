// ネイティブメニュー/ショートカット（Go 側）からのイベントを、フロントのコマンドに接続する。
// 設計: docs/アーキテクチャ・画面設計.md §1.1, §9
import { EventsOn } from '../../wailsjs/runtime/runtime'
import { tabsStore } from './stores/tabs.svelte'
import { uiStore } from './stores/ui.svelte'
import { openFiles, saveActive, saveActiveAs, requestQuit, printDocument, openFileByPath, openLicense, openReadme } from './commands'

/** メニューイベントの購読を開始し、解除関数を返す。 */
export function registerMenuHandlers(): () => void {
  const offs: Array<() => void> = [
    EventsOn('menu:new', () => tabsStore.newUntitled()),
    EventsOn('menu:open', () => {
      void openFiles()
    }),
    EventsOn('menu:save', () => {
      void saveActive()
    }),
    EventsOn('menu:saveAs', () => {
      void saveActiveAs()
    }),
    EventsOn('menu:print', () => {
      void printDocument()
    }),
    EventsOn('menu:toggleMode', () => {
      const active = tabsStore.active
      if (active) tabsStore.toggleMode(active.id)
    }),
    EventsOn('menu:toggleSidebar', () => uiStore.toggleSidebar()),
    EventsOn('menu:about', () => {
      void openReadme()
    }),
    EventsOn('menu:license', () => {
      void openLicense()
    }),
    EventsOn('app:request-quit', () => {
      void requestQuit()
    }),
    // IPC 経由（後発インスタンスから転送）のファイルを開く
    EventsOn('ipc:open-file', (path: string) => {
      void openFileByPath(path)
    })
  ]
  return () => offs.forEach((off) => off())
}
