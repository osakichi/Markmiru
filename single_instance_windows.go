//go:build windows

package main

import (
	"net"
	"syscall"
	"unsafe"
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

func setSocketPerms(_ string) {}
func verifyPeer(_ *net.UnixConn) bool { return true }

// platformGrantForeground は後発インスタンスが終了前に呼ぶ。
// 先発インスタンスの PID を特定し AllowSetForegroundWindow で許可を与えることで、
// 先発が SetForegroundWindow を成功させられるようにする。
func platformGrantForeground() {
	titlePtr, err := syscall.UTF16PtrFromString("Markmiru")
	if err != nil {
		return
	}
	hwnd, _, _ := procFindWindowW.Call(0, uintptr(unsafe.Pointer(titlePtr)))
	if hwnd == 0 {
		return
	}
	var pid uint32
	procGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	if pid == 0 {
		return
	}
	procAllowSetForegroundWindow.Call(uintptr(pid))
}

// activateWindowWin32 は先発インスタンスがウィンドウを前面に出すために呼ぶ。
// AllowSetForegroundWindow で許可を受けた後に呼ぶことで SetForegroundWindow が成功する。
func activateWindowWin32() {
	titlePtr, err := syscall.UTF16PtrFromString("Markmiru")
	if err != nil {
		return
	}
	hwnd, _, _ := procFindWindowW.Call(0, uintptr(unsafe.Pointer(titlePtr)))
	if hwnd == 0 {
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
	titlePtr, err := syscall.UTF16PtrFromString("Markmiru")
	if err != nil {
		return
	}
	hwnd, _, _ := procFindWindowW.Call(0, uintptr(unsafe.Pointer(titlePtr)))
	if hwnd == 0 {
		return
	}
	procPostMessageW.Call(hwnd, uintptr(wmSetFocus), 0, 0)
}
