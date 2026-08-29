//go:build windows

package main

import (
	"net"
	"syscall"
	"unsafe"

	"github.com/wailsapp/wails/v2/pkg/options/linux"
)

var (
	modUser32                    = syscall.NewLazyDLL("user32.dll")
	procFindWindowW              = modUser32.NewProc("FindWindowW")
	procShowWindow               = modUser32.NewProc("ShowWindow")
	procSetForegroundWindow      = modUser32.NewProc("SetForegroundWindow")
	procGetWindowThreadProcessId = modUser32.NewProc("GetWindowThreadProcessId")
	procAllowSetForegroundWindow = modUser32.NewProc("AllowSetForegroundWindow")
	procPostMessageW             = modUser32.NewProc("PostMessageW")
)

const wmSetFocus = 0x0007 // WM_SETFOCUS

// isMacOS は macOS 固有のメニュー構成（標準アプリメニューの付与等）を切り替えるための定数。
const isMacOS = false

func verifyPeer(_ *net.UnixConn) bool { return true }

// platformPrint は macOS 専用のネイティブ印刷実装。この OS では未処理（false）を返し、
// 呼び出し側が Wails の WindowPrint（WebView 内で window.print() を実行）へフォールバックする。
func platformPrint() bool { return false }

// platformSetMenuItemsEnabled は Linux 専用（Wails のメニュー更新が効かないため GTK 項目を直接更新する）。
// この OS では未処理（false）を返し、呼び出し側が Wails の MenuUpdateApplicationMenu を使う。
func platformSetMenuItemsEnabled(_ []string, _ bool) bool { return false }

// platformLinuxOptions は Linux 固有の Wails オプション（ウィンドウアイコン等）。この OS では nil。
func platformLinuxOptions() *linux.Options { return nil }

// registerDesktopIntegration は Linux 専用（.desktop / アイコンの自動登録）。この OS では何もしない。
func registerDesktopIntegration() {}

// findMainWindow はタイトル（appTitle）からメインウィンドウのハンドルを取得する。
// 見つからない場合は ok=false を返す。
func findMainWindow() (hwnd uintptr, ok bool) {
	titlePtr, err := syscall.UTF16PtrFromString(appTitle)
	if err != nil {
		return 0, false
	}
	hwnd, _, _ = procFindWindowW.Call(0, uintptr(unsafe.Pointer(titlePtr)))
	return hwnd, hwnd != 0
}

// platformGrantForeground は後発インスタンスが終了前に呼ぶ。
// 先発インスタンスの PID を特定し AllowSetForegroundWindow で許可を与えることで、
// 先発が SetForegroundWindow を成功させられるようにする。
func platformGrantForeground() {
	hwnd, ok := findMainWindow()
	if !ok {
		return
	}
	var pid uint32
	procGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	if pid == 0 {
		return
	}
	procAllowSetForegroundWindow.Call(uintptr(pid))
}

// platformRaiseWindow は先発インスタンスがウィンドウを前面に出すために呼ぶ（IPC 受信時）。
// 後発の AllowSetForegroundWindow で許可を受けた後に呼ぶことで SetForegroundWindow が成功する。
func platformRaiseWindow(_ *App) {
	hwnd, ok := findMainWindow()
	if !ok {
		return
	}
	const swRestore = 9
	procShowWindow.Call(hwnd, swRestore)
	procSetForegroundWindow.Call(hwnd)
}

// focusWebview はメインウィンドウへ WM_SETFOCUS を送り、WebView2 にキーボード
// フォーカスを渡す。Windows の WebView2 はコンテンツが子ウィンドウで動くため、
// 起動直後はクリックするまでキー入力が WebView に届かない。Wails は WM_SETFOCUS 受信時に
// chromium.Focus() を呼ぶ（winc wndproc → OnSetFocus）ので、メッセージを直接送って誘発する。
// ウィンドウが既にフォーカスを保持していても、メッセージ送信なので no-op にならない。
func focusWebview() {
	hwnd, ok := findMainWindow()
	if !ok {
		return
	}
	procPostMessageW.Call(hwnd, uintptr(wmSetFocus), 0, 0)
}
