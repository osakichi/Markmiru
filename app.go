package main

import (
	"context"
	_ "embed"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"

	"github.com/wailsapp/wails/v2/pkg/menu"
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
	quitting     atomic.Bool
	startupFiles []string   // コマンドライン引数のファイルパス（main から設定）
	pendingMu    sync.Mutex // frontReady を保護
	frontReady   bool       // フロント（glue）が IPC を受け取れる状態か（main の起動シードで true・pendingMu で保護）

	// unsavedChecker は未保存タブの有無を返す（main が web.State.HasUnsaved を注入）。終了時の判定に使う。
	unsavedChecker func() bool
	// persistSession は現在のセッション（開いているファイル・スタイル・サイドバー）を config へ保存する
	// （main が注入）。ウィンドウを閉じる直前に呼ぶ。
	persistSession func()
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

// openFileFromIPC は IPC（2つ目の起動）で受け取ったパスをフロント（glue）へ渡し、開かせる。
// 準備後はイベント（ipc:open-file）で即時配信する。準備前（起動直後の一瞬）は破棄する
// ——単一インスタンスでは2つ目の起動時に1つ目は既に稼働中のため、取りこぼしは実質発生しない。
func (a *App) openFileFromIPC(path string) {
	a.pendingMu.Lock()
	ready := a.frontReady
	a.pendingMu.Unlock()
	if ready {
		runtime.EventsEmit(a.ctx, "ipc:open-file", path)
	}
}

// bringToFront はウィンドウを前面に表示する。IPC 受信時に呼ぶ。
// runtime.WindowShow で表示状態を復元した後、platformRaiseWindow が OS 固有の前面化を行う——
// Windows は Win32 の SetForegroundWindow でフォアグラウンドロックを越えて前面に出し、
// Linux は gtk_window_present で前面化する（WindowShow は Linux では gtk_widget_show のため、
// 既に表示中のウィンドウを前面に出す効果が無い）。macOS は WindowShow が前面化まで行う。
func (a *App) bringToFront() {
	if a.ctx == nil {
		return
	}
	runtime.WindowShow(a.ctx)
	platformRaiseWindow(a)
}

// emit はネイティブメニュー等からフロントへイベントを送る内部ヘルパ。
// 小文字始まりのためバインディングには公開されない。
func (a *App) emit(event string) {
	runtime.EventsEmit(a.ctx, event)
}

// setMenuItemsEnabled は項目群の有効/無効を切り替えてネイティブメニューを再描画する
// （編集メニュー・保存メニュー共通の下回り）。
func (a *App) setMenuItemsEnabled(items []*menu.MenuItem, enabled bool) {
	if a.ctx == nil || len(items) == 0 {
		return
	}
	for _, it := range items {
		it.Disabled = !enabled
	}
	runtime.MenuUpdateApplicationMenu(a.ctx)
}

// SetEditMenuEnabled は手組み「編集」メニュー（Windows / Linux）の編集専用項目
// （取り消し/やり直し/切り取り/貼り付け）の有効・無効を、編集可能か（編集モードか）で切り替える。
// サーバ（web の syncMenus）が状態の変化時に呼ぶ。macOS はネイティブ編集メニューが
// 文脈に応じて自動制御するため何もしない。
func (a *App) SetEditMenuEnabled(canEdit bool) {
	if isMacOS {
		return
	}
	a.setMenuItemsEnabled(editOnlyMenuItems, canEdit)
}

// SetSaveMenuEnabled はメニュー「ファイル → 保存」の有効・無効を切り替える。
// アクティブタブが dirty または保存先未定の無題のときだけ有効にする（未変更タブへの保存は
// サーバ側でも no-op のため、入口のメニューを塞いで操作不能を明示する）。ファイルメニューは
// 全 OS 手組みのため macOS も対象。サーバ（web の syncMenus）が変化時に呼ぶ。
func (a *App) SetSaveMenuEnabled(canSave bool) {
	if saveMenuItem == nil {
		return
	}
	a.setMenuItemsEnabled([]*menu.MenuItem{saveMenuItem}, canSave)
}

// SetModeMenuEnabled はメニュー「表示 → 閲覧/編集切替」の有効・無効を切り替える。
// 読み取り専用タブ（About/ライセンス）とタブ無しでは無効にする（サーバ側の SetMode no-op は
// 防御として残る）。サーバ（web の syncMenus）が変化時に呼ぶ。
func (a *App) SetModeMenuEnabled(canToggle bool) {
	if modeMenuItem == nil {
		return
	}
	a.setMenuItemsEnabled([]*menu.MenuItem{modeMenuItem}, canToggle)
}

// OpenExternalURL は URL を OS の既定ブラウザ／メーラで開く（プレビュー内の外部リンク用）。
// WebView 自体を外部 URL へ遷移させないための委譲先。呼び出し側でスキームを検証済みとする。
func (a *App) OpenExternalURL(url string) {
	if a.ctx != nil {
		runtime.BrowserOpenURL(a.ctx, url)
	}
}

// ClipboardText は OS のクリップボードのテキストを返す（右クリック／編集メニューの「貼り付け」用）。
// Chromium 系 WebView は script からのクリップボード読み取りを禁止しており
// （`document.execCommand('paste')` は無効、`navigator.clipboard.readText()` は権限要求になる）、
// WebView 側だけでは貼り付けを実装できないため Go 側で読む。Ctrl+V は WebView が自前で処理する。
func (a *App) ClipboardText() (string, error) {
	if a.ctx == nil {
		return "", nil
	}
	return runtime.ClipboardGetText(a.ctx)
}

// FocusWindow は WebView にキーボードフォーカスを与える。
// Windows の WebView2 は起動直後クリックするまでキー入力が届かないため、
// アプリ内ダイアログ表示時にフロントから呼ぶ（他 OS は no-op）。
func (a *App) FocusWindow() {
	focusWebview()
}

// Print は OS の印刷ダイアログを開く（glue の do-print から呼ばれる）。
// macOS の WKWebView は window.print() を無視するため JS からここへ委譲する。
// まず platformPrint（macOS のみ実体。縦向きデフォルトの自前 NSPrintOperation。
// 向き等はパネルで変更可能）を試し、未処理なら Wails の WindowPrint へフォールバックする
// （Windows/Linux はこちらが常用経路で、WebView 内で window.print() を実行する。
// macOS でのフォールバックは横向き固定だが印刷自体は可能）。
func (a *App) Print() {
	if platformPrint() {
		return
	}
	if a.ctx != nil {
		runtime.WindowPrint(a.ctx)
	}
}

// Quit はアプリを終了する（サーバ主導の終了確認ループが未保存を処理し終えた後、Host.Quit 経由で呼ばれる）。
func (a *App) Quit() {
	a.quitting.Store(true)
	runtime.Quit(a.ctx)
}

// beforeClose はウィンドウを閉じる直前に呼ばれる。
// 未保存があれば終了確認ループ（サーバ主導・タブごとの3択）を起動し、いったん閉じるのを中止する
// （app:request-quit を発火 → glue が /quit/request を叩き、web が未保存タブを順に確認する）。
// 実際に閉じる経路（return false）では、直前にセッションとウィンドウサイズを保存する。
// 設計: docs/アーキテクチャ・画面設計.md §5.3（タブごと確認方式）
func (a *App) beforeClose(ctx context.Context) bool {
	if a.quitting.Load() {
		a.persistSessionState()
		a.saveWindowState()
		return false
	}
	if !a.anyUnsaved() {
		a.persistSessionState()
		a.saveWindowState()
		return false
	}
	a.emit("app:request-quit")
	return true
}

// persistSessionState は注入済みなら現在のセッションを config へ保存する（Go-SSR ビルドのみ）。
func (a *App) persistSessionState() {
	if a.persistSession != nil {
		a.persistSession()
	}
}

// anyUnsaved は未保存タブがあるか（注入された unsavedChecker＝サーバ状態）を返す。
func (a *App) anyUnsaved() bool {
	if a.unsavedChecker != nil {
		return a.unsavedChecker()
	}
	return false
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
// ウィンドウ状態は Go 側のこの経路だけが更新する（セッション保存 persistSession は既存のウィンドウ値を保持）。
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

// SaveFile は内容を指定パスへ UTF-8 で書き込み、保存後の FileDoc を返す。
func (a *App) SaveFile(path string, content string) (FileDoc, error) {
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return FileDoc{}, err
	}
	return newFileDoc(path, content), nil
}
