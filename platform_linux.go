//go:build linux

package main

import (
	"fmt"
	"net"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// isMacOS は macOS 固有のメニュー構成（標準アプリメニューの付与等）を切り替えるための定数。
const isMacOS = false

// setSocketPerms はソケットファイルを所有者専用に制限する。
// 0600: ファイル権限によるユーザー分離（ディレクトリの 0700 と合わせた二重防御）。
func setSocketPerms(path string) {
	_ = os.Chmod(path, 0o600)
}

func platformGrantForeground() {}
func activateWindowWin32()     {}
func focusWebview()            {}

// verifyPeer は接続元プロセスの UID と実行ファイルパスを照合する。
//   - SO_PEERCRED で UID を取得し自プロセスの UID と一致を確認（カーネル保証）
//   - /proc/<pid>/exe で実行ファイルパスを照合（同一ユーザーの別プログラムを弾く）
func verifyPeer(conn *net.UnixConn) bool {
	rawConn, err := conn.SyscallConn()
	if err != nil {
		return false
	}

	var peerUID uint32
	var peerPID int32
	var innerErr error

	_ = rawConn.Control(func(fd uintptr) {
		cred, err := unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
		if err != nil {
			innerErr = err
			return
		}
		peerUID = cred.Uid
		peerPID = cred.Pid
	})
	if innerErr != nil || peerUID != uint32(os.Getuid()) {
		return false
	}

	selfExe, err := os.Executable()
	if err != nil {
		return false
	}
	peerExe, err := os.Readlink(fmt.Sprintf("/proc/%d/exe", peerPID))
	if err != nil {
		return false
	}
	return filepath.Clean(selfExe) == filepath.Clean(peerExe)
}
