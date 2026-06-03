//go:build windows

package main

import (
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

// singleInstanceMutex はプロセス終了まで保持し続ける必要があるためパッケージ変数に置く。
var singleInstanceMutex windows.Handle

// ensureSingleInstance は wails.Run より前に呼ぶことで、
// 起動直後に多重起動をブロックする。
func ensureSingleInstance() {
	var err error
	singleInstanceMutex, err = windows.CreateMutex(
		nil, false,
		windows.StringToUTF16Ptr("Markmiru-SingleInstance-c7f3e1b2"),
	)
	if err != windows.ERROR_ALREADY_EXISTS {
		return
	}
	bringExistingToFront()
	os.Exit(0)
}

func bringExistingToFront() {
	user32 := windows.NewLazySystemDLL("user32.dll")
	findWindowW := user32.NewProc("FindWindowW")
	showWindow := user32.NewProc("ShowWindow")
	setForegroundWindow := user32.NewProc("SetForegroundWindow")

	titlePtr, _ := windows.UTF16PtrFromString("Markmiru")
	hwnd, _, _ := findWindowW.Call(0, uintptr(unsafe.Pointer(titlePtr)))
	if hwnd == 0 {
		return
	}
	const swRestore = 9
	showWindow.Call(hwnd, swRestore)
	setForegroundWindow.Call(hwnd)
}
