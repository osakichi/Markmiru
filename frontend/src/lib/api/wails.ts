// Go バインディング（frontend/wailsjs/go/main/App）の薄いラッパ。
// 設計: docs/アーキテクチャ・画面設計.md §1.1
import {
  OpenFiles,
  ReadFile,
  ReadLicense,
  ReadReadme,
  ReadImageAsDataURL,
  SaveFile,
  SaveFileDialog,
  SetDirtyState,
  Quit,
  LoadConfig,
  SaveConfig,
  GetPendingFiles
} from '../../../wailsjs/go/main/App'
import type { main } from '../../../wailsjs/go/models'

export type FileDoc = main.FileDoc

export interface SessionFile {
  path: string
}
export interface Session {
  files: SessionFile[]
  activeIndex: number
}
export interface AppConfig {
  session: Session
  sidebarOpen: boolean
  /** ユーザープロファイル（StyleProfile[]）の JSON 文字列 */
  profilesJson: string
  /** アクティブプロファイル ID */
  activeProfileId: string
}

/** ファイル選択ダイアログ（複数可）で開き、読み込んだ FileDoc 配列を返す。 */
export async function openFilesDialog(): Promise<FileDoc[]> {
  return (await OpenFiles()) ?? []
}

/** 指定パスを読み込み FileDoc を返す（セッション復元・再読込用）。 */
export async function readFile(path: string): Promise<FileDoc> {
  return await ReadFile(path)
}

/** 実行ファイルに埋め込まれたライセンス文書（LICENSE.md）の内容を返す。 */
export async function readLicense(): Promise<string> {
  return (await ReadLicense()) ?? ''
}

/** 実行ファイルに埋め込まれた README（README.md）の内容を返す。About 代わりに表示する。 */
export async function readReadme(): Promise<string> {
  return (await ReadReadme()) ?? ''
}

/** 保存先選択ダイアログ。キャンセル時は空文字を返す。 */
export async function saveFileDialog(suggestedName: string): Promise<string> {
  return await SaveFileDialog(suggestedName)
}

/** 内容を指定パスへ書き込み、保存後の FileDoc を返す。 */
export async function saveFile(path: string, content: string): Promise<FileDoc> {
  return await SaveFile(path, content)
}

/** 未保存の有無を Go に通知する（終了時の判定に使用）。 */
export async function setDirtyState(hasUnsaved: boolean): Promise<void> {
  await SetDirtyState(hasUnsaved)
}

/** アプリを終了する。 */
export async function quit(): Promise<void> {
  await Quit()
}

/** 設定（セッション・サイドバー状態）を読み込む。 */
export async function loadConfig(): Promise<AppConfig> {
  return (await LoadConfig()) as unknown as AppConfig
}

/** 設定を保存する。 */
export async function saveConfig(cfg: AppConfig): Promise<void> {
  await SaveConfig(cfg as unknown as main.Config)
}

/** 起動引数・IPC 早期受信ファイルを取得する（フロントエンド初期化完了後に一度だけ呼ぶ）。 */
export async function getPendingFiles(): Promise<string[]> {
  return (await GetPendingFiles()) ?? []
}

/**
 * ローカル画像を data URI として返す。
 * src が相対パスの場合は baseDir を基点に解決する。
 * ファイル不在・サイズ超過などは空文字を返す。
 */
export async function readImageAsDataURL(baseDir: string, src: string): Promise<string> {
  return (await ReadImageAsDataURL(baseDir, src)) ?? ''
}
