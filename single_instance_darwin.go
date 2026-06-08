//go:build darwin

package main

import (
	"net"
	"os"

	"golang.org/x/sys/unix"
)

// setSocketPerms はソケットファイルを所有者専用に制限する。
func setSocketPerms(path string) {
	_ = os.Chmod(path, 0o600)
}

func platformGrantForeground() {}
func activateWindowWin32()     {}
func focusWebview()            {}

// verifyPeer は接続元プロセスの実効 UID（EUID）を自プロセスと照合する。
// macOS では AF_UNIX 経由でピアの実行ファイルパスを cgo なしで取得することが困難なため、
// UID 照合のみ行う（同一ユーザーの別プログラムは弾けないが、他ユーザーの接続は防ぐ）。
func verifyPeer(conn *net.UnixConn) bool {
	rawConn, err := conn.SyscallConn()
	if err != nil {
		return false
	}

	var peerUID uint32
	var innerErr error

	_ = rawConn.Control(func(fd uintptr) {
		euid, _, err := unix.Getpeereid(int(fd))
		if err != nil {
			innerErr = err
			return
		}
		peerUID = euid
	})
	if innerErr != nil {
		return false
	}
	return peerUID == uint32(os.Geteuid())
}
