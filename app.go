package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App struct
type App struct {
	ctx          context.Context
	hasUnsaved   atomic.Bool
	quitting     atomic.Bool
	startupFiles []string   // コマンドライン引数のファイルパス（main から設定）
	pendingMu    sync.Mutex // pendingFiles / frontReady を保護
	pendingFiles []string   // フロント準備前に IPC 経由で届いたファイルパス
	frontReady   bool       // GetPendingFiles 呼出後に true になる（pendingMu で保護）
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// GetPendingFiles はフロントエンド初期化完了後に一度だけ呼ぶ。
// 起動引数（コマンドライン）＋ IPC 早期受信ファイルをまとめて返し、内部リストをクリアする。
// 呼出後は frontReady = true となり、以降の IPC ファイルはイベントで即時配信される。
func (a *App) GetPendingFiles() []string {
	a.pendingMu.Lock()
	defer a.pendingMu.Unlock()
	a.frontReady = true
	all := make([]string, 0, len(a.startupFiles)+len(a.pendingFiles))
	all = append(all, a.startupFiles...)
	all = append(all, a.pendingFiles...)
	a.startupFiles = nil
	a.pendingFiles = nil
	return all
}

// openFileFromIPC は IPC 経由で受け取ったパスをフロントへ渡す。
// フロント準備前に届いた場合はキューに積み、準備完了後（GetPendingFiles 呼出後）はイベントで即時配信する。
func (a *App) openFileFromIPC(path string) {
	a.pendingMu.Lock()
	if !a.frontReady {
		a.pendingFiles = append(a.pendingFiles, path)
		a.pendingMu.Unlock()
		return
	}
	a.pendingMu.Unlock()
	runtime.EventsEmit(a.ctx, "ipc:open-file", path)
}

// bringToFront はウィンドウを前面に表示する。IPC 受信時に呼ぶ。
// runtime.WindowShow で表示状態を復元した後、Windows では Win32 の
// SetForegroundWindow を呼ぶことでフォアグラウンドロックを越えて前面に出す。
func (a *App) bringToFront() {
	if a.ctx == nil {
		return
	}
	runtime.WindowShow(a.ctx)
	activateWindowWin32() // Windows: SetForegroundWindow; 他 OS: no-op
}

// emit はネイティブメニュー等からフロントへイベントを送る内部ヘルパ。
// 小文字始まりのためバインディングには公開されない。
func (a *App) emit(event string) {
	runtime.EventsEmit(a.ctx, event)
}

// SetDirtyState はフロントから未保存有無を通知する（終了時に確認ループを起動するか判断する）。
func (a *App) SetDirtyState(hasUnsaved bool) {
	a.hasUnsaved.Store(hasUnsaved)
}

// Quit はアプリを終了する（フロントの終了確認ループ完了後に呼ばれる）。
func (a *App) Quit() {
	a.quitting.Store(true)
	runtime.Quit(a.ctx)
}

// beforeClose はウィンドウを閉じる直前に呼ばれる。
// 未保存があればフロントの終了確認ループ（タブごとの3択）を起動し、いったん閉じるのを中止する。
// 設計: docs/アーキテクチャ・画面設計.md §5.3（タブごと確認方式）
func (a *App) beforeClose(ctx context.Context) bool {
	if a.quitting.Load() {
		return false
	}
	if !a.hasUnsaved.Load() {
		return false
	}
	a.emit("app:request-quit")
	return true
}

// FileDoc はファイルのパス・名前・親ディレクトリ・内容をまとめた DTO。
// パスの分解は Go の path/filepath に委ねる（OS 依存の区切りも正しく扱える）。
type FileDoc struct {
	Path    string `json:"path"`
	Name    string `json:"name"`
	Dir     string `json:"dir"`
	Content string `json:"content"`
}

func newFileDoc(path, content string) FileDoc {
	return FileDoc{
		Path:    path,
		Name:    filepath.Base(path),
		Dir:     filepath.Dir(path),
		Content: content,
	}
}

// markdownFilters はファイルダイアログのフィルタ。
var markdownFilters = []runtime.FileFilter{
	{DisplayName: "Markdown (*.md;*.markdown;*.mdown;*.txt)", Pattern: "*.md;*.markdown;*.mdown;*.txt"},
	{DisplayName: "すべてのファイル (*.*)", Pattern: "*.*"},
}

// OpenFiles はネイティブのファイル選択（複数可）を開き、選択ファイルを読み込んで返す。
// 読み込めなかったファイルはスキップする。
func (a *App) OpenFiles() ([]FileDoc, error) {
	paths, err := runtime.OpenMultipleFilesDialog(a.ctx, runtime.OpenDialogOptions{
		Title:   "ファイルを開く",
		Filters: markdownFilters,
	})
	if err != nil {
		return nil, err
	}
	docs := make([]FileDoc, 0, len(paths))
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		docs = append(docs, newFileDoc(p, string(data)))
	}
	return docs, nil
}

// ReadFile は指定パスを読み込み、FileDoc として返す（セッション復元・再読込用）。
// 相対パスは絶対パスに変換してから返す。これにより openFromDoc の重複チェックが正しく機能する。
func (a *App) ReadFile(path string) (FileDoc, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return FileDoc{}, err
	}
	data, err := os.ReadFile(absPath)
	if err != nil {
		return FileDoc{}, err
	}
	return newFileDoc(absPath, string(data)), nil
}

// SaveFileDialog は保存ダイアログを表示し、選択パスを返す（キャンセル時は空文字）。
func (a *App) SaveFileDialog(suggestedName string) (string, error) {
	return runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           "名前を付けて保存",
		DefaultFilename: suggestedName,
		Filters:         markdownFilters,
	})
}

// imageMaxBytes はローカル画像として読み込む最大サイズ（50 MB）。
const imageMaxBytes = 50 * 1024 * 1024

// imageMIME はファイル拡張子から MIME タイプを返す。
func imageMIME(ext string) string {
	switch strings.ToLower(ext) {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".svg":
		return "image/svg+xml"
	case ".bmp":
		return "image/bmp"
	case ".ico":
		return "image/x-icon"
	case ".avif":
		return "image/avif"
	default:
		return "application/octet-stream"
	}
}

// resolveImagePath は src を baseDir を基点とした絶対パスに解決する。
//   - 絶対パス（filepath.IsAbs == true, 例: C:\... /home/...）: そのまま使用
//   - ルート相対パス（\ または / 始まりでボリューム名なし, 例: \Users\... /Users/...）:
//     baseDir のボリューム名（Windows では "C:" 等, 他 OS では ""）を先頭に付与して絶対化
//   - 相対パス（例: ./img.png）: baseDir と結合
func resolveImagePath(baseDir, src string) string {
	if filepath.IsAbs(src) {
		return filepath.Clean(src)
	}
	if len(src) > 0 && os.IsPathSeparator(src[0]) {
		// ルート相対: ボリューム名（Windows: "C:" / 他 OS: ""）を補完する
		vol := filepath.VolumeName(baseDir)
		return filepath.Clean(vol + src)
	}
	return filepath.Clean(filepath.Join(baseDir, src))
}

// ReadImageAsDataURL はローカル画像ファイルを読み込み、data URI として返す。
// src が相対パスの場合は baseDir を基点に解決する。
// ファイルが存在しない場合は空文字を返す（エラーにしない）。
func (a *App) ReadImageAsDataURL(baseDir, src string) (string, error) {
	absPath := resolveImagePath(baseDir, src)

	info, err := os.Stat(absPath)
	if err != nil {
		return "", nil // ファイル不在は空文字で返す
	}
	if info.Size() > imageMaxBytes {
		return "", fmt.Errorf("image too large: %d bytes", info.Size())
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		return "", nil
	}

	mime := imageMIME(filepath.Ext(absPath))
	encoded := base64.StdEncoding.EncodeToString(data)
	return fmt.Sprintf("data:%s;base64,%s", mime, encoded), nil
}

// SaveFile は内容を指定パスへ UTF-8 で書き込み、保存後の FileDoc を返す。
func (a *App) SaveFile(path string, content string) (FileDoc, error) {
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return FileDoc{}, err
	}
	return newFileDoc(path, content), nil
}
