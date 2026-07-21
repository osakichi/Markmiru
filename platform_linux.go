//go:build linux

package main

import (
	"bytes"
	_ "embed"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/wailsapp/wails/v2/pkg/options/linux"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.org/x/sys/unix"
)

// linuxIcon はアプリアイコン（1024x1024 PNG）。ウィンドウアイコン（platformLinuxOptions）と
// デスクトップ登録（registerDesktopIntegration）の両方で使う。Linux ビルドのみ埋め込む。
//
//go:embed build/appicon.png
var linuxIcon []byte

// platformLinuxOptions は Wails の Linux 固有オプションを返す（他 OS では nil）。
//   - Icon: ウィンドウ自体のアイコン（X11 での最小化表示等に使われる。GNOME のドックや
//     Alt+Tab のアイコンはここではなく .desktop 由来のため registerDesktopIntegration が担う）
//   - ProgramName: g_set_prgname を明示し、Wayland の app_id / X11 の WM_CLASS を
//     実行ファイル名に依存せず "Markmiru" に固定する（.desktop とのマッチングに使われるため、
//     バイナリをリネームして起動されてもアイコン解決が壊れない）
//   - WebviewGpuPolicy: Wails は options.Linux が nil のとき Never を既定にする
//     （wails issue #2977 対応）が、非 nil を渡すとゼロ値＝Always に変わってしまうため、
//     従来どおりの Never を明示して挙動を維持する
func platformLinuxOptions() *linux.Options {
	return &linux.Options{
		Icon:             linuxIcon,
		ProgramName:      appTitle,
		WebviewGpuPolicy: linux.WebviewGpuPolicyNever,
	}
}

// registerDesktopIntegration は GNOME 等のシェルがドック・Alt+Tab・アプリ一覧のアイコンを
// 解決できるよう、XDG データディレクトリ（既定 ~/.local/share）へ .desktop ファイルと
// アイコンを自動登録する。起動のたびに内容を照合し、変化があるときだけ書き込む
// （実行ファイルの移動にも次回起動で追従する）。
//
// 背景: Linux のシェルはウィンドウの app_id / WM_CLASS に一致する .desktop の Icon を
// 表示する。特に Wayland にはウィンドウ自体へアイコンを載せる仕組みが無く、.desktop の
// 登録が唯一の手段。バイナリ単体配布（tar.gz）の方針を保ったまま「展開して起動するだけ」で
// アイコンが正しく出るよう、インストーラではなくアプリ自身が登録する。
// 登録に失敗してもアプリ動作には支障がないため、エラーはすべて無視する（ベストエフォート）。
func registerDesktopIntegration() {
	dataHome := os.Getenv("XDG_DATA_HOME")
	if dataHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return
		}
		dataHome = filepath.Join(home, ".local", "share")
	}

	// アイコンは hicolor テーマの apps へ登録する。hicolor が認識する最大サイズディレクトリが
	// 512x512 のため（1024 は index.theme に無く無視される）、1024px の PNG を 512 へ置く
	// （GTK は読み込み時に指定サイズへ縮小するため、実サイズ超過は問題にならない）。
	writeIfChanged(filepath.Join(dataHome, "icons", "hicolor", "512x512", "apps", "Markmiru.png"), linuxIcon)

	exe, err := os.Executable()
	if err != nil {
		return
	}
	desktop := "[Desktop Entry]\n" +
		"Type=Application\n" +
		"Name=Markmiru\n" +
		"Comment=Markdown viewer/editor\n" +
		"Exec=" + desktopExecQuote(exe) + " %F\n" +
		"Icon=Markmiru\n" +
		"Terminal=false\n" +
		"Categories=Office;Viewer;\n" +
		"StartupWMClass=Markmiru\n"
	writeIfChanged(filepath.Join(dataHome, "applications", "Markmiru.desktop"), []byte(desktop))
}

// writeIfChanged は内容が現状と異なる場合のみファイルを書き込む（毎回の起動での無駄な
// 書き込みとシェルのファイル監視の空発火を避ける）。失敗は無視する（ベストエフォート）。
func writeIfChanged(path string, data []byte) {
	if cur, err := os.ReadFile(path); err == nil && bytes.Equal(cur, data) {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	_ = os.WriteFile(path, data, 0o644)
}

// desktopExecQuote は Desktop Entry Spec の Exec 値用に実行ファイルパスを引用する。
// ダブルクォートで囲み、予約文字（\ " ` $）をエスケープする。バックスラッシュは
// 引用規則と文字列一般規則の二重適用のため、ファイル上は 4 連（\ → \\\\）になる。
func desktopExecQuote(path string) string {
	r := strings.NewReplacer(
		`\`, `\\\\`,
		`"`, `\\"`,
		"`", "\\\\`",
		`$`, `\\$`,
	)
	return `"` + r.Replace(path) + `"`
}

// isMacOS は macOS 固有のメニュー構成（標準アプリメニューの付与等）を切り替えるための定数。
const isMacOS = false

func platformGrantForeground() {}
func focusWebview()            {}

// platformRaiseWindow は既に表示中のウィンドウを前面へ出す（IPC 受信時の bringToFront から呼ぶ）。
// Wails の WindowShow は Linux では gtk_widget_show のため既表示ウィンドウの前面化効果が無く、
// gtk_window_present を呼ぶ WindowUnminimise を使う（最小化解除＋前面化・フォーカス要求）。
// 【既知の制約】Wayland ではコンポジタのフォーカス奪取防止により即時前面化されない場合があり、
// その際はドックのアイコン強調（注目表示）となる（コンポジタ側の仕様で、アプリからは制御不可）。
func platformRaiseWindow(a *App) {
	if a.ctx != nil {
		runtime.WindowUnminimise(a.ctx)
	}
}

// platformPrint は macOS 専用のネイティブ印刷実装。この OS では未処理（false）を返し、
// 呼び出し側が Wails の WindowPrint（WebView 内で window.print() を実行）へフォールバックする。
func platformPrint() bool { return false }

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
	// 実行中にバイナリファイルが置き換えられると（再ビルド等）、/proc/<pid>/exe の
	// リンク先は「<path> (deleted)」になりパス比較が常に不一致→受け渡しが無言で失敗する。
	// 接尾辞を除いて同一パスの新旧バイナリ間の受け渡しを許容する（そのパスへ配置できるのは
	// 同一ユーザーだけなので、検証の趣旨〔別プログラムの排除〕は保たれる）。
	const deletedSuffix = " (deleted)"
	selfExe = strings.TrimSuffix(selfExe, deletedSuffix)
	peerExe = strings.TrimSuffix(peerExe, deletedSuffix)
	return filepath.Clean(selfExe) == filepath.Clean(peerExe)
}
