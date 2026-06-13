// 開く/保存などのユーザー操作（コマンド）。ツールバー・メニュー（step4-2）から呼ぶ。
import { tick } from 'svelte'
import { tabsStore } from './stores/tabs.svelte'
import { isDirty } from './stores/tabs'
import { uiStore } from './stores/ui.svelte'
import { styleStore } from './style/style.svelte'
import { contentHasRemoteImages } from './markdown/renderer'
import { serializeStyle, parseStyleFile, type Style } from './style/styleDef'
import {
  openFilesDialog,
  saveFileDialog,
  saveFile,
  readFile,
  readLicense,
  readReadme,
  exportStyleDialog,
  importStyleDialog,
  quit,
  loadConfig,
  saveConfig,
  type AppConfig,
  type FileDoc
} from './api/wails'
import { dialogStore } from './dialog.svelte'
import { riskyDialog } from './riskyDialog.svelte'
import { stylePicker } from './stylePicker.svelte'

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

/** ライセンスタブの表示名。 */
const LICENSE_TAB_NAME = 'ライセンス'

/**
 * 同梱ライセンス（LICENSE.md）を編集不可タブで開く。メニュー「ライセンス...」から呼ぶ。
 * - filePath を持たせない（null）ため「開いているファイル一覧」にパスは出ず、セッションにも保存されない。
 * - readOnly タブとして開き、編集モードへの切替・保存を無効化する。
 * - 既に開いていれば再取得せずそのタブをアクティブ化する。
 */
export async function openLicense(): Promise<void> {
  const existing = tabsStore.tabs.find((t) => t.readOnly && t.fileName === LICENSE_TAB_NAME)
  if (existing) {
    tabsStore.activate(existing.id)
    return
  }
  const content = await readLicense()
  tabsStore.open({
    filePath: null,
    fileName: LICENSE_TAB_NAME,
    content,
    mode: 'view',
    readOnly: true
  })
}

/** README タブの表示名。 */
const README_TAB_NAME = 'Markmiru について'

/**
 * 同梱 README（README.md）を編集不可タブで開く。メニュー「Markmiru について...」から呼ぶ（About 代わり）。
 * 挙動は openLicense と同様（filePath=null・readOnly・既存があれば再利用）。
 */
export async function openReadme(): Promise<void> {
  const existing = tabsStore.tabs.find((t) => t.readOnly && t.fileName === README_TAB_NAME)
  if (existing) {
    tabsStore.activate(existing.id)
    return
  }
  const content = await readReadme()
  tabsStore.open({
    filePath: null,
    fileName: README_TAB_NAME,
    content,
    mode: 'view',
    readOnly: true
  })
}

/** ファイル名に使えない文字を _ に置換する。 */
function safeFileName(name: string): string {
  return (name.trim() || 'style').replace(/[\\/:*?"<>|]/g, '_')
}

/**
 * スタイルをファイルへエクスポートする（メタ情報付き封筒の JSON）。
 * メニュー「ファイル → スタイル → エクスポート...」から呼ぶ。対象はピッカーで選ばせ、
 * プリセット（builtin）はグレー表示で選択不可。設計: docs/スタイル設定設計.md §6
 */
export async function exportStyle(): Promise<void> {
  const items = styleStore.styles.map((p) => ({ id: p.id, name: p.name, builtin: p.builtin }))
  const id = await stylePicker.pick(items)
  if (!id) return
  const p = styleStore.styles.find((x) => x.id === id)
  if (!p || p.builtin) return
  const path = await exportStyleDialog(`${safeFileName(p.name)}.json`)
  if (!path) return
  await saveFile(path, serializeStyle(p))
}

/**
 * スタイルをファイルからインポートする。メニュー「ファイル → スタイル → インポート...」から呼ぶ。
 * 重複は「名前」で判定する（ユーザーから見た同一性は名前で表れるため）。
 * - 同名の既存ユーザースタイルがある場合は確認（上書き / 別名で追加 / キャンセル）。
 * - 同名のプリセットと衝突する場合は上書き不可のため、名前を一意化して追加。
 * - 衝突なしならそのままの名前で追加。ID は常に新規採番する。
 * 不正なファイルはエラーダイアログで通知し、状態は変更しない。
 */
export async function importStyle(): Promise<void> {
  const text = await importStyleDialog()
  if (!text) return
  let parsed: Style
  try {
    parsed = parseStyleFile(text)
  } catch (e) {
    const reason = e instanceof Error ? e.message : '不明なエラー'
    await dialogStore.notify('インポートに失敗しました', `スタイルを読み込めませんでした。\n${reason}`)
    return
  }
  const existing = styleStore.findByName(parsed.name)
  if (existing && !existing.builtin) {
    const choice = await dialogStore.confirmImportOverwrite(existing.name)
    if (choice === 'cancel') return
    if (choice === 'overwrite') {
      styleStore.overwriteImported(existing.id, parsed)
    } else {
      styleStore.addImported(parsed, { rename: true }) // 別名で追加（名前を一意化）
    }
    return
  }
  // 衝突なし → そのまま。プリセット名と衝突 → 名前を一意化して追加。
  styleStore.addImported(parsed, { rename: !!existing })
}

/** アクティブタブを保存。無題（パス未設定）なら保存先を尋ねる。 */
export async function saveActive(): Promise<void> {
  const tab = tabsStore.active
  if (!tab || tab.readOnly) return
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
  if (!tab || tab.readOnly) return
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
    stylesJson: JSON.stringify(styleStore.userStyles),
    activeStyleId: styleStore.activeId
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

  // スタイル（ユーザー定義）とアクティブスタイルを復元
  let userStyles: Style[] = []
  if (cfg.stylesJson) {
    try {
      userStyles = JSON.parse(cfg.stylesJson) as Style[]
    } catch {
      userStyles = []
    }
  }
  styleStore.loadUserStyles(userStyles, cfg.activeStyleId)

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

  // 外部画像を含む全タブ（非アクティブ含む）について表示可否を確認する。
  // Preview は uiStore.restoring 中はダイアログを表示しないため、ここで一括処理する。
  for (const tab of tabsStore.tabs) {
    if (contentHasRemoteImages(tab.content)) {
      const ok = await riskyDialog.confirm({
        title: '外部画像の読み込み確認',
        message: `「${tab.fileName}」に外部画像が含まれています。読み込みますか？`,
        acceptLabel: '表示する',
        rejectLabel: '表示しない'
      })
      tabsStore.setRemoteImagePolicy(tab.id, ok ? 'allow' : 'block')
    }
  }
}

/** 現在の状態から保存すべき設定を組み立てる（無題=パス無しタブはセッションに含めない）。 */
export function currentConfig(): AppConfig {
  const fileTabs = tabsStore.tabs.filter((t) => t.filePath)
  const files = fileTabs.map((t) => ({ path: t.filePath as string }))
  const activeIndex = fileTabs.findIndex((t) => t.id === tabsStore.activeId)
  return {
    session: { files, activeIndex },
    sidebarOpen: uiStore.sidebarOpen,
    stylesJson: JSON.stringify(styleStore.userStyles),
    activeStyleId: styleStore.activeId
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
