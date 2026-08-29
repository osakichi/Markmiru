//go:build !linux && !darwin && !windows

package main

import (
	"net"

	"github.com/wailsapp/wails/v2/pkg/options/linux"
)

// 対象外プラットフォーム用スタブ。Linux / macOS / Windows 以外でのビルドに備える。

// isMacOS は macOS 固有のメニュー構成（標準アプリメニューの付与等）を切り替えるための定数。
const isMacOS = false

func verifyPeer(_ *net.UnixConn) bool { return true }
func platformGrantForeground()        {}
func focusWebview()                   {}

// platformLinuxOptions は Linux 固有の Wails オプション（ウィンドウアイコン等）。この OS では nil。
func platformLinuxOptions() *linux.Options { return nil }

// registerDesktopIntegration は Linux 専用（.desktop / アイコンの自動登録）。この OS では何もしない。
func registerDesktopIntegration() {}

// platformRaiseWindow は Windows / Linux 専用（IPC 受信時の前面化）。この OS では何もしない。
func platformRaiseWindow(_ *App) {}

// platformPrint は macOS 専用のネイティブ印刷実装。この OS では未処理（false）を返し、
// 呼び出し側が Wails の WindowPrint（WebView 内で window.print() を実行）へフォールバックする。
func platformPrint() bool { return false }

// platformSetMenuItemsEnabled は Linux 専用（Wails のメニュー更新が効かないため GTK 項目を直接更新する）。
// この OS では未処理（false）を返し、呼び出し側が Wails の MenuUpdateApplicationMenu を使う。
func platformSetMenuItemsEnabled(_ []string, _ bool) bool { return false }
