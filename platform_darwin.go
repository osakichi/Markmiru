//go:build darwin

package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Foundation -framework Cocoa -framework WebKit

#import <AppKit/AppKit.h>
#import <WebKit/WebKit.h>
#include <stdbool.h>

// mmFindWKWebView は view 階層を深さ優先で辿り、最初に見つかった WKWebView を返す。
// Wails は WKWebView のポインタをアプリへ公開しないため、ウィンドウから探して取得する。
static WKWebView *mmFindWKWebView(NSView *view) {
	if ([view isKindOfClass:[WKWebView class]]) {
		return (WKWebView *)view;
	}
	for (NSView *sub in [view subviews]) {
		WKWebView *found = mmFindWKWebView(sub);
		if (found != nil) {
			return found;
		}
	}
	return nil;
}

// mmPrintWebView は WKWebView の内容を macOS ネイティブの印刷パネルで印刷する。
// Wails の WindowPrint 相当だが、(1) 用紙の向きのデフォルトを縦にする、
// (2) パネルに向き・用紙サイズ・拡大縮小のコントロールを表示する、
// (3) 用紙余白を Windows/Linux の既定と同等（約 1cm）にする、の 3 点が異なる
// （WindowPrint は横向き固定・余白 0 で、パネルにはそれらの選択肢が出ない）。
// WKWebView が見つからない場合は false を返す（呼び出し側が WindowPrint へフォールバック）。
// printOperationWithPrintInfo: は macOS 11 以降（本アプリのビルド要件＝SDK 11+ の範囲内）。
static bool mmPrintWebView(void) {
#if MAC_OS_X_VERSION_MAX_ALLOWED >= 110000
	if (@available(macOS 11.0, *)) {
		__block WKWebView *webView = nil;
		__block NSWindow *window = nil;
		void (^find)(void) = ^{
			for (NSWindow *w in [NSApp windows]) {
				WKWebView *v = mmFindWKWebView([w contentView]);
				if (v != nil) {
					webView = v;
					window = w;
					break;
				}
			}
		};
		// AppKit へのアクセスはメインスレッド必須。Go バインド呼び出しは別スレッドで届く。
		if ([NSThread isMainThread]) {
			find();
		} else {
			dispatch_sync(dispatch_get_main_queue(), find);
		}
		if (webView == nil) {
			return false;
		}
		// 印刷パネルはモーダル実行のため、Go 側を待たせないよう非同期で開く。
		dispatch_async(dispatch_get_main_queue(), ^{
			// 用紙余白は Windows/Linux（Chromium/WebKitGTK の既定 ≒ 1cm）に合わせる。
			// 印刷用 CSS（@media print）は padding を 0 にしており、余白はここだけが持つ。
			// 1cm = 72pt/2.54 ≒ 28.35pt。
			const CGFloat marginPt = 28.35;
			NSPrintInfo *pInfo = [NSPrintInfo sharedPrintInfo];
			pInfo.horizontalPagination = NSPrintingPaginationModeAutomatic;
			pInfo.verticalPagination = NSPrintingPaginationModeAutomatic;
			// 中央寄せはしない（Windows と同じく先頭ページ上寄せ）。
			pInfo.verticallyCentered = NO;
			pInfo.horizontallyCentered = NO;
			pInfo.orientation = NSPaperOrientationPortrait;
			pInfo.leftMargin = marginPt;
			pInfo.rightMargin = marginPt;
			pInfo.topMargin = marginPt;
			pInfo.bottomMargin = marginPt;

			NSPrintOperation *po = [webView printOperationWithPrintInfo:pInfo];
			po.showsPrintPanel = YES;
			po.showsProgressPanel = YES;
			po.printPanel.options = po.printPanel.options | NSPrintPanelShowsOrientation | NSPrintPanelShowsPaperSize | NSPrintPanelShowsScaling;
			po.view.frame = [webView bounds];
			[po runOperationModalForWindow:window delegate:nil didRunSelector:nil contextInfo:nil];
		});
		return true;
	}
#endif
	return false;
}
*/
import "C"

import (
	"net"
	"os"

	"github.com/wailsapp/wails/v2/pkg/options/linux"
	"golang.org/x/sys/unix"
)

// isMacOS は macOS 固有のメニュー構成（標準アプリメニューの付与等）を切り替えるための定数。
const isMacOS = true

// platformPrint は macOS ネイティブの印刷パネルを開く（縦向きデフォルト。向き・用紙サイズ・
// 拡大縮小はパネル上で変更可能）。WKWebView が見つからなければ false を返し、呼び出し側が
// Wails の WindowPrint（横向き固定）へフォールバックする。
func platformPrint() bool {
	return bool(C.mmPrintWebView())
}

// setSocketPerms はソケットファイルを所有者専用に制限する。
func setSocketPerms(path string) {
	_ = os.Chmod(path, 0o600)
}

func platformGrantForeground() {}
func activateWindowWin32()     {}
func focusWebview()            {}

// platformLinuxOptions は Linux 固有の Wails オプション（ウィンドウアイコン等）。この OS では nil。
func platformLinuxOptions() *linux.Options { return nil }

// registerDesktopIntegration は Linux 専用（.desktop / アイコンの自動登録）。この OS では何もしない。
func registerDesktopIntegration() {}

// verifyPeer は接続元プロセスの実効 UID（EUID）を自プロセスと照合する。
// macOS では AF_UNIX 経由でピアの実行ファイルパスを cgo なしで取得することが困難なため、
// UID 照合のみ行う（同一ユーザーの別プログラムは弾けないが、他ユーザーの接続は防ぐ）。
// ピア資格情報は getsockopt(SOL_LOCAL, LOCAL_PEERCRED) → Xucred で取得する
// （x/sys/unix に Getpeereid は存在しないため）。
func verifyPeer(conn *net.UnixConn) bool {
	rawConn, err := conn.SyscallConn()
	if err != nil {
		return false
	}

	var peerUID uint32
	var innerErr error

	_ = rawConn.Control(func(fd uintptr) {
		xucred, err := unix.GetsockoptXucred(int(fd), unix.SOL_LOCAL, unix.LOCAL_PEERCRED)
		if err != nil {
			innerErr = err
			return
		}
		peerUID = xucred.Uid
	})
	if innerErr != nil {
		return false
	}
	return peerUID == uint32(os.Geteuid())
}
