package main

import (
	"embed"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/menu"
	"github.com/wailsapp/wails/v2/pkg/menu/keys"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	wailswindows "github.com/wailsapp/wails/v2/pkg/options/windows"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

//go:embed all:frontend/dist
var assets embed.FS

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

// buildMenu はネイティブメニューを構築する。
// クリック/ショートカットは runtime イベントでフロントへ通知し、フロント側でコマンドを実行する。
// 設計: docs/アーキテクチャ・画面設計.md §9
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
	fileMenu.AddText("保存", keys.CmdOrCtrl("s"), func(_ *menu.CallbackData) { app.emit("menu:save") })
	fileMenu.AddText("名前を付けて保存", keys.Combo("s", keys.CmdOrCtrlKey, keys.ShiftKey), func(_ *menu.CallbackData) { app.emit("menu:saveAs") })
	fileMenu.AddSeparator()
	fileMenu.AddText("PDF 出力 / 印刷", keys.CmdOrCtrl("p"), func(_ *menu.CallbackData) { app.emit("menu:print") })
	fileMenu.AddSeparator()
	styleMenu := fileMenu.AddSubmenu("スタイル")
	styleMenu.AddText("インポート...", nil, func(_ *menu.CallbackData) { app.emit("menu:style-import") })
	styleMenu.AddText("エクスポート...", nil, func(_ *menu.CallbackData) { app.emit("menu:style-export") })
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
	//   各項目はフロントへ menu:* を発火し、ContextMenu と同じ editActions のハンドラで実行する。
	//   ショートカットは「表示のみ」: ラベルに "\t" ＋ キー表記を埋め込む（Win32 が右寄せ表示）。
	//   アクセラレータ（第2引数）には登録しない＝WebView/CodeMirror のネイティブなキー処理や、
	//   設定パネル・検索バー等の入力欄での Ctrl+C/V/X を奪わないため（キー操作は元々ネイティブで動作）。
	//   編集専用の項目は閲覧モードでは非活性にする（SetEditMenuEnabled が切り替える）。
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
		// フロントからのモード通知（SetEditMenuEnabled）で切り替える。
		editOnlyMenuItems = []*menu.MenuItem{undoItem, redoItem, cutItem, pasteItem}
		for _, it := range editOnlyMenuItems {
			it.Disabled = true
		}
	}

	viewMenu := appMenu.AddSubmenu("表示")
	viewMenu.AddText("閲覧/編集切替", keys.CmdOrCtrl("e"), func(_ *menu.CallbackData) { app.emit("menu:toggleMode") })
	viewMenu.AddText("サイドバー", keys.CmdOrCtrl("b"), func(_ *menu.CallbackData) { app.emit("menu:toggleSidebar") })

	helpMenu := appMenu.AddSubmenu("ヘルプ")
	helpMenu.AddText("Markmiru について...", nil, func(_ *menu.CallbackData) { app.emit("menu:about") })
	helpMenu.AddText("ライセンス...", nil, func(_ *menu.CallbackData) { app.emit("menu:license") })

	return appMenu
}

func main() {
	app := NewApp()

	// 多重起動防止 + IPC。既存インスタンスへファイルを渡して終了する場合がある。
	args := os.Args[1:]
	if ensureSingleInstance(app, args) {
		os.Exit(0)
	}
	app.startupFiles = args

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

	err := wails.Run(&options.App{
		Title:            appTitle,
		Width:            width,
		Height:           height,
		WindowStartState: startState,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
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
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
