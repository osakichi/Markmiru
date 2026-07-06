//go:build !linux && !darwin && !windows

package main

import "net"

// 対象外プラットフォーム用スタブ。Linux / macOS / Windows 以外でのビルドに備える。

// isMacOS は macOS 固有のメニュー構成（標準アプリメニューの付与等）を切り替えるための定数。
const isMacOS = false

func setSocketPerms(_ string)         {}
func verifyPeer(_ *net.UnixConn) bool { return true }
func platformGrantForeground()        {}
func activateWindowWin32()            {}
func focusWebview()                   {}

// platformPrint は macOS 専用のネイティブ印刷実装。この OS では未処理（false）を返し、
// 呼び出し側が Wails の WindowPrint（WebView 内で window.print() を実行）へフォールバックする。
func platformPrint() bool { return false }
