package web

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
)

func testState() *State {
	st := NewState()
	st.AddTab("/docs/a.md", "a.md", "# Hello\n\nbody text") // t1（自動アクティブ）
	st.AddTab("", "untitled", "## Second heading")          // t2
	return st
}

func do(h http.Handler, method, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
	return rec
}

// fakeHost は OS ダイアログ・書込のテスト用ダミー。
type fakeHost struct {
	openFiles       []OpenedFile
	savePath        string
	written         map[string]string
	readme          string
	license         string
	exportPath      string
	importContent   string
	editMenuEnabled bool              // SetEditMenuEnabled の最後の値
	saveMenuEnabled bool              // SetSaveMenuEnabled の最後の値
	modeMenuEnabled bool              // SetModeMenuEnabled の最後の値
	quitCalled      bool              // Quit が呼ばれたか
	readFiles       map[string]string // ReadFile が返す内容（path→content）。無ければ ok=false
	openedURL       string            // OpenURL に渡された最後の URL
	clipboard       string            // ClipboardText が返すテキスト
	clipboardErr    error             // ClipboardText が返すエラー
}

func (h *fakeHost) OpenFilesDialog() ([]OpenedFile, error) { return h.openFiles, nil }
func (h *fakeHost) SaveFileDialog(string) (string, error)  { return h.savePath, nil }
func (h *fakeHost) WriteFile(path, content string) error {
	if h.written == nil {
		h.written = map[string]string{}
	}
	h.written[path] = content
	return nil
}
func (h *fakeHost) ReadmeMarkdown() string                   { return h.readme }
func (h *fakeHost) LicenseMarkdown() string                  { return h.license }
func (h *fakeHost) ExportStyleDialog(string) (string, error) { return h.exportPath, nil }
func (h *fakeHost) ImportStyleDialog() (string, error)       { return h.importContent, nil }
func (h *fakeHost) SetEditMenuEnabled(canEdit bool)          { h.editMenuEnabled = canEdit }
func (h *fakeHost) SetSaveMenuEnabled(canSave bool)          { h.saveMenuEnabled = canSave }
func (h *fakeHost) SetModeMenuEnabled(canToggle bool)        { h.modeMenuEnabled = canToggle }
func (h *fakeHost) Quit()                                    { h.quitCalled = true }
func (h *fakeHost) ReadFile(path string) (string, string, bool) {
	content, ok := h.readFiles[path]
	if !ok {
		return "", "", false
	}
	return filepath.Base(path), content, true
}
func (h *fakeHost) OpenURL(url string) { h.openedURL = url }

func (h *fakeHost) ClipboardText() (string, error) { return h.clipboard, h.clipboardErr }

// newH は host 不要のテスト向けに空の fakeHost で Handler を作る。
func newH(st *State) http.Handler { return NewHandler(st, &fakeHost{}) }

// assertNavKeep は応答が navkeep（本文差し替えなし・シェル OOB のみ）であることを検証する:
// HX-Reswap: none、本文断片を含まない、タブバー/サイドバー/ダイアログ host の OOB を含む。
func assertNavKeep(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	if rec.Header().Get("HX-Reswap") != "none" {
		t.Errorf("response should carry HX-Reswap: none (keep the view untouched)")
	}
	body := rec.Body.String()
	if strings.Contains(body, "preview-scroll") || strings.Contains(body, "editor-input") {
		t.Errorf("navkeep response must not contain a content fragment: %s", body)
	}
	if !strings.Contains(body, `id="tabbar"`) || !strings.Contains(body, `id="sidebar"`) || !strings.Contains(body, `id="dialog-host"`) {
		t.Errorf("navkeep response should OOB-update tabbar/sidebar/dialog-host: %s", body)
	}
}

func TestServeShell(t *testing.T) {
	rec := do(newH(testState()), "GET", "/")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("content-type = %q", ct)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`id="app"`, `id="tabbar"`, `id="content"`, `id="sidebar"`, `id="styleblock"`, `id="dialog-host"`,
		`/assets/htmx.min.js`, `/assets/mermaid.min.js`, `/assets/glue.js`, `/assets/markdown.css`,
		`<title>Markmiru</title>`, `data-scheme="light"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("shell missing %q", want)
		}
	}
	// アクティブスタイル（light）の CSS 変数が #styleblock に注入されている。
	if !strings.Contains(body, "#content{") || !strings.Contains(body, "--md-font-size:16px") {
		t.Errorf("style vars not injected: %s", body)
	}
	// コードハイライト CSS（chroma）が注入されている。
	if !strings.Contains(body, ".chroma") {
		t.Errorf("code theme CSS not injected")
	}
	// タブが状態から描画されている。
	if !strings.Contains(body, "a.md") || !strings.Contains(body, "untitled") {
		t.Errorf("tab names not rendered: %s", body)
	}
	// アクティブ（t1=a.md）の本文が描画されている（preview-scroll＋見出しスラッグ id）。
	if !strings.Contains(body, `class="preview-scroll"`) || !strings.Contains(body, `id="hello"`) || !strings.Contains(body, "Hello") {
		t.Errorf("active content not rendered: %s", body)
	}
	// 非アクティブ t2 の本文はシェルには出ない。
	if strings.Contains(body, "Second heading") {
		t.Errorf("inactive tab content should not be in shell")
	}
}

func TestViewEndpoint(t *testing.T) {
	rec := do(newH(testState()), "GET", "/tabs/t2/view")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `class="markdown-body"`) {
		t.Errorf("view fragment missing markdown-body: %s", body)
	}
	if !strings.Contains(body, "Second heading") || !strings.Contains(body, `id="second-heading"`) {
		t.Errorf("t2 content not rendered: %s", body)
	}
}

func TestViewUnknownTab(t *testing.T) {
	rec := do(newH(testState()), "GET", "/tabs/nope/view")
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestActivateEndpoint(t *testing.T) {
	st := testState()
	rec := do(newH(st), "POST", "/tabs/t2/activate")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	// 本文は新アクティブ（t2）。
	if !strings.Contains(body, "Second heading") {
		t.Errorf("activated content not rendered: %s", body)
	}
	// タブバーを OOB で差し替え、t2 を active に。
	if !strings.Contains(body, `hx-swap-oob="true"`) {
		t.Errorf("activate should include OOB tabbar: %s", body)
	}
	// 状態が更新されている。
	if rr := st.RenderReqActive(); !strings.Contains(rr.Content, "Second heading") {
		t.Errorf("active tab not switched in state")
	}
}

func TestActivateUnknownTab(t *testing.T) {
	rec := do(newH(testState()), "POST", "/tabs/zzz/activate")
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestEmptyStateShell(t *testing.T) {
	rec := do(newH(NewState()), "GET", "/")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	// タブ無しでもシェルは描画される。
	if !strings.Contains(rec.Body.String(), `id="content"`) {
		t.Errorf("empty shell should still render regions")
	}
}

func remoteState() (*State, string) {
	st := NewState()
	tab := st.AddTab("/docs/r.md", "r.md", "# Pic\n\n![x](https://example.com/i.png)\n")
	return st, tab.ID
}

func TestRemoteImagePromptInShell(t *testing.T) {
	st, id := remoteState()
	body := do(newH(st), "GET", "/").Body.String()
	// 既定で遮断（プレースホルダ）かつ確認ダイアログが #dialog-host に出る。
	if !strings.Contains(body, "data-remote-blocked") {
		t.Errorf("remote image should be blocked by default")
	}
	if !strings.Contains(body, "外部画像が含まれています") || !strings.Contains(body, "r.md") {
		t.Errorf("remote-image dialog not shown: %s", body)
	}
	if !strings.Contains(body, "/tabs/"+id+"/remote-images?choice=allow") {
		t.Errorf("dialog missing allow action")
	}
}

func TestRemoteImageAllow(t *testing.T) {
	st, id := remoteState()
	rec := do(newH(st), "POST", "/tabs/"+id+"/remote-images?choice=allow")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `src="https://example.com/i.png"`) {
		t.Errorf("allowed remote image should keep src: %s", body)
	}
	// ダイアログ host を OOB で空に（クリア）。
	if !strings.Contains(body, `id="dialog-host" hx-swap-oob="true"`) || strings.Contains(body, "外部画像が含まれています") {
		t.Errorf("dialog should be cleared after allow: %s", body)
	}
	// 以後、再描画してもダイアログは出ない（ポリシー確定）。
	if strings.Contains(do(newH(st), "GET", "/tabs/"+id+"/view").Body.String(), "外部画像が含まれています") {
		t.Errorf("dialog should not reappear once policy set")
	}
}

func TestRemoteImageBlock(t *testing.T) {
	st, id := remoteState()
	body := do(newH(st), "POST", "/tabs/"+id+"/remote-images?choice=block").Body.String()
	// 実 src 属性（先頭スペース付き）でリモート URL を指していないこと（data-blocked-src は退避用で許容）。
	if strings.Contains(body, ` src="https://example.com/i.png"`) {
		t.Errorf("blocked remote image must not have live src")
	}
	if !strings.Contains(body, "data-remote-blocked") {
		t.Errorf("blocked placeholder expected")
	}
	if strings.Contains(body, "外部画像が含まれています") {
		t.Errorf("dialog should be cleared after block")
	}
}

func TestRemoteImageUnknownTab(t *testing.T) {
	st, _ := remoteState()
	rec := do(newH(st), "POST", "/tabs/nope/remote-images?choice=allow")
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestSidebarRenderedInShell(t *testing.T) {
	body := do(newH(testState()), "GET", "/").Body.String()
	if !strings.Contains(body, "開いているファイル") {
		t.Errorf("sidebar title missing")
	}
	if !strings.Contains(body, `class="file-item`) || !strings.Contains(body, "a.md") {
		t.Errorf("sidebar file list missing: %s", body)
	}
	// タブバーにトグルと新規ボタン。
	if !strings.Contains(body, `hx-post="/sidebar/toggle"`) || !strings.Contains(body, `hx-post="/tabs/new"`) {
		t.Errorf("tabbar controls missing")
	}
	// タブに閉じるボタン。
	if !strings.Contains(body, `hx-post="/tabs/t1/close"`) {
		t.Errorf("tab close control missing")
	}
}

func TestSidebarToggle(t *testing.T) {
	st := testState() // 既定で開
	rec := do(newH(st), "POST", "/sidebar/toggle")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `id="sidebar" hidden`) {
		t.Errorf("toggle should hide sidebar: %s", rec.Body.String())
	}
	if st.SidebarOpen() {
		t.Errorf("state should be closed after toggle")
	}
	// 再トグルで開く。
	body2 := do(newH(st), "POST", "/sidebar/toggle").Body.String()
	if strings.Contains(body2, "hidden") {
		t.Errorf("second toggle should reopen")
	}
}

func TestNewTab(t *testing.T) {
	st := testState()
	rec := do(newH(st), "POST", "/tabs/new")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if len(st.TabVMs()) != 3 {
		t.Errorf("expected 3 tabs after new, got %d", len(st.TabVMs()))
	}
	// 新規タブがアクティブ。
	last := st.TabVMs()[2]
	if !last.Active || last.FileName != "無題" {
		t.Errorf("new tab should be active untitled: %+v", last)
	}
}

func TestCloseTab(t *testing.T) {
	st := testState() // t1(active), t2
	rec := do(newH(st), "POST", "/tabs/t1/close")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	vms := st.TabVMs()
	if len(vms) != 1 || vms[0].ID != "t2" {
		t.Errorf("t1 should be closed leaving t2: %+v", vms)
	}
	// アクティブを閉じたので t2 がアクティブに。
	if !vms[0].Active {
		t.Errorf("t2 should become active after closing active t1")
	}
}

// dirtyTab は編集済み（未保存）タブを1つ持つ State を返す。
func dirtyTab(path string) (*State, string) {
	st := NewState()
	tab := st.AddTab(path, "a.md", "original")
	st.SetMode(tab.ID)                    // source
	st.UpdateContent(tab.ID, "edited!!!") // dirty に
	return st, tab.ID
}

func TestCloseDirtyShowsDialog(t *testing.T) {
	st, id := dirtyTab("/d/a.md")
	rec := do(newH(st), "POST", "/tabs/"+id+"/close")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	// 本文ではなく #dialog-host に差し替える（HX-Retarget）。
	if rec.Header().Get("HX-Retarget") != "#dialog-host" {
		t.Errorf("close of dirty tab should retarget dialog-host, got %q", rec.Header().Get("HX-Retarget"))
	}
	body := rec.Body.String()
	if !strings.Contains(body, "未保存の変更") || !strings.Contains(body, "a.md") {
		t.Errorf("close-unsaved dialog not shown: %s", body)
	}
	// 3択の action が揃っている。破棄は初期 disabled（キージャック対策）。
	for _, want := range []string{"choice=save", "choice=discard", "choice=cancel", "data-dialog-accept disabled"} {
		if !strings.Contains(body, want) {
			t.Errorf("dialog missing %q: %s", want, body)
		}
	}
	// タブはまだ閉じていない。
	if len(st.TabVMs()) != 1 {
		t.Errorf("tab should not be closed while dialog is shown")
	}
}

func TestEditedThenRevertedStaysDirty(t *testing.T) {
	st := NewState()
	tab := st.AddTab("/d/a.md", "a.md", "A")
	st.SetMode(tab.ID)
	st.UpdateContent(tab.ID, "Az") // z を追加
	st.UpdateContent(tab.ID, "A")  // z を削除（内容は元通り）
	if !st.IsDirty(tab.ID) {
		t.Errorf("once edited, tab should stay dirty even when content matches the original")
	}
	// クローズ時も確認ダイアログを出す。
	rec := do(newH(st), "POST", "/tabs/"+tab.ID+"/close")
	if rec.Header().Get("HX-Retarget") != "#dialog-host" {
		t.Errorf("edited-then-reverted tab should still prompt on close")
	}
	// 保存すると clean に戻る（書き込んだスナップショット＝現在値なら）。
	st.MarkSaved(tab.ID, "/d/a.md", "A")
	if st.IsDirty(tab.ID) {
		t.Errorf("after save, tab should be clean again")
	}
}

// 書き込み I/O 中にさらに編集が届いた場合、MarkSaved は「書き込んだスナップショット」を
// 基準にするため dirty のまま残る（取りこぼした編集の clean 誤認＝サイレント消失を防ぐ）。
func TestMarkSavedKeepsDirtyWhenEditedDuringWrite(t *testing.T) {
	st := NewState()
	tab := st.AddTab("/d/a.md", "a.md", "A")
	st.SetMode(tab.ID)
	st.UpdateContent(tab.ID, "A!")  // 保存対象のスナップショット
	st.UpdateContent(tab.ID, "A!!") // WriteFile 中に届いた編集（未書き込み）
	st.MarkSaved(tab.ID, "/d/a.md", "A!")
	if !st.IsDirty(tab.ID) {
		t.Errorf("edit arriving during write must keep the tab dirty")
	}
	if st.HasUnsaved() != true {
		t.Errorf("quit confirmation must still trigger for the unwritten edit")
	}
}

func TestCloseDirtyDiscard(t *testing.T) {
	st, id := dirtyTab("/d/a.md")
	do(newH(st), "POST", "/tabs/"+id+"/close?choice=discard")
	if len(st.TabVMs()) != 0 {
		t.Errorf("discard should close the tab")
	}
}

func TestCloseDirtyCancel(t *testing.T) {
	st, id := dirtyTab("/d/a.md")
	rec := do(newH(st), "POST", "/tabs/"+id+"/close?choice=cancel")
	if len(st.TabVMs()) != 1 {
		t.Errorf("cancel should keep the tab")
	}
	// キャンセルは本文に触れず（閲覧位置・キャレット保持）、ダイアログだけ OOB で閉じる。
	assertNavKeep(t, rec)
}

// ダイアログ表示中の保存（Ctrl+S）で、前提が消えた確認ダイアログが OOB で閉じられる。
func TestSaveClearsOpenDialog(t *testing.T) {
	st, id := dirtyTab("/d/a.md")
	host := &fakeHost{}
	h := NewHandler(st, host)
	do(h, "POST", "/tabs/"+id+"/close") // 未保存確認ダイアログを開く
	body := do(h, "POST", "/tabs/"+id+"/save").Body.String()
	if host.written["/d/a.md"] != "edited!!!" {
		t.Errorf("save should write while dialog is open: %+v", host.written)
	}
	if !strings.Contains(body, `<div id="dialog-host" hx-swap-oob="true"></div>`) {
		t.Errorf("save response should OOB-clear the now-false dialog: %s", body)
	}
}

// ダイアログ表示中に別経路で保存済み（clean）になったタブへの「保存して閉じる」は、
// 再書き込みせずに閉じる（外部編集を古い内容で潰さない）。
func TestCloseSaveSkipsWriteWhenAlreadyClean(t *testing.T) {
	st, id := dirtyTab("/d/a.md")
	host := &fakeHost{}
	h := NewHandler(st, host)
	do(h, "POST", "/tabs/"+id+"/close") // ダイアログ表示
	do(h, "POST", "/tabs/"+id+"/save")  // 別経路の保存で clean 化
	host.written["/d/a.md"] = "SENTINEL"
	do(h, "POST", "/tabs/"+id+"/close?choice=save")
	if host.written["/d/a.md"] != "SENTINEL" {
		t.Errorf("clean tab must not be rewritten by close-save")
	}
	if len(st.TabVMs()) != 0 {
		t.Errorf("close-save on clean tab should still close it")
	}
}

func TestCloseDirtySaveWritesAndCloses(t *testing.T) {
	st, id := dirtyTab("/d/a.md")
	host := &fakeHost{}
	do(NewHandler(st, host), "POST", "/tabs/"+id+"/close?choice=save")
	if host.written["/d/a.md"] != "edited!!!" {
		t.Errorf("save should write edited content, got %+v", host.written)
	}
	if len(st.TabVMs()) != 0 {
		t.Errorf("save should close the tab after writing")
	}
}

func TestCloseDirtyUntitledSaveCancelKeepsTab(t *testing.T) {
	st := NewState()
	tab := st.AddTab("", "無題", "original")
	st.SetMode(tab.ID)
	st.UpdateContent(tab.ID, "edited")
	host := &fakeHost{savePath: ""} // 保存先選択をキャンセル
	do(NewHandler(st, host), "POST", "/tabs/"+tab.ID+"/close?choice=save")
	if len(host.written) != 0 {
		t.Errorf("cancelled save must not write")
	}
	if len(st.TabVMs()) != 1 {
		t.Errorf("cancelled save should keep the tab open")
	}
}

func TestCloseCleanTabImmediately(t *testing.T) {
	st := NewState()
	tab := st.AddTab("/d/a.md", "a.md", "clean") // 未編集
	rec := do(newH(st), "POST", "/tabs/"+tab.ID+"/close")
	if rec.Header().Get("HX-Retarget") != "" {
		t.Errorf("clean tab close should not show a dialog")
	}
	if len(st.TabVMs()) != 0 {
		t.Errorf("clean tab should close immediately")
	}
}

func TestCloseUnknownTab(t *testing.T) {
	rec := do(newH(testState()), "POST", "/tabs/zzz/close")
	if rec.Code != http.StatusNotFound {
		t.Errorf("status %d, want 404", rec.Code)
	}
}

func TestOpenFiles(t *testing.T) {
	st := NewState() // 空
	host := &fakeHost{openFiles: []OpenedFile{
		{Path: "/d/x.md", Name: "x.md", Content: "# X"},
		{Path: "/d/y.md", Name: "y.md", Content: "# Y"},
	}}
	do(NewHandler(st, host), "POST", "/tabs/open")
	vms := st.TabVMs()
	if len(vms) != 2 || vms[0].FileName != "x.md" || vms[1].FileName != "y.md" {
		t.Errorf("opened tabs unexpected: %+v", vms)
	}
	if !vms[1].Active {
		t.Errorf("last opened tab should be active")
	}
}

func TestOpenCancel(t *testing.T) {
	st := testState()
	do(NewHandler(st, &fakeHost{openFiles: nil}), "POST", "/tabs/open")
	if len(st.TabVMs()) != 2 {
		t.Errorf("cancel should not change tabs (got %d)", len(st.TabVMs()))
	}
}

func TestOpenPathAddsAndActivates(t *testing.T) {
	st := NewState()
	st.AddTab("/d/a.md", "a.md", "A")
	host := &fakeHost{readFiles: map[string]string{"/d/b.md": "# B"}}
	do(NewHandler(st, host), "POST", "/tabs/open-path?path=/d/b.md")
	vms := st.TabVMs()
	if len(vms) != 2 || vms[1].FileName != "b.md" || !vms[1].Active {
		t.Errorf("open-path should add and activate the file: %+v", vms)
	}
}

func TestOpenPathActivatesExisting(t *testing.T) {
	st := NewState()
	st.AddTab("/d/a.md", "a.md", "A")
	st.AddTab("/d/b.md", "b.md", "B") // b がアクティブ
	host := &fakeHost{readFiles: map[string]string{"/d/a.md": "A"}}
	do(NewHandler(st, host), "POST", "/tabs/open-path?path=/d/a.md")
	if len(st.TabVMs()) != 2 {
		t.Errorf("open-path of already-open file must not duplicate")
	}
	if st.ActiveID() == "" || st.TabVMs()[0].FileName != "a.md" || !st.TabVMs()[0].Active {
		t.Errorf("open-path should re-activate the existing tab: %+v", st.TabVMs())
	}
}

func TestOpenPathMissingIgnored(t *testing.T) {
	st := NewState()
	st.AddTab("/d/a.md", "a.md", "A")
	host := &fakeHost{} // readFiles nil → 読めない
	do(NewHandler(st, host), "POST", "/tabs/open-path?path=/d/gone.md")
	if len(st.TabVMs()) != 1 {
		t.Errorf("unreadable path should not add a tab")
	}
}

func TestSaveExistingPath(t *testing.T) {
	st, id := dirtyTab("/docs/a.md")
	host := &fakeHost{}
	rec := do(NewHandler(st, host), "POST", "/tabs/"+id+"/save")
	if host.written["/docs/a.md"] != "edited!!!" {
		t.Errorf("content not written to existing path: %+v", host.written)
	}
	if st.IsDirty(id) {
		t.Errorf("tab should be clean after save")
	}
	// 上書き保存は本文を差し替えない＝閲覧のスクロール位置・編集キャレットを保つ。
	assertNavKeep(t, rec)
}

// メニュー「保存」はアクティブタブの dirty 状態に同期する（編集で有効化・保存で再び無効化）。
func TestSaveMenuSyncsWithDirty(t *testing.T) {
	st := NewState()
	host := &fakeHost{}
	h := NewHandler(st, host)
	tab := st.AddTab("/d/a.md", "a.md", "A")
	st.SetMode(tab.ID)
	// 未編集のうちは無効。
	do(h, "GET", "/")
	if host.saveMenuEnabled {
		t.Errorf("save menu should be disabled while clean")
	}
	// 編集で dirty → 有効。
	do(h, "POST", "/tabs/"+tab.ID+"/content?value=A!")
	if !host.saveMenuEnabled {
		t.Errorf("save menu should be enabled when dirty")
	}
	// 保存で clean → 再び無効。
	do(h, "POST", "/tabs/"+tab.ID+"/save")
	if host.saveMenuEnabled {
		t.Errorf("save menu should be disabled again after save")
	}
}

// 「閲覧/編集切替」メニューは読み取り専用タブで無効、通常タブに戻れば再び有効。
func TestModeMenuDisabledForReadOnly(t *testing.T) {
	st := NewState()
	host := &fakeHost{readme: "# readme"}
	h := NewHandler(st, host)
	do(h, "POST", "/doc/about") // 読み取り専用タブがアクティブ → 無効を通知
	if host.modeMenuEnabled {
		t.Errorf("read-only tab should disable the mode toggle menu")
	}
	do(h, "POST", "/tabs/new") // 通常（無題）タブへ → 有効を通知
	if !host.modeMenuEnabled {
		t.Errorf("normal tab should re-enable the mode toggle menu")
	}
}

// 無題タブは未編集でも「保存」有効（初回保存＝保存ダイアログに繋がる）。読み取り専用タブは無効。
func TestSaveMenuEnabledForUntitledDisabledForReadOnly(t *testing.T) {
	st := NewState()
	host := &fakeHost{readme: "# readme"}
	h := NewHandler(st, host)
	do(h, "POST", "/tabs/new") // 無題（クリーン）がアクティブ
	if !host.saveMenuEnabled {
		t.Errorf("untitled tab should enable the save menu")
	}
	do(h, "POST", "/doc/about") // 読み取り専用タブがアクティブ
	if host.saveMenuEnabled {
		t.Errorf("read-only tab should disable the save menu")
	}
}

// 未変更（dirty でない）タブへの「保存」はファイルへ書き込まない（完全 no-op）。
// 外部エディタでの編集を、開いた時点の古い内容で上書きする事故を防ぐ。
func TestSaveCleanTabDoesNotWrite(t *testing.T) {
	st := NewState()
	tab := st.AddTab("/docs/a.md", "a.md", "hello") // 未編集
	host := &fakeHost{}
	rec := do(NewHandler(st, host), "POST", "/tabs/"+tab.ID+"/save")
	if len(host.written) != 0 {
		t.Errorf("clean tab save must not write: %+v", host.written)
	}
	assertNavKeep(t, rec)
}

func TestSaveUntitledUsesDialog(t *testing.T) {
	st := NewState()
	tab := st.AddTab("", "無題", "draft")
	host := &fakeHost{savePath: "/chosen/note.md"}
	rec := do(NewHandler(st, host), "POST", "/tabs/"+tab.ID+"/save")
	if host.written["/chosen/note.md"] != "draft" {
		t.Errorf("untitled save should write to dialog path: %+v", host.written)
	}
	if p, n, _, _ := st.SaveInfo(tab.ID); p != "/chosen/note.md" || n != "note.md" {
		t.Errorf("tab not updated after save-as: path=%q name=%q", p, n)
	}
	// 保存先が決まった＝BaseDir が変わるため、こちらは本文を再描画する（通常の nav）。
	if rec.Header().Get("HX-Reswap") != "" {
		t.Errorf("first save of untitled should rerender content")
	}
}

// 編集モード中の初回保存（無題→保存先決定）は本文を再描画しない＝キャレット・スクロールを
// 保つ（BaseDir 変更の再描画が必要なのは閲覧モードのみ。タブ名はタブバー OOB が反映）。
func TestSaveUntitledInSourceModeKeepsCaret(t *testing.T) {
	st := NewState()
	tab := st.AddTab("", "無題", "draft")
	st.SetMode(tab.ID) // source
	host := &fakeHost{savePath: "/chosen/note.md"}
	rec := do(NewHandler(st, host), "POST", "/tabs/"+tab.ID+"/save")
	if host.written["/chosen/note.md"] != "draft" {
		t.Errorf("untitled save should write: %+v", host.written)
	}
	assertNavKeep(t, rec)
	if !strings.Contains(rec.Body.String(), "note.md") {
		t.Errorf("tabbar OOB should carry the new file name: %s", rec.Body.String())
	}
}

func TestSaveUntitledCancel(t *testing.T) {
	st := NewState()
	tab := st.AddTab("", "無題", "draft")
	host := &fakeHost{savePath: ""} // キャンセル
	rec := do(NewHandler(st, host), "POST", "/tabs/"+tab.ID+"/save")
	if len(host.written) != 0 {
		t.Errorf("cancel should not write")
	}
	assertNavKeep(t, rec)
}

func TestOpenAboutAndLicense(t *testing.T) {
	st := NewState()
	host := &fakeHost{readme: "# README本文", license: "# ライセンス本文"}
	h := NewHandler(st, host)

	body := do(h, "POST", "/doc/about").Body.String()
	if !strings.Contains(body, "README") {
		t.Errorf("about content not rendered: %s", body)
	}
	if len(st.TabVMs()) != 1 || st.TabVMs()[0].FileName != "Markmiru について" {
		t.Errorf("about tab not opened: %+v", st.TabVMs())
	}
	// 再度開いても重複タブを作らない（既存を再アクティブ化）。
	do(h, "POST", "/doc/about")
	if len(st.TabVMs()) != 1 {
		t.Errorf("about should reuse existing tab, got %d", len(st.TabVMs()))
	}
	// ライセンスは別タブ。
	do(h, "POST", "/doc/license")
	if len(st.TabVMs()) != 2 {
		t.Errorf("license should open a second tab, got %d", len(st.TabVMs()))
	}
}

func TestSettingsToggleAndStyleList(t *testing.T) {
	st := testState()
	// シェルに設定モーダル（既定 hidden）とスタイル選択肢。
	shell := do(newH(st), "GET", "/").Body.String()
	if !strings.Contains(shell, `id="settings" hidden`) {
		t.Errorf("settings drawer should be hidden by default")
	}
	if !strings.Contains(shell, "ライト (プリセット)") || !strings.Contains(shell, `<option value="dark"`) {
		t.Errorf("style selector missing presets: %s", shell)
	}
	if !strings.Contains(shell, `hx-post="/settings/style"`) {
		t.Errorf("style pulldown should post to /settings/style: %s", shell)
	}
	// トグルで開く。
	body := do(newH(st), "POST", "/settings/toggle").Body.String()
	if strings.Contains(body, "hidden") {
		t.Errorf("toggle should open settings")
	}
	if !st.SettingsOpen() {
		t.Errorf("state should be open after toggle")
	}
}

func TestSettingsNumericAndHexInputs(t *testing.T) {
	st := NewState()
	st.AddTab("/d/a.md", "a.md", "# A")
	st.DuplicateActiveStyle() // 編集可能スタイル
	body := do(newH(st), "POST", "/settings/toggle").Body.String()
	if !strings.Contains(body, `type="number"`) {
		t.Errorf("numeric input missing in settings panel")
	}
	if !strings.Contains(body, `class="field-hex"`) {
		t.Errorf("hex text input missing in settings panel")
	}
	if !strings.Contains(body, "data-pair") {
		t.Errorf("paired-input wrapper missing")
	}
	// スライダーと数値が同じ endpoint を指す（対で存在）。
	if strings.Count(body, `hx-post="/settings/field/fontSize"`) < 2 {
		t.Errorf("both slider and number should post to fontSize endpoint")
	}
	// 見出しフィールドの動的 endpoint も生成される。
	if !strings.Contains(body, `hx-post="/settings/heading/0/fontSize"`) {
		t.Errorf("heading numeric endpoint missing")
	}
}

func TestSetStyleSwitchesActive(t *testing.T) {
	st := testState()
	body := do(newH(st), "POST", "/settings/style?value=dark").Body.String()
	if st.ActiveStyle().ID != "dark" {
		t.Errorf("active style should be dark")
	}
	// restyle 応答: 本文＋OOB styleblock（dark の背景）＋コードテーマ。
	if !strings.Contains(body, "#0d1117") { // dark 背景色
		t.Errorf("dark style vars not injected: %s", body)
	}
	if !strings.Contains(body, `id="markmiru-code-theme" hx-swap-oob="true"`) {
		t.Errorf("code theme OOB missing")
	}
	// ダーク系は印刷用の配色（ライトのプリセットの値）を @media print で差し替える。
	if !strings.Contains(body, "@media print{ #content{ --md-color:#24292f") {
		t.Errorf("print vars for dark style not injected: %s", body)
	}
}

// 明るいスタイルは画面の配色のまま印刷するため、印刷用の差し替えを出さない。
func TestNoPrintVarsForLightStyle(t *testing.T) {
	body := do(newH(testState()), "GET", "/").Body.String()
	if strings.Contains(body, "@media print{ #content{") {
		t.Errorf("light style must not emit print vars: %s", body)
	}
}

func TestPresetNotEditableUntilDuplicate(t *testing.T) {
	st := testState() // active=light（プリセット）
	// プリセットのフィールド編集は無効。
	do(newH(st), "POST", "/settings/field/fontSize?value=24")
	if st.ActiveStyle().FontSize != 16 {
		t.Errorf("preset must not be edited directly, got %v", st.ActiveStyle().FontSize)
	}
	// 複製 → 編集可能なユーザースタイルがアクティブ。
	do(newH(st), "POST", "/settings/duplicate")
	if st.ActiveStyle().Builtin {
		t.Errorf("duplicate should create editable style")
	}
}

func TestFieldEditLivePreview(t *testing.T) {
	st := NewState()
	st.AddTab("/d/a.md", "a.md", "# A")
	st.DuplicateActiveStyle() // 編集可能スタイルをアクティブに
	// value はクエリでも可（FormValue はクエリも読む）。styleblock のみ返る。
	body := do(newH(st), "POST", "/settings/field/fontSize?value=22").Body.String()
	if st.ActiveStyle().FontSize != 22 {
		t.Errorf("font size not updated, got %v", st.ActiveStyle().FontSize)
	}
	if !strings.Contains(body, `id="styleblock"`) || !strings.Contains(body, "--md-font-size:22px") {
		t.Errorf("styleblock not returned with new size: %s", body)
	}
	if strings.Contains(body, "hx-swap-oob") {
		t.Errorf("field edit should be a plain styleblock swap (no OOB), got: %s", body)
	}
}

func TestColorFieldUpdatesStyleblock(t *testing.T) {
	st := NewState()
	st.AddTab("/d/a.md", "a.md", "# A")
	st.DuplicateActiveStyle()
	body := do(newH(st), "POST", "/settings/field/background?value=%23112233").Body.String()
	if st.ActiveStyle().Background != "#112233" {
		t.Errorf("background not updated, got %q", st.ActiveStyle().Background)
	}
	if !strings.Contains(body, "--md-bg:#112233") {
		t.Errorf("styleblock should carry new bg: %s", body)
	}
}

func TestColorSchemeFieldRestyles(t *testing.T) {
	st := NewState()
	st.AddTab("/d/a.md", "a.md", "# A")
	st.DuplicateActiveStyle()
	body := do(newH(st), "POST", "/settings/field/colorScheme?value=dark").Body.String()
	if st.ActiveStyle().ColorScheme != "dark" {
		t.Errorf("colorScheme not updated")
	}
	// restyle 応答（コードテーマ OOB を含む）。
	if !strings.Contains(body, `id="markmiru-code-theme" hx-swap-oob="true"`) {
		t.Errorf("colorScheme change should restyle (code theme OOB): %s", body)
	}
}

func TestCustomCSSUpdatesCustomcss(t *testing.T) {
	st := NewState()
	st.AddTab("/d/a.md", "a.md", "# A")
	st.DuplicateActiveStyle()
	body := do(newH(st), "POST", "/settings/field/customCSS?value=.markdown-body%7Bcolor%3Ared%7D").Body.String()
	if !strings.Contains(st.ActiveStyle().CustomCSS, "color:red") {
		t.Errorf("customCSS not stored: %q", st.ActiveStyle().CustomCSS)
	}
	if !strings.Contains(body, `id="customcss"`) || !strings.Contains(body, "color:red") {
		t.Errorf("customcss swap expected: %s", body)
	}
}

func TestBoolFieldToggle(t *testing.T) {
	st := NewState()
	st.AddTab("/d/a.md", "a.md", "# A")
	st.DuplicateActiveStyle()
	do(newH(st), "POST", "/settings/field/maxWidthFull?value=true")
	if !st.ActiveStyle().MaxWidthFull {
		t.Errorf("maxWidthFull should be true")
	}
	body := do(newH(st), "POST", "/settings/field/maxWidthFull?value=").Body.String()
	if st.ActiveStyle().MaxWidthFull {
		t.Errorf("maxWidthFull should be false when unchecked")
	}
	if strings.Contains(body, "--md-max-width:none") {
		t.Errorf("unchecked maxWidthFull should not yield max-width:none")
	}
}

func editableState(t *testing.T) *State {
	t.Helper()
	st := NewState()
	st.AddTab("/d/a.md", "a.md", "# A")
	st.DuplicateActiveStyle()
	return st
}

func TestHeadingFieldUpdate(t *testing.T) {
	st := editableState(t)
	do(newH(st), "POST", "/settings/heading/0/color?value=%23ff0000") // h1 色
	if st.ActiveStyle().Headings[0].Color != "#ff0000" {
		t.Errorf("h1 color not updated: %q", st.ActiveStyle().Headings[0].Color)
	}
	body := do(newH(st), "POST", "/settings/heading/0/fontSize?value=40").Body.String()
	if st.ActiveStyle().Headings[0].FontSize != 40 {
		t.Errorf("h1 size not updated")
	}
	if !strings.Contains(body, "--md-h1-size:40px") {
		t.Errorf("styleblock should reflect h1 size: %s", body)
	}
}

func TestRenameStyle(t *testing.T) {
	st := editableState(t)
	do(newH(st), "POST", "/settings/rename?value=%E3%83%9E%E3%82%A4") // "マイ"
	if st.ActiveStyle().Name != "マイ" {
		t.Errorf("rename failed: %q", st.ActiveStyle().Name)
	}
	// 空は拒否。
	do(newH(st), "POST", "/settings/rename?value=")
	if st.ActiveStyle().Name != "マイ" {
		t.Errorf("empty rename should be rejected")
	}
}

func TestDeleteStyle(t *testing.T) {
	st := editableState(t)
	n := len(st.Styles())
	do(newH(st), "POST", "/settings/delete")
	if len(st.Styles()) != n-1 {
		t.Errorf("style not deleted")
	}
	if st.ActiveStyle().ID != "light" {
		t.Errorf("active should fall back to light after delete, got %q", st.ActiveStyle().ID)
	}
}

func TestExportStyle(t *testing.T) {
	st := editableState(t)
	host := &fakeHost{exportPath: "/out/mystyle.json"}
	do(NewHandler(st, host), "POST", "/settings/export")
	js := host.written["/out/mystyle.json"]
	if !strings.Contains(js, `"kind": "style"`) {
		t.Errorf("exported JSON not written / wrong format: %q", js)
	}
}

func TestImportStyle(t *testing.T) {
	st := NewState()
	n := len(st.Styles())
	host := &fakeHost{importContent: `{"app":"Markmiru","kind":"style","version":1,"style":{"name":"取込","color":"#123456"}}`}
	do(NewHandler(st, host), "POST", "/settings/import")
	if len(st.Styles()) != n+1 {
		t.Errorf("imported style not added")
	}
	if st.ActiveStyle().Name != "取込" || st.ActiveStyle().Color != "#123456" {
		t.Errorf("imported style not active/normalized: %+v", st.ActiveStyle())
	}
	if st.ActiveStyle().Builtin {
		t.Errorf("imported style must be editable")
	}
}

func TestModeToggle(t *testing.T) {
	st := NewState()
	tab := st.AddTab("/d/a.md", "a.md", "# Title\n\ntext")
	// 既定は閲覧。切替で編集（textarea オーバーレイ）。
	body := do(newH(st), "POST", "/tabs/"+tab.ID+"/mode").Body.String()
	if m, _ := st.ActiveMeta(); m != "source" {
		t.Errorf("mode should be source after toggle")
	}
	if !strings.Contains(body, `class="editor"`) || !strings.Contains(body, `class="editor-input"`) {
		t.Errorf("editor overlay not rendered: %s", body)
	}
	if !strings.Contains(body, `id="hl-`+tab.ID) {
		t.Errorf("highlight layer id missing")
	}
	// もう一度で閲覧に戻る。
	body = do(newH(st), "POST", "/tabs/"+tab.ID+"/mode").Body.String()
	if !strings.Contains(body, `class="markdown-body"`) {
		t.Errorf("should return to view: %s", body)
	}
}

func TestNewTabIsEditMode(t *testing.T) {
	st := NewState()
	do(newH(st), "POST", "/tabs/new")
	if m, _ := st.ActiveMeta(); m != "source" {
		t.Errorf("new untitled tab should open in source mode")
	}
}

func TestEditContentUpdatesHighlight(t *testing.T) {
	st := NewState()
	tab := st.AddTab("", "無題", "")
	st.SetMode(tab.ID)                                                                 // source
	body := do(newH(st), "POST", "/tabs/"+tab.ID+"/content?value=%23+H").Body.String() // "# H"
	if _, _, c, _ := st.SaveInfo(tab.ID); c != "# H" {
		t.Errorf("content not updated: %q", c)
	}
	// 応答はハイライト層の「中身」（innerHTML 差し替え用）＋タブバー OOB のみ。
	// <pre id="hl-…"> 要素自体は差し替えない（クライアント側で保持＝スクロール位置維持）。
	if !strings.Contains(body, "# H") {
		t.Errorf("highlighted content not returned: %s", body)
	}
	if strings.Contains(body, "<pre") {
		t.Errorf("edit-sync must not replace the <pre> element itself: %s", body)
	}
	if !strings.Contains(body, `id="tabbar"`) {
		t.Errorf("edit-sync should OOB-update the tabbar: %s", body)
	}
}

// 編集オーバーレイの改行パディング（最下部の 1 行ズレ・先頭空行の欠落防止）:
//   - ハイライト層は末尾に改行を 1 つ補う（<pre> は末尾改行を行として描画しないが、
//     textarea は末尾改行の後にキャレット行を持つ。補わないと最下部で表示が 1 行ズレる）
//   - <pre>/<textarea> の開始タグ直後には犠牲改行を置く（HTML パーサはこの位置の改行
//     1 つを無視するため、先頭が空行の文書で行・値が欠けない）
func TestEditorOverlayNewlinePadding(t *testing.T) {
	st := NewState()
	tab := st.AddTab("/d/a.md", "a.md", "\nA\n") // 先頭空行＋末尾改行
	st.SetMode(tab.ID)
	// テンプレート由来の犠牲改行はファイルの改行コード（CRLF チェックアウト）に依存するため、
	// ブラウザの入力正規化（CRLF→LF）と同じ変換をかけてから検証する。
	body := strings.ReplaceAll(do(newH(st), "POST", "/tabs/"+tab.ID+"/activate").Body.String(), "\r\n", "\n")
	if !strings.Contains(body, "aria-hidden=\"true\">\n\nA\n\n</pre>") {
		t.Errorf("highlight layer should carry sacrificial + trailing newline padding: %s", body)
	}
	if !strings.Contains(body, "delay:150ms\">\n\nA\n</textarea>") {
		t.Errorf("textarea should carry a sacrificial leading newline: %s", body)
	}
	// 編集応答（innerHTML 差し替え）はパーサの改行除去を受けないため、犠牲改行なし＋末尾補いのみ。
	body = do(newH(st), "POST", "/tabs/"+tab.ID+"/content?value=%0AB%0A").Body.String() // "\nB\n"
	if !strings.HasPrefix(body, "\nB\n\n<div id=\"tabbar\"") {
		t.Errorf("edit-sync should be padded highlight followed by tabbar OOB: %q", body)
	}
}

func TestEditMenuSyncsWithMode(t *testing.T) {
	st := NewState()
	tab := st.AddTab("/d/a.md", "a.md", "# A") // 既定は閲覧
	host := &fakeHost{}
	h := NewHandler(st, host)
	// シェル描画（閲覧）→ 編集メニューは無効。
	do(h, "GET", "/")
	if host.editMenuEnabled {
		t.Errorf("view mode should disable edit menu")
	}
	// 編集モードへ → 有効化される（サーバ主導）。
	do(h, "POST", "/tabs/"+tab.ID+"/mode")
	if !host.editMenuEnabled {
		t.Errorf("source mode should enable edit menu")
	}
	// 閲覧へ戻す → 無効化。
	do(h, "POST", "/tabs/"+tab.ID+"/mode")
	if host.editMenuEnabled {
		t.Errorf("back to view should disable edit menu")
	}
}

func TestEditMenuDisabledForReadOnly(t *testing.T) {
	st := NewState()
	host := &fakeHost{readme: "# readme"}
	h := NewHandler(st, host)
	do(h, "POST", "/doc/about") // 読み取り専用タブ（常に閲覧）
	if host.editMenuEnabled {
		t.Errorf("read-only tab should keep edit menu disabled")
	}
}

func TestEditContentUpdatesTabbarDirty(t *testing.T) {
	st := NewState()
	tab := st.AddTab("/d/a.md", "a.md", "orig")
	st.SetMode(tab.ID) // source
	body := do(newH(st), "POST", "/tabs/"+tab.ID+"/content?value=changed").Body.String()
	// ハイライト層に加えてタブバーを OOB 更新し、未保存マーク（•）を即時反映する。
	if !strings.Contains(body, `id="tabbar" hx-swap-oob="true"`) {
		t.Errorf("edit response should OOB-update tabbar: %s", body)
	}
	if !strings.Contains(body, "•") {
		t.Errorf("dirty tab should show unsaved mark in tabbar: %s", body)
	}
}

func TestReadOnlyTabNoModeToggle(t *testing.T) {
	st := NewState()
	host := &fakeHost{readme: "# readme"}
	do(NewHandler(st, host), "POST", "/doc/about") // 読み取り専用タブ
	id := st.ActiveID()
	rec := do(NewHandler(st, host), "POST", "/tabs/"+id+"/mode")
	if rec.Code != http.StatusNotFound {
		t.Errorf("read-only tab mode toggle should 404, got %d", rec.Code)
	}
}

func TestActiveSave(t *testing.T) {
	st, _ := dirtyTab("/d/a.md")
	host := &fakeHost{}
	do(NewHandler(st, host), "POST", "/active/save")
	if host.written["/d/a.md"] != "edited!!!" {
		t.Errorf("active save should write active tab: %+v", host.written)
	}
}

func TestActiveMode(t *testing.T) {
	st := NewState()
	st.AddTab("/d/a.md", "a.md", "# A")
	do(newH(st), "POST", "/active/mode")
	if m, _ := st.ActiveMeta(); m != "source" {
		t.Errorf("active mode toggle failed")
	}
}

func TestActivePrintForcesViewAndTriggers(t *testing.T) {
	st := NewState()
	tab := st.AddTab("/d/a.md", "a.md", "# A")
	st.SetMode(tab.ID) // source
	rec := do(newH(st), "POST", "/active/print")
	if m, _ := st.ActiveMeta(); m != "view" {
		t.Errorf("print should force view mode")
	}
	if rec.Header().Get("HX-Trigger-After-Settle") != "do-print" {
		t.Errorf("print should set do-print trigger header")
	}
}

func TestFindOpenFragment(t *testing.T) {
	rec := do(newH(testState()), "POST", "/find/open")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	// glue.js が配線するフック（入力・件数・前後・閉じる）が揃っている。
	for _, want := range []string{
		`class="find-bar"`, "data-find-input", "data-find-count",
		"data-find-prev", "data-find-next", "data-find-close",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("find-bar missing %q: %s", want, body)
		}
	}
	// バーはサーバ状態を持たない純 UI（hx-* を持たない）。
	if strings.Contains(body, "hx-post") {
		t.Errorf("find-bar should not carry server actions: %s", body)
	}
}

func TestQuitRequestShowsDialogForFirstUnsaved(t *testing.T) {
	st := NewState()
	st.AddTab("/d/a.md", "a.md", "A") // clean
	b := st.AddTab("/d/b.md", "b.md", "B")
	st.UpdateContent(b.ID, "B!") // dirty
	host := &fakeHost{}
	rec := do(NewHandler(st, host), "POST", "/quit/request")
	if rec.Header().Get("HX-Retarget") != "#dialog-host" {
		t.Errorf("quit with unsaved should show a dialog")
	}
	body := rec.Body.String()
	if !strings.Contains(body, "未保存の変更") || !strings.Contains(body, "b.md") {
		t.Errorf("quit dialog should target first unsaved tab (b): %s", body)
	}
	if !strings.Contains(body, "/quit/step?id="+b.ID) {
		t.Errorf("quit dialog buttons should post to /quit/step: %s", body)
	}
	if host.quitCalled {
		t.Errorf("must not quit while unsaved tabs remain")
	}
}

func TestQuitRequestNoUnsavedQuitsImmediately(t *testing.T) {
	st := NewState()
	st.AddTab("/d/a.md", "a.md", "A") // clean
	host := &fakeHost{}
	do(NewHandler(st, host), "POST", "/quit/request")
	if !host.quitCalled {
		t.Errorf("quit with no unsaved should quit immediately")
	}
}

func TestQuitStepDiscardLastQuits(t *testing.T) {
	st := NewState()
	tab := st.AddTab("/d/a.md", "a.md", "A")
	st.UpdateContent(tab.ID, "A!") // 唯一の未保存
	host := &fakeHost{}
	do(NewHandler(st, host), "POST", "/quit/step?id="+tab.ID+"&choice=discard")
	if !host.quitCalled {
		t.Errorf("resolving the last unsaved tab should quit")
	}
}

func TestQuitStepSaveAdvancesToNext(t *testing.T) {
	st := NewState()
	a := st.AddTab("/d/a.md", "a.md", "A")
	b := st.AddTab("/d/b.md", "b.md", "B")
	st.UpdateContent(a.ID, "A!")
	st.UpdateContent(b.ID, "B!")
	host := &fakeHost{}
	h := NewHandler(st, host)
	rec := do(h, "POST", "/quit/step?id="+a.ID+"&choice=save")
	if host.written["/d/a.md"] != "A!" {
		t.Errorf("save step should write tab a: %+v", host.written)
	}
	if rec.Header().Get("HX-Retarget") != "#dialog-host" || !strings.Contains(rec.Body.String(), "b.md") {
		t.Errorf("after saving a, should prompt for b: %s", rec.Body.String())
	}
	if host.quitCalled {
		t.Errorf("must not quit while b is unsaved")
	}
	do(h, "POST", "/quit/step?id="+b.ID+"&choice=discard")
	if !host.quitCalled {
		t.Errorf("after resolving b, should quit")
	}
}

func TestQuitStepCancelAborts(t *testing.T) {
	st := NewState()
	tab := st.AddTab("/d/a.md", "a.md", "A")
	st.UpdateContent(tab.ID, "A!")
	host := &fakeHost{}
	do(NewHandler(st, host), "POST", "/quit/step?id="+tab.ID+"&choice=cancel")
	if host.quitCalled {
		t.Errorf("cancel should abort quit")
	}
	if len(st.TabVMs()) != 1 {
		t.Errorf("cancel should keep all tabs")
	}
}

func TestQuitStepUntitledSaveCancelAborts(t *testing.T) {
	st := NewState()
	tab := st.AddTab("", "無題", "")
	st.UpdateContent(tab.ID, "draft") // dirty 無題
	host := &fakeHost{savePath: ""}   // 保存先選択をキャンセル
	do(NewHandler(st, host), "POST", "/quit/step?id="+tab.ID+"&choice=save")
	if host.quitCalled {
		t.Errorf("cancelled save should abort quit, not quit")
	}
	if len(st.TabVMs()) != 1 {
		t.Errorf("cancelled save should keep the tab")
	}
}

func TestQuitCancelRestoresDirtyTabs(t *testing.T) {
	st := NewState()
	a := st.AddTab("/d/a.md", "a.md", "A")
	b := st.AddTab("/d/b.md", "b.md", "B")
	st.UpdateContent(a.ID, "A!")
	st.UpdateContent(b.ID, "B!")
	host := &fakeHost{}
	h := NewHandler(st, host)
	do(h, "POST", "/quit/step?id="+a.ID+"&choice=discard") // a を破棄（閉じない・印だけ）
	do(h, "POST", "/quit/step?id="+b.ID+"&choice=cancel")  // b でキャンセル → 終了中止
	if host.quitCalled {
		t.Errorf("cancel should abort quit")
	}
	if len(st.TabVMs()) != 2 {
		t.Errorf("cancel should keep both tabs open, got %d", len(st.TabVMs()))
	}
	if !st.IsDirty(a.ID) || !st.IsDirty(b.ID) {
		t.Errorf("cancel should leave both tabs unsaved")
	}
	// 印が解除され、再度の終了要求は最初の未保存（a）から確認が始まる。
	if !strings.Contains(do(h, "POST", "/quit/request").Body.String(), "a.md") {
		t.Errorf("after cancel, quit should re-prompt from first unsaved (a)")
	}
}

func TestSessionFilesExcludesUntitledAndReadOnly(t *testing.T) {
	st := NewState()
	st.AddTab("/d/a.md", "a.md", "A")
	st.AddTab("", "無題", "draft") // 無題 → 除外
	b := st.AddTab("/d/b.md", "b.md", "B")
	st.OpenDoc("ライセンス", "L") // 読み取り専用 → 除外
	st.Activate(b.ID)
	files, active := st.SessionFiles()
	if len(files) != 2 || files[0] != "/d/a.md" || files[1] != "/d/b.md" {
		t.Errorf("session should list only file-backed tabs: %+v", files)
	}
	if active != 1 {
		t.Errorf("active index should point to b (1), got %d", active)
	}
}

func TestUserStylesRoundTrip(t *testing.T) {
	st := NewState()
	st.DuplicateActiveStyle() // 編集可能ユーザースタイルを作成・アクティブ化
	st.UpdateActiveStyleField("background", "#123456")
	activeID := st.ActiveStyle().ID
	js := st.UserStylesJSON()
	if !strings.Contains(js, "#123456") {
		t.Errorf("user styles JSON should contain edited style: %q", js)
	}
	// 別 State へ復元。
	st2 := NewState()
	st2.RestoreStylesJSON(js, activeID)
	if st2.ActiveStyle().ID != activeID {
		t.Errorf("restored active style id mismatch: %s vs %s", st2.ActiveStyle().ID, activeID)
	}
	if st2.ActiveStyle().Background != "#123456" {
		t.Errorf("restored style should keep edited background, got %q", st2.ActiveStyle().Background)
	}
	if st2.ActiveStyle().Builtin {
		t.Errorf("restored user style must be editable")
	}
}

func TestRestoreActivePreset(t *testing.T) {
	st := NewState()
	st.RestoreStylesJSON("", "dark") // ユーザースタイル無し・プリセットをアクティブに
	if st.ActiveStyle().ID != "dark" {
		t.Errorf("should restore preset active style, got %s", st.ActiveStyle().ID)
	}
}

func TestMissingFileDialogOnShell(t *testing.T) {
	st := NewState()
	st.AddTab("/d/a.md", "a.md", "A")
	st.SetPendingMissing([]string{"/d/gone.md"})
	body := do(newH(st), "GET", "/").Body.String()
	if !strings.Contains(body, "ファイルが見つかりません") || !strings.Contains(body, "/d/gone.md") {
		t.Errorf("missing-file dialog should appear on shell: %s", body)
	}
	if !strings.Contains(body, `hx-post="/missing/retry"`) || !strings.Contains(body, `hx-post="/missing/skip"`) {
		t.Errorf("missing dialog should offer retry/skip")
	}
}

func TestMissingSkipAdvancesThenClears(t *testing.T) {
	st := NewState()
	st.SetPendingMissing([]string{"/d/x.md", "/d/y.md"})
	h := newH(st)
	body := do(h, "POST", "/missing/skip").Body.String() // x をスキップ → y へ
	if !strings.Contains(body, "/d/y.md") {
		t.Errorf("after skipping x, should prompt for y: %s", body)
	}
	if st.FirstMissing() != "/d/y.md" {
		t.Errorf("x should be removed, got first=%q", st.FirstMissing())
	}
	body2 := do(h, "POST", "/missing/skip").Body.String() // y もスキップ → 解消
	if strings.Contains(body2, "ファイルが見つかりません") {
		t.Errorf("after skipping all, dialog should be gone: %s", body2)
	}
	if st.FirstMissing() != "" {
		t.Errorf("no missing should remain")
	}
}

func TestMissingRetrySuccessOpensTab(t *testing.T) {
	st := NewState()
	st.SetPendingMissing([]string{"/d/back.md"})
	host := &fakeHost{readFiles: map[string]string{"/d/back.md": "# Back"}}
	do(NewHandler(st, host), "POST", "/missing/retry")
	if st.FirstMissing() != "" {
		t.Errorf("retry success should clear the missing entry")
	}
	vms := st.TabVMs()
	if len(vms) != 1 || vms[0].FileName != "back.md" {
		t.Errorf("retry should open the file as a tab: %+v", vms)
	}
}

func TestMissingRetryStillMissingReprompts(t *testing.T) {
	st := NewState()
	st.SetPendingMissing([]string{"/d/still.md"})
	host := &fakeHost{} // readFiles nil → 読めない
	body := do(NewHandler(st, host), "POST", "/missing/retry").Body.String()
	if !strings.Contains(body, "/d/still.md") {
		t.Errorf("retry that still fails should re-prompt: %s", body)
	}
	if st.FirstMissing() != "/d/still.md" {
		t.Errorf("still-missing file should remain pending")
	}
}

func TestLinkConfirmShowsDialog(t *testing.T) {
	st := NewState()
	host := &fakeHost{}
	rec := httptest.NewRecorder()
	NewHandler(st, host).ServeHTTP(rec, httptest.NewRequest("POST", "/link/confirm?url=https://example.com/x", nil))
	body := rec.Body.String()
	if !strings.Contains(body, "既定のアプリで開きますか") || !strings.Contains(body, "https://example.com/x") {
		t.Errorf("link confirm dialog not rendered: %s", body)
	}
	if !strings.Contains(body, `hx-post="/link/open"`) || !strings.Contains(body, `hx-post="/link/dismiss"`) {
		t.Errorf("link dialog should offer open/dismiss")
	}
	// 危険側「はい」は初期 disabled（500ms 遅延）。
	if !strings.Contains(body, "data-dialog-accept disabled") {
		t.Errorf("open button should start disabled")
	}
	if st.PendingLink() != "https://example.com/x" {
		t.Errorf("pending link should be stored, got %q", st.PendingLink())
	}
}

func TestLinkOpenCallsHostAndClears(t *testing.T) {
	st := NewState()
	st.SetPendingLink("https://example.com/x")
	host := &fakeHost{}
	body := do(NewHandler(st, host), "POST", "/link/open").Body.String()
	if host.openedURL != "https://example.com/x" {
		t.Errorf("open should call host.OpenURL, got %q", host.openedURL)
	}
	if st.PendingLink() != "" {
		t.Errorf("pending link should be cleared after open")
	}
	// 応答は #dialog-host を OOB で空にする（ダイアログが確実に閉じる）。
	if !strings.Contains(body, `id="dialog-host" hx-swap-oob="true"`) || strings.Contains(body, "開きますか") {
		t.Errorf("open response should clear the dialog via OOB: %s", body)
	}
}

func TestLinkDismissClearsWithoutOpening(t *testing.T) {
	st := NewState()
	st.SetPendingLink("https://example.com/x")
	host := &fakeHost{}
	do(NewHandler(st, host), "POST", "/link/dismiss")
	if host.openedURL != "" {
		t.Errorf("dismiss must not open a URL")
	}
	if st.PendingLink() != "" {
		t.Errorf("pending link should be cleared after dismiss")
	}
}

func TestLinkConfirmRejectsNonExternalScheme(t *testing.T) {
	st := NewState()
	host := &fakeHost{}
	rec := httptest.NewRecorder()
	NewHandler(st, host).ServeHTTP(rec, httptest.NewRequest("POST", "/link/confirm?url=file:///etc/passwd", nil))
	if strings.Contains(rec.Body.String(), "開きますか") {
		t.Errorf("non-external scheme should not show a dialog")
	}
	if st.PendingLink() != "" {
		t.Errorf("non-external scheme should not be stored")
	}
	// 保持中の非外部 URL は開かない（防御）。
	st.SetPendingLink("javascript:alert(1)")
	do(NewHandler(st, host), "POST", "/link/open")
	if host.openedURL != "" {
		t.Errorf("must never open a non-external URL: %q", host.openedURL)
	}
}

// Windows ではアプリ自身が http://wails.localhost/ から配信されるため、文書内アンカーや
// 相対リンクが解決されてできる自オリジンの URL は外部として扱わない（OS ブラウザへ渡さない）。
func TestLinkRejectsWebViewOwnOrigin(t *testing.T) {
	st := NewState()
	host := &fakeHost{}
	for _, u := range []string{"http://wails.localhost/#anchor", "http://wails.localhost/other.md"} {
		rec := httptest.NewRecorder()
		NewHandler(st, host).ServeHTTP(rec, httptest.NewRequest("POST", "/link/confirm?url="+url.QueryEscape(u), nil))
		if strings.Contains(rec.Body.String(), "開きますか") {
			t.Errorf("own origin should not show a dialog: %q", u)
		}
		if st.PendingLink() != "" {
			t.Errorf("own origin should not be stored: %q", u)
		}
		// 保持中でも開かない（多層防御）。
		st.SetPendingLink(u)
		do(NewHandler(st, host), "POST", "/link/open")
		if host.openedURL != "" {
			t.Errorf("must never open the app's own origin: %q", host.openedURL)
		}
		st.SetPendingLink("")
	}
	// 紛らわしいホスト名は外部のまま（前方一致で誤って弾かない）。
	rec := httptest.NewRecorder()
	NewHandler(st, host).ServeHTTP(rec, httptest.NewRequest("POST", "/link/confirm?url=https://wails.localhost.example.com/", nil))
	if !strings.Contains(rec.Body.String(), "開きますか") {
		t.Errorf("a different host that merely starts with the WebView host is external")
	}
}

func TestServeAssets(t *testing.T) {
	h := newH(testState())
	for _, p := range []string{"/assets/htmx.min.js", "/assets/mermaid.min.js", "/assets/glue.js", "/assets/app.css", "/assets/markdown.css"} {
		rec := do(h, "GET", p)
		if rec.Code != http.StatusOK {
			t.Errorf("asset %s status = %d, want 200", p, rec.Code)
		}
		if rec.Body.Len() == 0 {
			t.Errorf("asset %s is empty", p)
		}
	}
}

func TestCSPHeaderPresent(t *testing.T) {
	rec := do(newH(testState()), "GET", "/")
	csp := rec.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "default-src 'none'") || !strings.Contains(csp, "script-src 'self'") {
		t.Errorf("CSP header missing/wrong on shell: %q", csp)
	}
	// 断片・アセットにも付与される（全レスポンス）。
	if do(newH(testState()), "GET", "/assets/glue.js").Header().Get("Content-Security-Policy") == "" {
		t.Errorf("assets should also carry CSP header")
	}
	if do(newH(testState()), "POST", "/tabs/t2/activate").Header().Get("Content-Security-Policy") == "" {
		t.Errorf("fragment responses should also carry CSP header")
	}
}

func TestServeVendoredFonts(t *testing.T) {
	h := newH(testState())
	// fonts.css と少なくとも1つの woff2 が embed から配信される。
	if rec := do(h, "GET", "/assets/fonts.css"); rec.Code != http.StatusOK || rec.Body.Len() == 0 {
		t.Fatalf("fonts.css not served: status=%d len=%d", rec.Code, rec.Body.Len())
	}
	rec := do(h, "GET", "/assets/fonts/noto-sans-jp-latin-400-normal.woff2")
	if rec.Code != http.StatusOK || rec.Body.Len() == 0 {
		t.Errorf("vendored woff2 not served: status=%d len=%d", rec.Code, rec.Body.Len())
	}
	// シェルが fonts.css を読み込む。
	if !strings.Contains(do(h, "GET", "/").Body.String(), "/assets/fonts.css") {
		t.Errorf("shell should link fonts.css")
	}
}

func TestUnknownPathNotFound(t *testing.T) {
	rec := do(newH(testState()), "GET", "/nope")
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

// --- コンテキストメニュー（POST /ctxmenu/open） ------------------------------

// ctxItems は右クリックメニュー断片を「項目キー→その項目が disabled か」に分解する。
// キーは data-ctx-cmd の値、リンク項目は "link-open" / "link-copy"（1 項目＝1 行前提）。
func ctxItems(t *testing.T, body string) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	for _, line := range strings.Split(body, "\n") {
		var key string
		switch {
		case strings.Contains(line, "data-ctx-cmd="):
			key = strings.SplitN(strings.SplitN(line, `data-ctx-cmd="`, 2)[1], `"`, 2)[0]
		case strings.Contains(line, "data-ctx-link-open="):
			key = "link-open"
		case strings.Contains(line, "data-ctx-link-copy="):
			key = "link-copy"
		default:
			continue
		}
		out[key] = strings.Contains(line, " disabled")
	}
	return out
}

// 項目の並びは全ケースで同一。閲覧モードでは編集系が、選択が無ければ切り取り/コピーが
// disabled になる（項目自体は消さない）。
func TestCtxMenuViewMode(t *testing.T) {
	h := newH(testState())
	items := ctxItems(t, do(h, "POST", "/ctxmenu/open").Body.String())
	for _, key := range []string{"undo", "redo", "cut", "copy", "paste", "selectAll", "link-open", "link-copy"} {
		if _, ok := items[key]; !ok {
			t.Fatalf("menu item %q missing in view mode: %v", key, items)
		}
	}
	for _, key := range []string{"undo", "redo", "cut", "paste", "copy", "link-open", "link-copy"} {
		if !items[key] {
			t.Errorf("%q should be disabled in view mode without selection", key)
		}
	}
	if items["selectAll"] {
		t.Errorf("selectAll should always be enabled")
	}
	// 選択があればコピーのみ活性化（切り取りは閲覧モードでは不可のまま）。
	items = ctxItems(t, do(h, "POST", "/ctxmenu/open?sel=1").Body.String())
	if items["copy"] {
		t.Errorf("copy should be enabled with selection")
	}
	if !items["cut"] {
		t.Errorf("cut should stay disabled in view mode")
	}
}

// 編集モード: 編集系が活性。選択の有無が切り取り/コピーの活性を決める。
func TestCtxMenuEditMode(t *testing.T) {
	st := testState()
	st.SetMode(st.ActiveID())
	h := newH(st)
	items := ctxItems(t, do(h, "POST", "/ctxmenu/open?sel=1&undo=1&redo=1").Body.String())
	for _, key := range []string{"undo", "redo", "cut", "copy", "paste", "selectAll"} {
		if items[key] {
			t.Errorf("%q should be enabled in edit mode with selection", key)
		}
	}
	items = ctxItems(t, do(h, "POST", "/ctxmenu/open?undo=1&redo=1").Body.String())
	if !items["cut"] || !items["copy"] {
		t.Errorf("cut/copy should be disabled without selection: %v", items)
	}
	if items["paste"] {
		t.Errorf("paste should stay enabled without selection")
	}
}

// 取り消し履歴を使い切った側（undo / redo）だけが非活性になる。編集モードでも履歴が無ければ非活性。
func TestCtxMenuUndoRedoHistory(t *testing.T) {
	st := testState()
	st.SetMode(st.ActiveID())
	h := newH(st)
	items := ctxItems(t, do(h, "POST", "/ctxmenu/open?redo=1").Body.String())
	if !items["undo"] {
		t.Errorf("undo should be disabled when nothing is left to undo: %v", items)
	}
	if items["redo"] {
		t.Errorf("redo should stay enabled: %v", items)
	}
	items = ctxItems(t, do(h, "POST", "/ctxmenu/open?undo=1").Body.String())
	if items["undo"] {
		t.Errorf("undo should stay enabled: %v", items)
	}
	if !items["redo"] {
		t.Errorf("redo should be disabled when nothing is left to redo: %v", items)
	}
	// 閲覧モードでは履歴があっても編集操作自体ができないため非活性のまま。
	items = ctxItems(t, do(newH(testState()), "POST", "/ctxmenu/open?undo=1&redo=1").Body.String())
	if !items["undo"] || !items["redo"] {
		t.Errorf("undo/redo must stay disabled in view mode: %v", items)
	}
}

// リンク項目: 外部スキームのみ活性化し、それ以外（javascript: 等）は URL を出さず disabled のまま。
func TestCtxMenuLink(t *testing.T) {
	h := newH(testState())
	body := do(h, "POST", "/ctxmenu/open?link=https%3A%2F%2Fexample.com%2F").Body.String()
	if items := ctxItems(t, body); items["link-open"] || items["link-copy"] {
		t.Errorf("link items should be enabled for an external URL: %v", items)
	}
	if !strings.Contains(body, `data-ctx-link-open="https://example.com/"`) {
		t.Errorf("link URL missing: %q", body)
	}
	body = do(h, "POST", "/ctxmenu/open?link=javascript%3Aalert(1)").Body.String()
	if items := ctxItems(t, body); !items["link-open"] || !items["link-copy"] {
		t.Errorf("link items must stay disabled for a non-external URL: %v", items)
	}
	if strings.Contains(body, "javascript") {
		t.Errorf("non-external URL must not be emitted: %q", body)
	}
}

// 自オリジン（Windows の http://wails.localhost/…）はリンクではないので 2 項目は非活性のまま。
// glue の externalHref が除外するが、サーバ側でも同じ判定を持つ（2026-08-17 の Windows 検証で
// 文書内アンカーがリンク扱いになる不具合を検出）。
func TestCtxMenuLinkRejectsWebViewOwnOrigin(t *testing.T) {
	h := newH(testState())
	for _, u := range []string{"http://wails.localhost/#anchor", "http://wails.localhost/other.md"} {
		body := do(h, "POST", "/ctxmenu/open?link="+url.QueryEscape(u)).Body.String()
		if items := ctxItems(t, body); !items["link-open"] || !items["link-copy"] {
			t.Errorf("link items must stay disabled for the app's own origin (%q): %v", u, items)
		}
		if strings.Contains(body, "wails.localhost") {
			t.Errorf("own-origin URL must not be emitted: %q", body)
		}
	}
}

// --- クリップボード読み取り（POST /clipboard/read） ---------------------------

// クリップボードの内容をそのまま text/plain で返す（貼り付けは glue が挿入する）。
func TestClipboardRead(t *testing.T) {
	host := &fakeHost{clipboard: "貼り付ける文字列"}
	rec := do(NewHandler(testState(), host), "POST", "/clipboard/read")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Body.String(); got != "貼り付ける文字列" {
		t.Errorf("body = %q, want the clipboard text", got)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("Content-Type = %q, want text/plain", ct)
	}
}

// 取得失敗時は本文を返さない（glue 側は無視して何も挿入しない）。
func TestClipboardReadError(t *testing.T) {
	host := &fakeHost{clipboard: "secret", clipboardErr: errors.New("no clipboard")}
	rec := do(NewHandler(testState(), host), "POST", "/clipboard/read")
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "secret") {
		t.Errorf("clipboard text must not be returned on error: %q", rec.Body.String())
	}
}
