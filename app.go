package main

import (
	"context"
	_ "embed"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"Markmiru/render"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// licenseMarkdown は同梱のライセンス文書（Markmiru 自体＋サードパーティ）。
// 実行ファイルに埋め込むため、配布時に別ファイルを配置する必要はない。
//
//go:embed LICENSE.md
var licenseMarkdown string

// readmeMarkdown は同梱の README（概要・機能一覧等）。
// ネイティブメニュー「Markmiru について...」で About 画面の代わりとして表示する。
//
//go:embed README.md
var readmeMarkdown string

// App struct
type App struct {
	ctx          context.Context
	hasUnsaved   atomic.Bool
	quitting     atomic.Bool
	startupFiles []string   // コマンドライン引数のファイルパス（main から設定）
	pendingMu    sync.Mutex // pendingFiles / frontReady を保護
	pendingFiles []string   // フロント準備前に IPC 経由で届いたファイルパス
	frontReady   bool       // GetPendingFiles 呼出後に true になる（pendingMu で保護）
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.restoreWindowSize()
}

// GetPendingFiles はフロントエンド初期化完了後に一度だけ呼ぶ。
// 起動引数（コマンドライン）＋ IPC 早期受信ファイルをまとめて返し、内部リストをクリアする。
// 呼出後は frontReady = true となり、以降の IPC ファイルはイベントで即時配信される。
func (a *App) GetPendingFiles() []string {
	a.pendingMu.Lock()
	defer a.pendingMu.Unlock()
	a.frontReady = true
	all := make([]string, 0, len(a.startupFiles)+len(a.pendingFiles))
	all = append(all, a.startupFiles...)
	all = append(all, a.pendingFiles...)
	a.startupFiles = nil
	a.pendingFiles = nil
	return all
}

// openFileFromIPC は IPC 経由で受け取ったパスをフロントへ渡す。
// フロント準備前に届いた場合はキューに積み、準備完了後（GetPendingFiles 呼出後）はイベントで即時配信する。
func (a *App) openFileFromIPC(path string) {
	a.pendingMu.Lock()
	if !a.frontReady {
		a.pendingFiles = append(a.pendingFiles, path)
		a.pendingMu.Unlock()
		return
	}
	a.pendingMu.Unlock()
	runtime.EventsEmit(a.ctx, "ipc:open-file", path)
}

// bringToFront はウィンドウを前面に表示する。IPC 受信時に呼ぶ。
// runtime.WindowShow で表示状態を復元した後、Windows では Win32 の
// SetForegroundWindow を呼ぶことでフォアグラウンドロックを越えて前面に出す。
func (a *App) bringToFront() {
	if a.ctx == nil {
		return
	}
	runtime.WindowShow(a.ctx)
	activateWindowWin32() // Windows: SetForegroundWindow; 他 OS: no-op
}

// emit はネイティブメニュー等からフロントへイベントを送る内部ヘルパ。
// 小文字始まりのためバインディングには公開されない。
func (a *App) emit(event string) {
	runtime.EventsEmit(a.ctx, event)
}

// SetDirtyState はフロントから未保存有無を通知する（終了時に確認ループを起動するか判断する）。
func (a *App) SetDirtyState(hasUnsaved bool) {
	a.hasUnsaved.Store(hasUnsaved)
}

// SetEditMenuEnabled は手組み「編集」メニュー（Windows / Linux）の編集専用項目
// （取り消し/やり直し/切り取り/貼り付け）の有効・無効を、編集可能か（編集モードか）で切り替える。
// フロントがアクティブタブのモード変化時に呼ぶ。macOS はネイティブ編集メニューが文脈に応じて
// 自動制御するため何もしない。
func (a *App) SetEditMenuEnabled(canEdit bool) {
	if isMacOS || a.ctx == nil {
		return
	}
	for _, it := range editOnlyMenuItems {
		it.Disabled = !canEdit
	}
	runtime.MenuUpdateApplicationMenu(a.ctx)
}

// FocusWindow は WebView にキーボードフォーカスを与える。
// Windows の WebView2 は起動直後クリックするまでキー入力が届かないため、
// アプリ内ダイアログ表示時にフロントから呼ぶ（他 OS は no-op）。
func (a *App) FocusWindow() {
	focusWebview()
}

// ClipboardGetText は OS クリップボードのテキストを返す（右クリックメニューの貼り付け用）。
// ブラウザのクリップボード API 制限を避けるため、OS クリップボードを Go 経由で扱う。
func (a *App) ClipboardGetText() (string, error) {
	return runtime.ClipboardGetText(a.ctx)
}

// ClipboardSetText は OS クリップボードへテキストを書き込む（右クリックメニューのコピー/切り取り用）。
func (a *App) ClipboardSetText(text string) error {
	return runtime.ClipboardSetText(a.ctx, text)
}

// Quit はアプリを終了する（フロントの終了確認ループ完了後に呼ばれる）。
func (a *App) Quit() {
	a.quitting.Store(true)
	runtime.Quit(a.ctx)
}

// beforeClose はウィンドウを閉じる直前に呼ばれる。
// 未保存があればフロントの終了確認ループ（タブごとの3択）を起動し、いったん閉じるのを中止する。
// 実際に閉じる経路（return false）では、直前に現在のウィンドウサイズを保存する。
// 設計: docs/アーキテクチャ・画面設計.md §5.3（タブごと確認方式）
func (a *App) beforeClose(ctx context.Context) bool {
	if a.quitting.Load() {
		a.saveWindowState()
		return false
	}
	if !a.hasUnsaved.Load() {
		a.saveWindowState()
		return false
	}
	a.emit("app:request-quit")
	return true
}

// restoreWindowSize は前回保存した通常時のウィンドウサイズを runtime.WindowSetSize で復元する。
//
// 復元を「起動オプション（options.Width/Height）」ではなく WindowSetSize で行うのが要点。
// 保存に使う WindowGetSize と復元に使う WindowSetSize は、どのプラットフォームでも同一の基準
// （同じ次元）を指すため対称で、保存→復元の往復で値が安定する。
// 一方 options.Width/Height は macOS ではコンテンツ領域（タイトルバー除く）として解釈される
// のに対し WindowGetSize はフレーム（タイトルバー込み）を返すため非対称で、起動オプション経由で
// 復元すると開閉のたびにタイトルバー分だけウィンドウが肥大化する（macOS 固有）。この経路に
// 揃えることで、プラットフォーム分岐なしの共通コードで肥大化を解消する。
//
// 最大化起動時（WindowStartState=Maximised）はサイズを上書きしない（通常サイズは保持済み）。
func (a *App) restoreWindowSize() {
	if a.ctx == nil {
		return
	}
	cfg, err := a.LoadConfig()
	if err != nil || cfg.WindowMaximised {
		return
	}
	if cfg.WindowWidth > 0 && cfg.WindowHeight > 0 {
		runtime.WindowSetSize(a.ctx, cfg.WindowWidth, cfg.WindowHeight)
	}
}

// saveWindowState は現在のウィンドウサイズ／最大化状態を config に保存する。
// 最大化中は通常サイズ（復元サイズ）を上書きせず、最大化フラグのみ更新する。
// ウィンドウ状態は Go 側のこの経路だけが更新する（フロントの SaveConfig は既存値を保持）。
func (a *App) saveWindowState() {
	if a.ctx == nil {
		return
	}
	cfg, err := a.LoadConfig()
	if err != nil {
		return
	}
	maximised := runtime.WindowIsMaximised(a.ctx)
	cfg.WindowMaximised = maximised
	if !maximised {
		if w, h := runtime.WindowGetSize(a.ctx); w > 0 && h > 0 {
			cfg.WindowWidth = w
			cfg.WindowHeight = h
		}
	}
	_ = writeConfig(cfg)
}

// FileDoc はファイルのパス・名前・親ディレクトリ・内容をまとめた DTO。
// パスの分解は Go の path/filepath に委ねる（OS 依存の区切りも正しく扱える）。
type FileDoc struct {
	Path    string `json:"path"`
	Name    string `json:"name"`
	Dir     string `json:"dir"`
	Content string `json:"content"`
}

func newFileDoc(path, content string) FileDoc {
	return FileDoc{
		Path:    path,
		Name:    filepath.Base(path),
		Dir:     filepath.Dir(path),
		Content: content,
	}
}

// markdownFilters はファイルダイアログのフィルタ。
var markdownFilters = []runtime.FileFilter{
	{DisplayName: "Markdown (*.md;*.markdown;*.mdown;*.txt)", Pattern: "*.md;*.markdown;*.mdown;*.txt"},
	{DisplayName: "すべてのファイル (*.*)", Pattern: "*.*"},
}

// jsonFilters はスタイルの入出力ダイアログ用フィルタ。
var jsonFilters = []runtime.FileFilter{
	{DisplayName: "Markmiru スタイル (*.json)", Pattern: "*.json"},
	{DisplayName: "すべてのファイル (*.*)", Pattern: "*.*"},
}

// OpenFiles はネイティブのファイル選択（複数可）を開き、選択ファイルを読み込んで返す。
// 読み込めなかったファイルはスキップする。
func (a *App) OpenFiles() ([]FileDoc, error) {
	paths, err := runtime.OpenMultipleFilesDialog(a.ctx, runtime.OpenDialogOptions{
		Title:   "ファイルを開く",
		Filters: markdownFilters,
	})
	if err != nil {
		return nil, err
	}
	docs := make([]FileDoc, 0, len(paths))
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		docs = append(docs, newFileDoc(p, string(data)))
	}
	return docs, nil
}

// ReadFile は指定パスを読み込み、FileDoc として返す（セッション復元・再読込用）。
// 相対パスは絶対パスに変換してから返す。これにより openFromDoc の重複チェックが正しく機能する。
func (a *App) ReadFile(path string) (FileDoc, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return FileDoc{}, err
	}
	data, err := os.ReadFile(absPath)
	if err != nil {
		return FileDoc{}, err
	}
	return newFileDoc(absPath, string(data)), nil
}

// ReadLicense は実行ファイルに埋め込まれたライセンス文書（LICENSE.md）の内容を返す。
// ネイティブメニューの「ライセンス...」から呼び、編集不可タブとして表示する。
func (a *App) ReadLicense() string {
	return licenseMarkdown
}

// ReadReadme は実行ファイルに埋め込まれた README（README.md）の内容を返す。
// ネイティブメニューの「Markmiru について...」から呼び、編集不可タブとして表示する（About 代わり）。
// 先頭にビルド版（git ショート SHA）を表示して、About を開いてすぐバージョンを確認できるようにする。
func (a *App) ReadReadme() string {
	return "**バージョン**: `" + version + "`\n\n---\n\n" + readmeMarkdown
}

// SaveFileDialog は保存ダイアログを表示し、選択パスを返す（キャンセル時は空文字）。
func (a *App) SaveFileDialog(suggestedName string) (string, error) {
	return runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           "名前を付けて保存",
		DefaultFilename: suggestedName,
		Filters:         markdownFilters,
	})
}

// ExportStyleDialog はスタイル書き出し用の保存ダイアログを表示し、
// 選択パスを返す（キャンセル時は空文字）。書き込みは SaveFile を使う。
func (a *App) ExportStyleDialog(suggestedName string) (string, error) {
	return runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           "スタイルのエクスポート",
		DefaultFilename: suggestedName,
		Filters:         jsonFilters,
	})
}

// ImportStyleDialog はスタイル読み込み用の選択ダイアログ（単一）を表示し、
// 選択ファイルの内容を返す（キャンセル時は空文字）。
func (a *App) ImportStyleDialog() (string, error) {
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title:   "スタイルプロファイルのインポート",
		Filters: jsonFilters,
	})
	if err != nil || path == "" {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// imageMaxBytes はローカル画像として読み込む最大サイズ（50 MB）。
const imageMaxBytes = 50 * 1024 * 1024

// imageMIME はファイル拡張子から MIME タイプを返す。
func imageMIME(ext string) string {
	switch strings.ToLower(ext) {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".svg":
		return "image/svg+xml"
	case ".bmp":
		return "image/bmp"
	case ".ico":
		return "image/x-icon"
	case ".avif":
		return "image/avif"
	default:
		return "application/octet-stream"
	}
}

// resolveImagePath は src を baseDir を基点とした絶対パスに解決する。
//   - 絶対パス（filepath.IsAbs == true, 例: C:\... /home/...）: そのまま使用
//   - ルート相対パス（\ または / 始まりでボリューム名なし, 例: \Users\... /Users/...）:
//     baseDir のボリューム名（Windows では "C:" 等, 他 OS では ""）を先頭に付与して絶対化
//   - 相対パス（例: ./img.png）: baseDir と結合
func resolveImagePath(baseDir, src string) string {
	if filepath.IsAbs(src) {
		return filepath.Clean(src)
	}
	if len(src) > 0 && os.IsPathSeparator(src[0]) {
		// ルート相対: ボリューム名（Windows: "C:" / 他 OS: ""）を補完する
		vol := filepath.VolumeName(baseDir)
		return filepath.Clean(vol + src)
	}
	return filepath.Clean(filepath.Join(baseDir, src))
}

// ReadImageAsDataURL はローカル画像ファイルを読み込み、data URI として返す。
// src が相対パスの場合は baseDir を基点に解決する。
// ファイルが存在しない場合は空文字を返す（エラーにしない）。
func (a *App) ReadImageAsDataURL(baseDir, src string) (string, error) {
	absPath := resolveImagePath(baseDir, src)

	info, err := os.Stat(absPath)
	if err != nil {
		return "", nil // ファイル不在は空文字で返す
	}
	if info.Size() > imageMaxBytes {
		return "", fmt.Errorf("image too large: %d bytes", info.Size())
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		return "", nil
	}

	mime := imageMIME(filepath.Ext(absPath))
	encoded := base64.StdEncoding.EncodeToString(data)
	return fmt.Sprintf("data:%s;base64,%s", mime, encoded), nil
}

// SaveFile は内容を指定パスへ UTF-8 で書き込み、保存後の FileDoc を返す。
func (a *App) SaveFile(path string, content string) (FileDoc, error) {
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return FileDoc{}, err
	}
	return newFileDoc(path, content), nil
}

// RenderResult は RenderHTML の戻り値（閲覧モードの描画用）。
type RenderResult struct {
	HTML            string `json:"html"`
	HasRemoteImages bool   `json:"hasRemoteImages"`
}

// RenderHTML は Markdown を安全な HTML へ変換して返す（閲覧モードの描画）。
// mermaid は <pre class="mermaid"> プレースホルダのまま返り、描画は WebView の mermaid.js が担う。
// baseDir はローカル画像解決の基点（空なら data URI 化しない）。allowRemoteImages が false なら
// リモート画像は読み込まない。設計: docs/Go中心化移行設計.md §4
func (a *App) RenderHTML(content, baseDir string, allowRemoteImages bool) RenderResult {
	r, err := render.RenderMarkdown(content, render.Options{BaseDir: baseDir, AllowRemoteImages: allowRemoteImages})
	if err != nil {
		return RenderResult{}
	}
	return RenderResult{HTML: r.HTML, HasRemoteImages: r.HasRemoteImages}
}

// HighlightCSS はコードハイライト用 CSS（chroma クラス）を colorScheme（"light"/"dark"）に応じて返す。
func (a *App) HighlightCSS(scheme string) string {
	return render.HighlightCSS(scheme)
}
