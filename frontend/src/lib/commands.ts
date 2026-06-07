// 開く/保存などのユーザー操作（コマンド）。ツールバー・メニュー（step4-2）から呼ぶ。
import { tick } from 'svelte'
import { tabsStore } from './stores/tabs.svelte'
import { isDirty } from './stores/tabs'
import { uiStore } from './stores/ui.svelte'
import { styleStore } from './style/style.svelte'
import type { StyleProfile } from './style/profile'
import {
  openFilesDialog,
  saveFileDialog,
  saveFile,
  readFile,
  quit,
  loadConfig,
  saveConfig,
  type AppConfig,
  type FileDoc
} from './api/wails'
import { dialogStore } from './dialog.svelte'

function suggestName(fileName: string): string {
  return fileName.endsWith('.md') ? fileName : `${fileName}.md`
}

/** パスを指定してファイルを開く。ファイルが見つからない場合はセッション復元と同じ確認ダイアログを表示する。 */
export async function openFileByPath(path: string): Promise<void> {
  let doc = await tryRead(path)
  while (!doc) {
    const choice = await dialogStore.confirmMissing(path)
    if (choice === 'skip') return
    doc = await tryRead(path)
  }
  tabsStore.openFromDoc(doc)
}

/** ファイルを開く（複数可）。各ファイルをタブで開く。 */
export async function openFiles(): Promise<void> {
  const docs = await openFilesDialog()
  for (const doc of docs) {
    tabsStore.openFromDoc(doc)
  }
}

/** アクティブタブを保存。無題（パス未設定）なら保存先を尋ねる。 */
export async function saveActive(): Promise<void> {
  const tab = tabsStore.active
  if (!tab) return
  let path = tab.filePath
  if (!path) {
    path = await saveFileDialog(suggestName(tab.fileName))
    if (!path) return // キャンセル
  }
  const doc = await saveFile(path, tab.content)
  tabsStore.markSavedDoc(tab.id, doc)
}

/** .md を .pdf に置き換えた保存ダイアログ用ファイル名を返す。 */
function suggestPdfName(fileName: string): string {
  return fileName.toLowerCase().endsWith('.md')
    ? fileName.slice(0, -3) + '.pdf'
    : fileName + '.pdf'
}

/** 印刷／PDF 出力。閲覧モードに切替えてから window.print() を呼ぶ。設計 §5.2 */
export async function printDocument(): Promise<void> {
  const tab = tabsStore.active
  if (!tab) return
  if (tab.mode !== 'view') {
    tabsStore.setMode(tab.id, 'view')
    await tick()
    // mermaid 図の描画完了を少し待つ
    await new Promise((resolve) => setTimeout(resolve, 250))
  }
  // document.title がブラウザ印刷ダイアログのデフォルトファイル名になるため、
  // 印刷前にタブ名由来の .pdf 名を設定し、完了後に元に戻す。
  const prevTitle = document.title
  document.title = suggestPdfName(tab.fileName)
  window.print()
  document.title = prevTitle
}

/** アクティブタブを名前を付けて保存。 */
export async function saveActiveAs(): Promise<void> {
  const tab = tabsStore.active
  if (!tab) return
  const path = await saveFileDialog(suggestName(tab.fileName))
  if (!path) return
  const doc = await saveFile(path, tab.content)
  tabsStore.markSavedDoc(tab.id, doc)
}

/**
 * 終了確認ループ（タブごとに3択）。設計: docs/アーキテクチャ・画面設計.md §5.3
 * - 保存済みタブ: 確認なしで閉じる
 * - 未保存タブ: 3択（保存/保存しない/キャンセル）
 *   - 保存: （無題なら保存先選択→）保存して閉じる
 *   - 保存しない: 破棄して閉じる
 *   - キャンセル（または無題の保存先選択をキャンセル）: 終了処理を中止し、残りのタブはそのまま
 * 全タブを処理し終えたら Quit() で終了する。
 */
/** タブを閉じた結果。 */
export interface CloseOutcome {
  /** キャンセルされ、タブを閉じなかった場合 true */
  cancelled: boolean
  /** 閉じた後もディスク上に残るファイル（セッション復元対象）。無題の破棄時などは undefined。 */
  savedFile?: { path: string }
}

/**
 * タブを閉じる（未保存なら3択で確認）。設計: docs/アーキテクチャ・画面設計.md §5.3
 * - 未保存でない → そのまま閉じる
 * - 未保存 → 保存（無題は保存先ダイアログ。ｷｬﾝｾﾙなら閉じない）/ 保存しない（破棄して閉じる）/ キャンセル（閉じない）
 * 「プログラム終了」もこの処理を各タブに適用するだけにして実装を共通化する。
 */
export async function closeTab(id: string): Promise<CloseOutcome> {
  const tab = tabsStore.tabs.find((t) => t.id === id)
  if (!tab) return { cancelled: false }

  if (!isDirty(tab)) {
    const savedFile = tab.filePath ? { path: tab.filePath } : undefined
    tabsStore.close(id)
    return { cancelled: false, savedFile }
  }

  const choice = await dialogStore.confirmSave(tab.fileName)
  if (choice === 'save') {
    let path = tab.filePath
    if (!path) {
      path = await saveFileDialog(suggestName(tab.fileName))
      if (!path) return { cancelled: true } // 保存先選択をキャンセル → 閉じない
    }
    const doc = await saveFile(path, tab.content)
    tabsStore.markSavedDoc(id, doc)
    const savedFile = { path: doc.path }
    tabsStore.close(id)
    return { cancelled: false, savedFile }
  }
  if (choice === 'discard') {
    // 破棄: 既存ファイルはディスク上に残るので復元対象に含める。無題は含めない。
    const savedFile = tab.filePath ? { path: tab.filePath } : undefined
    tabsStore.close(id)
    return { cancelled: false, savedFile }
  }
  // キャンセル → 閉じない
  return { cancelled: true }
}

/**
 * 終了処理。開いているタブを1つずつ closeTab で閉じる（共通処理）。
 * 途中でキャンセルされたら終了そのものを中止する。全タブ閉じたら最終セッションを保存して終了。
 */
export async function requestQuit(): Promise<void> {
  // タブを閉じる間は反応的な設定保存を抑止し、最終セッションは末尾でまとめて保存する
  uiStore.closing = true

  const activeId = tabsStore.activeId
  const sessionFiles: { path: string }[] = []
  let activeIndex = -1

  const tabs = [...tabsStore.tabs]
  for (const tab of tabs) {
    const wasActive = tab.id === activeId
    const outcome = await closeTab(tab.id)
    if (outcome.cancelled) {
      // どこかでキャンセル → 終了を中止（残りのタブはそのまま）
      uiStore.closing = false
      return
    }
    if (outcome.savedFile) {
      sessionFiles.push(outcome.savedFile)
      if (wasActive) activeIndex = sessionFiles.length - 1
    }
  }

  await saveConfig({
    session: { files: sessionFiles, activeIndex },
    sidebarOpen: uiStore.sidebarOpen,
    profilesJson: JSON.stringify(styleStore.userProfiles),
    activeProfileId: styleStore.activeId
  })
  await quit()
}

// --- セッション復元・設定永続化（設計: docs/アーキテクチャ・画面設計.md §5.6, §7） ---

async function tryRead(path: string): Promise<FileDoc | null> {
  try {
    return await readFile(path)
  } catch {
    return null
  }
}

/** 起動時に前回のセッション（開いていたファイル群）を復元する。不在ファイルは再試行/スキップ。 */
export async function restoreSession(): Promise<void> {
  const cfg = await loadConfig()
  uiStore.sidebarOpen = cfg.sidebarOpen

  // スタイルプロファイル（ユーザー定義）とアクティブテーマを復元
  let userProfiles: StyleProfile[] = []
  if (cfg.profilesJson) {
    try {
      userProfiles = JSON.parse(cfg.profilesJson) as StyleProfile[]
    } catch {
      userProfiles = []
    }
  }
  styleStore.loadUserProfiles(userProfiles, cfg.activeProfileId)

  const files = cfg.session?.files ?? []
  let activeTabId: string | null = null
  for (let i = 0; i < files.length; i++) {
    const sf = files[i]
    let doc = await tryRead(sf.path)
    while (!doc) {
      const choice = await dialogStore.confirmMissing(sf.path)
      if (choice === 'skip') break
      doc = await tryRead(sf.path)
    }
    if (!doc) continue
    const tab = tabsStore.openFromDoc(doc)
    if (i === cfg.session.activeIndex) activeTabId = tab.id
  }
  if (activeTabId) tabsStore.activate(activeTabId)
}

/** 現在の状態から保存すべき設定を組み立てる（無題=パス無しタブはセッションに含めない）。 */
export function currentConfig(): AppConfig {
  const fileTabs = tabsStore.tabs.filter((t) => t.filePath)
  const files = fileTabs.map((t) => ({ path: t.filePath as string }))
  const activeIndex = fileTabs.findIndex((t) => t.id === tabsStore.activeId)
  return {
    session: { files, activeIndex },
    sidebarOpen: uiStore.sidebarOpen,
    profilesJson: JSON.stringify(styleStore.userProfiles),
    activeProfileId: styleStore.activeId
  }
}

/** 現在の設定を永続化する。 */
export async function persistConfig(): Promise<void> {
  await saveConfig(currentConfig())
}

// 設定保存のデバウンス（スライダー等の連続変更でファイル書込が多発しないように）。
let persistTimer: ReturnType<typeof setTimeout> | null = null
let pendingConfig: AppConfig | null = null

/** 設定保存を遅延実行でまとめる。App の永続化 effect から呼ぶ。 */
export function schedulePersist(cfg: AppConfig): void {
  pendingConfig = cfg
  if (persistTimer) clearTimeout(persistTimer)
  persistTimer = setTimeout(() => {
    persistTimer = null
    if (pendingConfig) void saveConfig(pendingConfig)
  }, 250)
}
