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
	//   Ctrl+C/V/X/A/Z はアクセラレータ登録しない（WebView/CodeMirror のネイティブなキー処理＝
	//   フォーカス文脈に応じた既存挙動を奪わないため）。メニューはクリックでの呼び出し用。
	if isMacOS {
		appMenu.Append(menu.EditMenu())
	} else {
		editMenu := appMenu.AddSubmenu("編集")
		editMenu.AddText("取り消し", nil, func(_ *menu.CallbackData) { app.emit("menu:undo") })
		editMenu.AddText("やり直し", nil, func(_ *menu.CallbackData) { app.emit("menu:redo") })
		editMenu.AddSeparator()
		editMenu.AddText("切り取り", nil, func(_ *menu.CallbackData) { app.emit("menu:cut") })
		editMenu.AddText("コピー", nil, func(_ *menu.CallbackData) { app.emit("menu:copy") })
		editMenu.AddText("貼り付け", nil, func(_ *menu.CallbackData) { app.emit("menu:paste") })
		editMenu.AddText("すべて選択", nil, func(_ *menu.CallbackData) { app.emit("menu:selectAll") })
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
