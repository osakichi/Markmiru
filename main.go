package main

import (
	"os"
	"path/filepath"

	"Markmiru/web"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/menu"
	"github.com/wailsapp/wails/v2/pkg/menu/keys"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	wailswindows "github.com/wailsapp/wails/v2/pkg/options/windows"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// version はビルド時に ldflags（-X main.version=<git ショート SHA>）で埋め込まれる版。
// 既定値 "dev" は scripts/build.ps1（または build.sh）を介さずに素の wails build をした場合の表示。
var version = "dev"

// appTitle はウィンドウタイトル。Windows ではこの文字列でメインウィンドウを検索する
// （platform_windows.go の findMainWindow）ため、両者で同一値を共有する。
const appTitle = "Markmiru"

// editOnlyMenuItems は手組み「編集」メニュー（Windows / Linux）のうち、編集モードでのみ
// 使える項目（取り消し/やり直し/切り取り/貼り付け）。閲覧モードでは非活性にするため、
// SetEditMenuEnabled から参照できるよう保持する。macOS はネイティブ編集メニューのため未使用。
var editOnlyMenuItems []*menu.MenuItem

// saveMenuItem はメニュー「ファイル → 保存」。アクティブタブが dirty または無題のときだけ
// 有効にするため SetSaveMenuEnabled から参照できるよう保持する（全 OS 手組み＝全 OS 対象）。
var saveMenuItem *menu.MenuItem

// modeMenuItem はメニュー「表示 → 閲覧/編集切替」。読み取り専用タブでは無効にするため
// SetModeMenuEnabled から参照できるよう保持する。既定は有効（起動時のアクティブは通常タブか
// 無題＝切替可。web.Server の変化検出の基準値と一致させてある）。
var modeMenuItem *menu.MenuItem

// buildMenu はネイティブメニューを構築する。
// クリック/ショートカットは runtime イベント（menu:*）を発火し、glue.js が htmx で対応する
// Go-SSR エンドポイントを叩く。設計: docs/アーキテクチャ・画面設計.md §9
func buildMenu(app *App) *menu.Menu {
	appMenu := menu.NewMenu()

	// macOS では先頭に標準アプリメニュー（About / サービス / 隠す / 終了 Cmd+Q）を置く。
	// これにより mac の慣習に沿った「Markmiru」メニューが提供され、Quit も標準で機能する。
	if isMacOS {
		appMenu.Append(menu.AppMenu())
	}

	fileMenu := appMenu.AddSubmenu("ファイル")
	fileMenu.AddText("新規", keys.CmdOrCtrl("n"), func(_ *menu.CallbackData) { app.emit("menu:new") })
	fileMenu.AddText("開く", keys.CmdOrCtrl("o"), func(_ *menu.CallbackData) { app.emit("menu:open") })
	saveMenuItem = fileMenu.AddText("保存", keys.CmdOrCtrl("s"), func(_ *menu.CallbackData) { app.emit("menu:save") })
	// 起動直後は無効から始める（セッション復元は常に閲覧・クリーンのため）。以後はサーバが
	// 状態変化に応じて SetSaveMenuEnabled で切り替える（dirty または無題のとき有効）。
	// この初期値は web.Server の変化検出の基準値（ゼロ値＝無効）と一致させてある。
	saveMenuItem.Disabled = true
	fileMenu.AddText("名前を付けて保存", keys.Combo("s", keys.CmdOrCtrlKey, keys.ShiftKey), func(_ *menu.CallbackData) { app.emit("menu:saveAs") })
	fileMenu.AddSeparator()
	fileMenu.AddText("PDF 出力 / 印刷", keys.CmdOrCtrl("p"), func(_ *menu.CallbackData) { app.emit("menu:print") })
	fileMenu.AddSeparator()
	styleMenu := fileMenu.AddSubmenu("スタイル")
	styleMenu.AddText("インポート...", nil, func(_ *menu.CallbackData) { app.emit("menu:style-import") })
	styleMenu.AddText("エクスポート...", nil, func(_ *menu.CallbackData) { app.emit("menu:style-export") })
	// 設定はファイル直下に置く（Windows の「ファイル > オプション」慣習。全 OS 同一実装）。
	// macOS 本来の定位置（アプリメニューの Settings…）は menu.AppMenu() がネイティブ固定で
	// 追加不可、Linux 慣習の「編集 > 設定」も macOS の menu.EditMenu() が固定のため OS 間で
	// 場所が割れる。将来スタイル以外の設定が増える想定のため「スタイル」配下にも入れない。
	fileMenu.AddSeparator()
	fileMenu.AddText("設定...", keys.CmdOrCtrl(","), func(_ *menu.CallbackData) { app.emit("menu:settings") })
	// macOS は標準アプリメニューが「終了（Cmd+Q）」を提供するため、ファイルメニューには置かない
	// （Cmd+Q の二重割り当てを避ける）。Windows / Linux ではここで提供する。
	if !isMacOS {
		fileMenu.AddSeparator()
		fileMenu.AddText("終了", keys.CmdOrCtrl("q"), func(_ *menu.CallbackData) { runtime.Quit(app.ctx) })
	}

	// 編集メニュー（ファイルと表示の間に置くのが各 OS の慣習）。
	// macOS: menu.EditMenu() のネイティブ標準メニュー。取り消し/コピー/貼り付け等が
	//   ネイティブセレクタ（copy: 等）経由でフォーカス文脈に応じて確実に動作するため維持する。
	//   【既知の制約】メニュー名・項目は英語（"Edit"/"Copy"…）固定で日本語化できない。Wails
	//   v2.12.0 のネイティブ実装（WailsMenu.m appendRole）がこれらの文字列をハードコードして
	//   おり、Info.plist の CFBundleLocalizations / ja.lproj 等のアプリ側ローカライズでは
	//   変更不可（日本語 OS でも "Edit" のまま）。日本語化は Wails のパッチ/フォーク、または
	//   ネイティブ動作を捨てて手組みするしかなく、現状は英語表記を許容する。docs の既知課題参照。
	// Windows / Linux: Wails のロールメニューは macOS 専用のため、日本語ラベルで手組みする。
	//   各項目は menu:* を発火し、glue.js の editExec（textarea への execCommand）が実行する。
	//   ショートカットは「表示のみ」: ラベルに "\t" ＋ キー表記を埋め込む（Win32 が右寄せ表示）。
	//   アクセラレータ（第2引数）には登録しない＝WebView（textarea）のネイティブなキー処理や、
	//   設定パネル・検索バー等の入力欄での Ctrl+C/V/X を奪わないため（キー操作は元々ネイティブで動作）。
	//   編集専用の項目は閲覧モードでは非活性にする（サーバ主導で SetEditMenuEnabled が切り替える）。
	if isMacOS {
		appMenu.Append(menu.EditMenu())
	} else {
		editMenu := appMenu.AddSubmenu("編集")
		undoItem := editMenu.AddText("取り消し\tCtrl+Z", nil, func(_ *menu.CallbackData) { app.emit("menu:undo") })
		redoItem := editMenu.AddText("やり直し\tCtrl+Y", nil, func(_ *menu.CallbackData) { app.emit("menu:redo") })
		editMenu.AddSeparator()
		cutItem := editMenu.AddText("切り取り\tCtrl+X", nil, func(_ *menu.CallbackData) { app.emit("menu:cut") })
		editMenu.AddText("コピー\tCtrl+C", nil, func(_ *menu.CallbackData) { app.emit("menu:copy") })
		pasteItem := editMenu.AddText("貼り付け\tCtrl+V", nil, func(_ *menu.CallbackData) { app.emit("menu:paste") })
		editMenu.AddText("すべて選択\tCtrl+A", nil, func(_ *menu.CallbackData) { app.emit("menu:selectAll") })
		editMenu.AddSeparator()
		editMenu.AddText("検索...\tCtrl+F", nil, func(_ *menu.CallbackData) { app.emit("menu:find") })

		// 編集モードでのみ使える項目。初期は閲覧モード相当（セッション復元は常に閲覧）として無効から始め、
		// サーバが本文描画のたびに SetEditMenuEnabled で切り替える。
		editOnlyMenuItems = []*menu.MenuItem{undoItem, redoItem, cutItem, pasteItem}
		for _, it := range editOnlyMenuItems {
			it.Disabled = true
		}
	}

	viewMenu := appMenu.AddSubmenu("表示")
	modeMenuItem = viewMenu.AddText("閲覧/編集切替", keys.CmdOrCtrl("e"), func(_ *menu.CallbackData) { app.emit("menu:toggleMode") })
	viewMenu.AddText("サイドバー", keys.CmdOrCtrl("b"), func(_ *menu.CallbackData) { app.emit("menu:toggleSidebar") })

	helpMenu := appMenu.AddSubmenu("ヘルプ")
	helpMenu.AddText("Markmiru について...", nil, func(_ *menu.CallbackData) { app.emit("menu:about") })
	helpMenu.AddText("ライセンス...", nil, func(_ *menu.CallbackData) { app.emit("menu:license") })

	return appMenu
}

// webHost は web パッケージの Host インタフェースを App 経由で実装する（web→main の境界アダプタ）。
// SSR ハンドラ（web）から必要な OS 連携（ダイアログ・ファイル I/O・外部 URL 起動・ウィンドウ等）を
// Wails runtime を持つ App のメソッドへ委譲する。
type webHost struct{ app *App }

func (h webHost) OpenFilesDialog() ([]web.OpenedFile, error) {
	docs, err := h.app.OpenFiles()
	if err != nil {
		return nil, err
	}
	out := make([]web.OpenedFile, len(docs))
	for i, d := range docs {
		out[i] = web.OpenedFile{Path: d.Path, Name: d.Name, Content: d.Content}
	}
	return out, nil
}

func (h webHost) SaveFileDialog(name string) (string, error) { return h.app.SaveFileDialog(name) }

func (h webHost) WriteFile(path, content string) error {
	_, err := h.app.SaveFile(path, content)
	return err
}

func (h webHost) ReadmeMarkdown() string  { return h.app.ReadReadme() }
func (h webHost) LicenseMarkdown() string { return h.app.ReadLicense() }

func (h webHost) ExportStyleDialog(name string) (string, error) { return h.app.ExportStyleDialog(name) }
func (h webHost) ImportStyleDialog() (string, error)            { return h.app.ImportStyleDialog() }

func (h webHost) SetEditMenuEnabled(canEdit bool)   { h.app.SetEditMenuEnabled(canEdit) }
func (h webHost) SetSaveMenuEnabled(canSave bool)   { h.app.SetSaveMenuEnabled(canSave) }
func (h webHost) SetModeMenuEnabled(canToggle bool) { h.app.SetModeMenuEnabled(canToggle) }
func (h webHost) Quit()                             { h.app.Quit() }

func (h webHost) OpenURL(url string) { h.app.OpenExternalURL(url) }

func (h webHost) ReadFile(path string) (string, string, bool) {
	doc, err := h.app.ReadFile(path)
	if err != nil {
		return "", "", false
	}
	return doc.Name, doc.Content, true
}

func main() {
	app := NewApp()

	// 多重起動防止 + IPC。既存インスタンスへファイルを渡して終了する場合がある。
	args := os.Args[1:]
	if ensureSingleInstance(app, args) {
		os.Exit(0)
	}
	app.startupFiles = args

	// Linux: シェル（GNOME 等）がドック・Alt+Tab・アプリ一覧のアイコンを解決できるよう、
	// ~/.local/share 配下へ .desktop とアイコンを自動登録する（他 OS は no-op）。
	registerDesktopIntegration()

	configDir, _ := os.UserConfigDir()

	// 前回保存したウィンドウサイズ／最大化状態で起動する（無ければ既定値）。
	cfg, _ := app.LoadConfig()
	width, height := cfg.WindowWidth, cfg.WindowHeight
	if width <= 0 {
		width = defaultWindowWidth
	}
	if height <= 0 {
		height = defaultWindowHeight
	}
	startState := options.Normal
	if cfg.WindowMaximised {
		startState = options.Maximised
	}

	// アセット配信: Go-SSR（web パッケージ）の http.Handler。状態を config から復元して構築する。
	// assetserver.Options は Assets=nil のとき全 GET を Handler へ転送する（TCP なしのプロセス内疑似 HTTP）。
	st := web.NewState()
	// スタイル・サイドバー・セッションを config から復元する（§5.6, §7）。
	st.RestoreStylesJSON(cfg.StylesJson, cfg.ActiveStyleId)
	st.SetSidebarOpen(cfg.SidebarOpen)

	// 開く対象パス = 前回セッション ∪ 起動引数（重複排除、順序: セッション→引数）。
	// アクティブは、起動引数があればその最後、無ければセッションのアクティブ。
	var paths []string
	seen := map[string]bool{}
	activePath := ""
	for _, f := range cfg.Session.Files {
		if !seen[f.Path] {
			seen[f.Path] = true
			paths = append(paths, f.Path)
		}
	}
	if i := cfg.Session.ActiveIndex; i >= 0 && i < len(cfg.Session.Files) {
		activePath = cfg.Session.Files[i].Path
	}
	for _, p := range app.startupFiles {
		abs, err := filepath.Abs(p)
		if err != nil {
			continue
		}
		if !seen[abs] {
			seen[abs] = true
			paths = append(paths, abs)
		}
		activePath = abs // 起動引数で指定したファイルを最後＝アクティブに
	}

	// 各パスを開く。読めなかったものは保留し、シェル表示時に再試行/スキップを確認する。
	var missing []string
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			missing = append(missing, p)
			continue
		}
		tab := st.AddTab(p, filepath.Base(p), string(data))
		if p == activePath {
			st.Activate(tab.ID)
		}
	}
	st.SetPendingMissing(missing)
	if len(paths) == 0 {
		st.NewUntitled() // 開く対象が全く無ければ空の新規タブ（不在ファイルのみのときは確認を優先）
	}
	app.unsavedChecker = st.HasUnsaved // 終了時の未保存判定をサーバ状態に委ねる
	// 起動引数は復元に使用済み。以後の単一インスタンス IPC はイベント（ipc:open-file）で即時配信する。
	app.startupFiles = nil
	app.pendingMu.Lock()
	app.frontReady = true
	app.pendingMu.Unlock()
	app.persistSession = func() { // ウィンドウを閉じる直前に現在のセッションを保存
		files, activeIndex := st.SessionFiles()
		c, _ := app.LoadConfig()
		sf := make([]SessionFile, len(files))
		for i, p := range files {
			sf[i] = SessionFile{Path: p}
		}
		c.Session = Session{Files: sf, ActiveIndex: activeIndex}
		c.SidebarOpen = st.SidebarOpen()
		c.StylesJson = st.UserStylesJSON()
		c.ActiveStyleId = st.ActiveStyle().ID
		_ = writeConfig(c) // ウィンドウ状態は saveWindowState が別途保持
	}
	assetOpts := &assetserver.Options{Handler: web.NewHandler(st, webHost{app})}

	err := wails.Run(&options.App{
		Title:            appTitle,
		Width:            width,
		Height:           height,
		WindowStartState: startState,
		AssetServer:      assetOpts,
		BackgroundColour: &options.RGBA{R: 255, G: 255, B: 255, A: 1},
		Menu:             buildMenu(app),
		OnStartup:        app.startup,
		OnBeforeClose:    app.beforeClose,
		Bind: []interface{}{
			app,
		},
		Windows: &wailswindows.Options{
			WebviewUserDataPath: filepath.Join(configDir, "Markmiru", "cache"),
		},
		// Linux 固有オプション（ウィンドウアイコン・ProgramName・GPU ポリシー維持）。
		// 他 OS では nil（Wails は自 OS 以外のオプションを無視する）。
		Linux: platformLinuxOptions(),
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
