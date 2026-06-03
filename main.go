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

// buildMenu はネイティブメニューを構築する。
// クリック/ショートカットは runtime イベントでフロントへ通知し、フロント側でコマンドを実行する。
// 設計: docs/アーキテクチャ・画面設計.md §9
func buildMenu(app *App) *menu.Menu {
	appMenu := menu.NewMenu()

	fileMenu := appMenu.AddSubmenu("ファイル")
	fileMenu.AddText("新規", keys.CmdOrCtrl("n"), func(_ *menu.CallbackData) { app.emit("menu:new") })
	fileMenu.AddText("開く", keys.CmdOrCtrl("o"), func(_ *menu.CallbackData) { app.emit("menu:open") })
	fileMenu.AddText("保存", keys.CmdOrCtrl("s"), func(_ *menu.CallbackData) { app.emit("menu:save") })
	fileMenu.AddText("名前を付けて保存", keys.Combo("s", keys.CmdOrCtrlKey, keys.ShiftKey), func(_ *menu.CallbackData) { app.emit("menu:saveAs") })
	fileMenu.AddSeparator()
	fileMenu.AddText("PDF 出力 / 印刷", keys.CmdOrCtrl("p"), func(_ *menu.CallbackData) { app.emit("menu:print") })
	fileMenu.AddSeparator()
	fileMenu.AddText("終了", keys.CmdOrCtrl("q"), func(_ *menu.CallbackData) { runtime.Quit(app.ctx) })

	viewMenu := appMenu.AddSubmenu("表示")
	viewMenu.AddText("閲覧/ソース切替", keys.CmdOrCtrl("e"), func(_ *menu.CallbackData) { app.emit("menu:toggleMode") })
	viewMenu.AddText("サイドバー", keys.CmdOrCtrl("b"), func(_ *menu.CallbackData) { app.emit("menu:toggleSidebar") })

	// 標準の編集メニュー（コピー/貼り付け等）
	appMenu.Append(menu.EditMenu())

	return appMenu
}

func main() {
	ensureSingleInstance()

	app := NewApp()

	configDir, _ := os.UserConfigDir()

	err := wails.Run(&options.App{
		Title:  "Markmiru",
		Width:  1024,
		Height: 768,
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
