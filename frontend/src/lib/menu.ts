// ネイティブメニュー/ショートカット（Go 側）からのイベントを、フロントのコマンドに接続する。
// 設計: docs/アーキテクチャ・画面設計.md §1.1, §9
import { EventsOn } from '../../wailsjs/runtime/runtime'
import { tabsStore } from './stores/tabs.svelte'
import { uiStore } from './stores/ui.svelte'
import { openFiles, saveActive, saveActiveAs, requestQuit, printDocument, openFileByPath, openLicense, openReadme, exportStyle, importStyle } from './commands'
import { menuUndo, menuRedo, menuCut, menuCopy, menuPaste, menuSelectAll, menuFind } from './editActions'

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
    // 編集メニュー（Windows / Linux の手組みメニューから発火。macOS はネイティブ
    // 標準メニューが直接処理するため、これらのイベントは飛んでこない）。
    EventsOn('menu:undo', () => menuUndo()),
    EventsOn('menu:redo', () => menuRedo()),
    EventsOn('menu:cut', () => {
      void menuCut()
    }),
    EventsOn('menu:copy', () => {
      void menuCopy()
    }),
    EventsOn('menu:paste', () => {
      void menuPaste()
    }),
    EventsOn('menu:selectAll', () => menuSelectAll()),
    EventsOn('menu:find', () => menuFind()),
    EventsOn('menu:style-import', () => {
      void importStyle()
    }),
    EventsOn('menu:style-export', () => {
      void exportStyle()
    }),
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
