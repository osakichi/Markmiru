//go:build !linux && !darwin && !windows

package main

import "net"

// 対象外プラットフォーム用スタブ。Linux / macOS / Windows 以外でのビルドに備える。

func setSocketPerms(_ string)         {}
func verifyPeer(_ *net.UnixConn) bool { return true }
func platformGrantForeground()        {}
func activateWindowWin32()            {}
func focusWebview()                   {}
