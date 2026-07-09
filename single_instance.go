package main

import (
	"encoding/json"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// IPC 通信のセキュリティ制限値。
// ipcMaxPayload は ipcMaxPaths / ipcMaxPathLen を変更した場合に自動連動する。
const (
	ipcMaxPathLen = 4096     // 1 パスあたりの生バイト数上限（JSON デコード後。Linux PATH_MAX に準拠）
	ipcMaxPaths   = 64       // パス数上限
	// JSON エンコード後のペイロード上限。
	// バックスラッシュは JSON で \\ に倍増するため生バイスの最悪 2 倍を確保し、
	// 括弧・引用符・カンマ等の構造オーバーヘッド分として 1024 バイトを加算する。
	ipcMaxPayload = ipcMaxPaths*ipcMaxPathLen*2 + 1024
)

// ipcSocketPath はユーザー固有ディレクトリ内のソケットパスを返す。
// UserCacheDir（Win=%LocalAppData%, mac=~/Library/Caches, Linux=~/.cache）を使用し
// OS レベルで他ユーザーのアクセスを防ぐ。
func ipcSocketPath() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "Markmiru")
	// 0700: 所有者のみアクセス可（Unix/macOS で有効; Windows は %LocalAppData% の ACL に依存）
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return filepath.Join(dir, "ipc-c7f3e1b2.sock"), nil
}

// ensureSingleInstance は多重起動を防ぐ。
// 既存インスタンスが見つかった場合: ファイルパスを IPC で送信して true を返す（呼び元は os.Exit する）。
// 見つからない場合: IPC リスナーを起動して false を返す。
func ensureSingleInstance(app *App, args []string) bool {
	socketPath, err := ipcSocketPath()
	if err != nil {
		return false
	}

	// 既存インスタンスへの接続を試みる
	conn, err := net.DialTimeout("unix", socketPath, 500*time.Millisecond)
	if err == nil {
		defer conn.Close()
		// Windows: 先発インスタンスが SetForegroundWindow できるよう許可を与える。
		// AllowSetForegroundWindow は送信前（自プロセスがまだフォアグラウンドの間）に呼ぶ必要がある。
		platformGrantForeground()
		data, _ := json.Marshal(args)
		if len(data) <= ipcMaxPayload {
			_ = conn.SetWriteDeadline(time.Now().Add(time.Second))
			_, _ = conn.Write(data)
			// 先発インスタンスの処理完了（接続クローズ＝EOF）を待ってから終了する。
			// Linux のピア検証は /proc/<pid>/exe を参照するため、送信直後に exit すると
			// 検証時点で自プロセスが消えており受け渡しが破棄される（送信成功でも開かれない）。
			_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
			_, _ = conn.Read(make([]byte, 1))
		}
		return true
	}

	// 既存インスタンスなし（またはソケット残存）: 新規リスナーを起動
	_ = os.Remove(socketPath)
	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		return false
	}
	setSocketPerms(socketPath) // プラットフォーム固有の権限設定（Unix: 0600, Windows: no-op）

	go func() {
		defer ln.Close()
		defer os.Remove(socketPath)
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go handleIPCConn(app, c)
		}
	}()

	return false
}

func handleIPCConn(app *App, conn net.Conn) {
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))

	unixConn, ok := conn.(*net.UnixConn)
	if !ok {
		return
	}

	// ピア検証（プラットフォーム固有: Linux=UID+exe照合, macOS=UID照合, Windows=no-op）
	if !verifyPeer(unixConn) {
		return
	}

	var args []string
	if err := json.NewDecoder(io.LimitReader(conn, ipcMaxPayload)).Decode(&args); err != nil {
		return
	}

	for _, path := range validateIPCPaths(args) {
		app.openFileFromIPC(path)
	}
	app.bringToFront()
}

// validateIPCPaths は受信したパスリストを検証し、安全なものだけを返す。
// ipcMaxPathLen は JSON デコード後の生バイト数で比較する。
func validateIPCPaths(args []string) []string {
	if len(args) > ipcMaxPaths {
		args = args[:ipcMaxPaths]
	}
	out := make([]string, 0, len(args))
	for _, p := range args {
		if len(p) > ipcMaxPathLen {
			continue
		}
		// ヌルバイト禁止（パス偽装防止）
		if strings.ContainsRune(p, '\x00') {
			continue
		}
		if strings.TrimSpace(p) == "" {
			continue
		}
		out = append(out, p)
	}
	return out
}
