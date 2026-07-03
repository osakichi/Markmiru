// Package web はサーバサイドレンダリング（SSR）の HTTP ハンドラを提供する。
// アプリの UI はこの層が html/template で生成する HTML／HTML 断片で、htmx がそれを DOM に差し替える。
//
// Wails の AssetServer.Handler に載せ、WebView の要求横取り（macOS/Linux=wails:// スキーム、
// Windows=WebView2 の WebResourceRequested）を介して **プロセス内で直接呼ばれる**疑似 HTTP。
// 実際の TCP/UNIX ソケットや開放ポートは無い。設計: docs/アーキテクチャ・画面設計.md §1, §4.4。
//
// 状態（タブ集合／アクティブ／dirty／セッション／スタイル）は State が保持する。OS 連携（ネイティブ
// ダイアログ・ファイル I/O・ウィンドウ・外部 URL 起動）は Host インタフェース経由で、main の webHost
// アダプタが app へ委譲する（web→main の循環参照を避ける境界）。
package web

import (
	"embed"
	"html/template"
	"io/fs"
	"net/http"
	"strconv"
	"strings"

	"Markmiru/render"
	"Markmiru/style"
)

//go:embed templates/*.html
var templatesFS embed.FS

//go:embed assets/*
var assetsFS embed.FS

var shellTmpl = template.Must(template.New("").Funcs(template.FuncMap{
	"inc": func(i int) int { return i + 1 },
	// dict はキー・値の並びから map を作る（部分テンプレートへ複数値を渡す用）。
	"dict": func(kv ...any) map[string]any {
		m := make(map[string]any, len(kv)/2)
		for i := 0; i+1 < len(kv); i += 2 {
			if k, ok := kv[i].(string); ok {
				m[k] = kv[i+1]
			}
		}
		return m
	},
}).ParseFS(templatesFS, "templates/*.html"))

// --- テンプレートのビューモデル ---

type tabVM struct {
	ID       string
	FileName string
	Active   bool
	Dirty    bool
}

type contentVM struct {
	Body  template.HTML
	Empty bool
	// Scheme は本文ラッパに出力する colorScheme（glue.js が mermaid テーマに使う。
	// 本文再描画ごとに最新スキームが伝わるよう #content 側に持たせる）。
	Scheme string
	// 編集モード（Mode == "source"）用。
	Mode        string
	TabID       string
	Raw         string        // textarea の内容
	Highlighted template.HTML // chroma ハイライト層
}

// styleOptVM は設定パネルのスタイル選択肢。
type styleOptVM struct {
	ID      string
	Name    string
	Builtin bool
	Active  bool
}

type shellVM struct {
	Title   string
	Tabs    []tabVM
	Content contentVM
	// StyleVars はアクティブスタイルの CSS 変数（#content 配下が継承）。
	// CodeCSS はコードハイライト用 CSS（chroma）。いずれも <style> に注入する。
	StyleVars template.CSS
	CodeCSS   template.CSS
	// Scheme はアクティブスタイルの colorScheme（"light"/"dark"）。
	// glue.js が mermaid のテーマ切替に使う（#app の data-scheme として出力）。
	Scheme string
	// Dialog は表示すべきダイアログ（無ければ nil）。種類は dialogVM.Kind で分岐
	// （外部画像 / 未保存クローズ / 不在ファイル / 外部リンク確認）。
	Dialog *dialogVM
	// SidebarOpen はサイドバーの開閉状態。
	SidebarOpen bool
	// ActiveID はアクティブタブ ID（保存ボタンの対象）。
	ActiveID string
	// ActiveMode はアクティブタブのモード（"view"/"source"）、ActiveReadOnly は読み取り専用か
	// （モード切替ボタンの表示・ラベルに使う）。
	ActiveMode     string
	ActiveReadOnly bool

	// 設定パネル
	SettingsOpen    bool
	Styles          []styleOptVM
	ActiveStyle     style.Style // 編集フィールドの現在値
	ActiveEditable  bool        // アクティブが編集可能（非 builtin）か
	FontOptions     []style.FontOption
	CodeFontOptions []style.FontOption
	// CustomCSS はアクティブスタイルの上書き CSS（#customcss に注入）。
	CustomCSS template.CSS
	// PrintCodeCSS は印刷用（@media print）のライトなコードハイライト CSS。
	PrintCodeCSS template.CSS
}

// renderReq は描画に必要な値（State から mutex 内でコピーされる）。
type renderReq struct {
	TabID       string
	FileName    string
	Content     string
	Dir         string
	Mode        string // "view" | "source"
	AllowRemote bool
	PolicyUnset bool // RemoteImagePolicy が未確認（""）か
	Empty       bool
}

// dialogVM は #dialog-host に出す確認ダイアログ用データ。
// Kind でテンプレートを切り替える: "remote"=外部画像確認 / "close-unsaved"=未保存クローズ確認 /
// "missing-file"=起動時の不在ファイル確認（再試行/スキップ）。
type dialogVM struct {
	Kind     string
	TabID    string
	FileName string
	Path     string // missing-file: 不在ファイルのフルパス（表示用）
	URL      string // link: 開く予定の外部 URL（表示用）
	Quit     bool   // close-unsaved: 終了ループ中か
}

// OpenedFile は OS のファイル選択で開いた1ファイル。
type OpenedFile struct {
	Path    string
	Name    string
	Content string
}

// Host は OS 連携（ネイティブダイアログ・ファイル書込）を web へ提供する。
// main の App（Wails runtime を持つ）がアダプタで実装する。web→main の循環参照を避けるための境界。
type Host interface {
	// OpenFilesDialog はネイティブの複数ファイル選択を開き、読み込んだファイルを返す（キャンセルは空）。
	OpenFilesDialog() ([]OpenedFile, error)
	// SaveFileDialog は保存先選択ダイアログを開き、選択パスを返す（キャンセルは ""）。
	SaveFileDialog(suggestedName string) (string, error)
	// WriteFile は内容を指定パスへ書き込む。
	WriteFile(path, content string) error
	// ReadmeMarkdown は同梱 README（バージョン付き、About 用）を返す。
	ReadmeMarkdown() string
	// LicenseMarkdown は同梱ライセンス文書を返す。
	LicenseMarkdown() string
	// ExportStyleDialog はスタイル書き出し用の保存先を返す（キャンセルは ""）。
	ExportStyleDialog(suggestedName string) (string, error)
	// ImportStyleDialog はスタイル読み込み用の選択ファイル内容を返す（キャンセルは ""）。
	ImportStyleDialog() (string, error)
	// SetEditMenuEnabled は手組み編集メニュー（Win/Linux）の編集専用項目（取り消し/やり直し/
	// 切り取り/貼り付け）の有効・無効を切り替える。アクティブタブが編集モードのとき canEdit=true。
	// macOS はネイティブ編集メニューが文脈で自動制御するため no-op。
	// サーバがモードを保持するため、フロント通知ではなくサーバ主導で本文描画のたびに同期する。
	SetEditMenuEnabled(canEdit bool)
	// Quit はアプリを終了する（終了ループで未保存タブをすべて処理し終えた後に呼ぶ）。
	Quit()
	// ReadFile は指定パスを読み込み、ファイル名と内容を返す（不在ファイルの「再試行」用）。
	// 読めなければ ok=false。
	ReadFile(path string) (name, content string, ok bool)
	// OpenURL は URL を OS の既定ブラウザ／メーラで開く（外部リンク確認後）。
	OpenURL(url string)
}

// Server は状態を保持し HTTP ハンドラを束ねる。
type Server struct {
	state *State
	host  Host
}

// NewHandler は Go-SSR の http.Handler を返す。
//   - GET /                   … アプリシェル（状態から描画）
//   - GET /tabs/{id}/view     … 指定タブの閲覧 HTML 断片
//   - POST /tabs/{id}/activate … アクティブ切替（本文＋タブバー OOB）
//   - GET /assets/...         … 同梱の静的アセット（htmx / glue.js / app.css）
func NewHandler(state *State, host Host) http.Handler {
	s := &Server{state: state, host: host}
	mux := http.NewServeMux()
	sub, err := fs.Sub(assetsFS, "assets")
	if err != nil {
		panic(err) // embed 済みのため通常起こらない
	}
	mux.Handle("GET /assets/", http.StripPrefix("/assets/", http.FileServer(http.FS(sub))))
	mux.HandleFunc("GET /{$}", s.serveShell)
	mux.HandleFunc("GET /tabs/{id}/view", s.serveView)
	mux.HandleFunc("POST /tabs/{id}/activate", s.serveActivate)
	mux.HandleFunc("POST /tabs/{id}/close", s.serveClose)
	mux.HandleFunc("POST /tabs/new", s.serveNew)
	mux.HandleFunc("POST /tabs/open", s.serveOpen)
	mux.HandleFunc("POST /tabs/open-path", s.serveOpenPath)
	mux.HandleFunc("POST /tabs/{id}/save", s.serveSave)
	mux.HandleFunc("POST /tabs/{id}/save-as", s.serveSaveAs)
	mux.HandleFunc("POST /active/save", s.serveActiveSave)
	mux.HandleFunc("POST /active/save-as", s.serveActiveSaveAs)
	mux.HandleFunc("POST /active/mode", s.serveActiveMode)
	mux.HandleFunc("POST /active/print", s.serveActivePrint)
	mux.HandleFunc("POST /tabs/{id}/mode", s.serveMode)
	mux.HandleFunc("POST /tabs/{id}/content", s.serveContent)
	mux.HandleFunc("POST /tabs/{id}/remote-images", s.serveRemoteImages)
	mux.HandleFunc("POST /sidebar/toggle", s.serveSidebarToggle)
	mux.HandleFunc("POST /find/open", s.serveFindOpen)
	mux.HandleFunc("POST /quit/request", s.serveQuitRequest)
	mux.HandleFunc("POST /quit/step", s.serveQuitStep)
	mux.HandleFunc("POST /missing/retry", s.serveMissingRetry)
	mux.HandleFunc("POST /missing/skip", s.serveMissingSkip)
	mux.HandleFunc("POST /link/confirm", s.serveLinkConfirm)
	mux.HandleFunc("POST /link/open", s.serveLinkOpen)
	mux.HandleFunc("POST /link/dismiss", s.serveLinkDismiss)
	mux.HandleFunc("POST /doc/about", s.serveAbout)
	mux.HandleFunc("POST /doc/license", s.serveLicense)
	mux.HandleFunc("POST /settings/toggle", s.serveSettingsToggle)
	mux.HandleFunc("POST /settings/style/{id}", s.serveSetStyle)
	mux.HandleFunc("POST /settings/duplicate", s.serveDuplicateStyle)
	mux.HandleFunc("POST /settings/field/{key}", s.serveField)
	mux.HandleFunc("POST /settings/heading/{n}/{key}", s.serveHeadingField)
	mux.HandleFunc("POST /settings/rename", s.serveRename)
	mux.HandleFunc("POST /settings/delete", s.serveDeleteStyle)
	mux.HandleFunc("POST /settings/export", s.serveExportStyle)
	mux.HandleFunc("POST /settings/import", s.serveImportStyle)
	return withCSP(mux)
}

// cspPolicy は WebView に適用する Content-Security-Policy。
//   - script-src 'self' 'unsafe-eval': 自己ホスト JS（htmx/mermaid/glue）のみ。mermaid/htmx が
//     Function コンストラクタを使うため 'unsafe-eval' が必要。外部・インラインスクリプトは禁止。
//   - style-src 'unsafe-inline': #styleblock 等のインライン <style> と mermaid の SVG インラインスタイル。
//   - img-src data: http: https:: ローカル画像の data URI と、許可済み外部画像（http/https）。
//   - connect-src 'self': htmx のリクエスト（プロセス内 http.Handler）。
//   - default-src 'none' で他は既定拒否。
const cspPolicy = "default-src 'none'; " +
	"script-src 'self' 'unsafe-eval'; " +
	"style-src 'self' 'unsafe-inline'; " +
	"img-src 'self' data: http: https:; " +
	"font-src 'self' data:; " +
	"connect-src 'self'; " +
	"object-src 'none'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'"

// withCSP は全レスポンスに CSP ヘッダを付与するミドルウェア（設計: docs/アーキテクチャ・画面設計.md §3）。
func withCSP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", cspPolicy)
		next.ServeHTTP(w, r)
	})
}

// renderContent は描画要求を本文ビューモデルに変換する（render パイプラインを呼ぶ）。
// リモート画像を含み、かつポリシー未確認なら確認ダイアログ（dialogVM）も返す。
// scheme は本文ラッパに出力し、glue.js の mermaid テーマ切替に使う。
func renderContent(req renderReq, scheme string) (contentVM, *dialogVM) {
	if req.Empty {
		return contentVM{Empty: true, Scheme: scheme, Mode: "view"}, nil
	}
	// 編集モード: textarea ＋ chroma ハイライト層。
	if req.Mode == "source" {
		return contentVM{
			Mode:        "source",
			Scheme:      scheme,
			TabID:       req.TabID,
			Raw:         req.Content,
			Highlighted: template.HTML(render.HighlightInner(req.Content, "markdown")),
		}, nil
	}
	// 閲覧モード。
	res, err := render.RenderMarkdown(req.Content, render.Options{BaseDir: req.Dir, AllowRemoteImages: req.AllowRemote})
	if err != nil {
		return contentVM{Body: template.HTML("<p>レンダリングに失敗しました。</p>"), Scheme: scheme, Mode: "view"}, nil
	}
	var dlg *dialogVM
	if res.HasRemoteImages && req.PolicyUnset {
		dlg = &dialogVM{Kind: "remote", TabID: req.TabID, FileName: req.FileName}
	}
	return contentVM{Body: template.HTML(res.HTML), Scheme: scheme, Mode: "view"}, dlg
}

// renderActive はアクティブタブを現在のスキームで描画する。
func (s *Server) renderActive() (contentVM, *dialogVM) {
	return renderContent(s.state.RenderReqActive(), s.state.ActiveStyle().ColorScheme)
}

// syncEditMenu はネイティブ編集メニュー（Win/Linux 手組み）の有効/無効をアクティブモードに同期する。
// アクティブ本文を描画する各所（シェル・nav・印刷）から呼ぶ。編集モード（"source"）でのみ有効化。
func (s *Server) syncEditMenu(mode string) {
	if s.host != nil {
		s.host.SetEditMenuEnabled(mode == "source")
	}
}

// settingsFields は VM に設定パネル用の情報（開閉・スタイル一覧・アクティブスタイル）を詰める。
func (s *Server) settingsFields(vm *shellVM) {
	active := s.state.ActiveStyle()
	vm.SettingsOpen = s.state.SettingsOpen()
	vm.ActiveStyle = active
	vm.ActiveEditable = !active.Builtin
	vm.FontOptions = style.FontOptions()
	vm.CodeFontOptions = style.CodeFontOptions()
	for _, p := range s.state.Styles() {
		vm.Styles = append(vm.Styles, styleOptVM{ID: p.ID, Name: p.Name, Builtin: p.Builtin, Active: p.ID == active.ID})
	}
}

func (s *Server) serveShell(w http.ResponseWriter, _ *http.Request) {
	st := s.state.ActiveStyle()
	content, dlg := renderContent(s.state.RenderReqActive(), st.ColorScheme)
	if mp := s.state.FirstMissing(); mp != "" {
		dlg = &dialogVM{Kind: "missing-file", Path: mp} // 不在ファイル確認を優先
	}
	mode, ro := s.state.ActiveMeta()
	s.syncEditMenu(mode)
	vm := shellVM{
		Title:          "Markmiru",
		Tabs:           s.state.TabVMs(),
		Content:        content,
		StyleVars:      template.CSS(style.CSS(st)),
		CodeCSS:        template.CSS(render.HighlightCSS(st.ColorScheme)),
		Scheme:         st.ColorScheme,
		Dialog:         dlg,
		SidebarOpen:    s.state.SidebarOpen(),
		ActiveID:       s.state.ActiveID(),
		ActiveMode:     mode,
		ActiveReadOnly: ro,
		CustomCSS:      template.CSS(st.CustomCSS),
		PrintCodeCSS:   template.CSS(render.HighlightCSS("light")),
	}
	s.settingsFields(&vm)
	htmlHeader(w)
	_ = shellTmpl.ExecuteTemplate(w, "shell.html", vm)
}

// navVM はタブ操作後の応答用 VM（本文＋タブバー＋サイドバー＋ダイアログを更新）。
func (s *Server) navVM() shellVM {
	content, dlg := s.renderActive()
	mode, ro := s.state.ActiveMeta()
	s.syncEditMenu(mode)
	return shellVM{
		Tabs:           s.state.TabVMs(),
		Content:        content,
		Dialog:         dlg,
		SidebarOpen:    s.state.SidebarOpen(),
		ActiveID:       s.state.ActiveID(),
		ActiveMode:     mode,
		ActiveReadOnly: ro,
	}
}

func (s *Server) serveView(w http.ResponseWriter, r *http.Request) {
	req, ok := s.state.RenderReq(r.PathValue("id"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	content, dlg := renderContent(req, s.state.ActiveStyle().ColorScheme)
	htmlHeader(w)
	_ = shellTmpl.ExecuteTemplate(w, "view", shellVM{Content: content, Dialog: dlg})
}

func (s *Server) serveActivate(w http.ResponseWriter, r *http.Request) {
	if !s.state.Activate(r.PathValue("id")) {
		http.NotFound(w, r)
		return
	}
	htmlHeader(w)
	_ = shellTmpl.ExecuteTemplate(w, "nav", s.navVM())
}

// serveMode は閲覧/編集モードを切り替え、本文＋タブバー（トグルボタン）を更新する。
func (s *Server) serveMode(w http.ResponseWriter, r *http.Request) {
	if !s.state.SetMode(r.PathValue("id")) {
		http.NotFound(w, r)
		return
	}
	htmlHeader(w)
	_ = shellTmpl.ExecuteTemplate(w, "nav", s.navVM())
}

// serveContent は編集中の本文を更新し、chroma ハイライト層（#hl-{id}）を差し替える。
// あわせてタブバーを OOB で更新し、未保存マーク（•）を入力と同時に反映する（dirty はサーバが保持）。
func (s *Server) serveContent(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	value := r.FormValue("value")
	if !s.state.UpdateContent(id, value) {
		http.NotFound(w, r)
		return
	}
	mode, ro := s.state.ActiveMeta()
	vm := shellVM{
		Tabs:           s.state.TabVMs(),
		ActiveID:       s.state.ActiveID(),
		ActiveMode:     mode,
		ActiveReadOnly: ro,
		Content: contentVM{
			TabID:       id,
			Highlighted: template.HTML(render.HighlightInner(value, "markdown")),
		},
	}
	htmlHeader(w)
	_ = shellTmpl.ExecuteTemplate(w, "edit-sync", vm)
}

func (s *Server) serveNew(w http.ResponseWriter, _ *http.Request) {
	s.state.NewUntitled()
	htmlHeader(w)
	_ = shellTmpl.ExecuteTemplate(w, "nav", s.navVM())
}

// serveClose はタブを閉じる。未保存なら確認ダイアログ（保存/破棄/キャンセル）を出す。
// choice 未指定: 未保存なら #dialog-host にダイアログを出し（HX-Retarget）、保存済みなら即クローズ。
// choice=save|discard|cancel: ダイアログの選択結果を処理して nav を返す（ダイアログはクリア）。
func (s *Server) serveClose(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	switch r.URL.Query().Get("choice") {
	case "":
		if s.state.IsDirty(id) {
			_, fileName, _, _ := s.state.SaveInfo(id)
			s.serveCloseDialog(w, &dialogVM{Kind: "close-unsaved", TabID: id, FileName: fileName})
			return
		}
		if !s.state.Close(id) {
			http.NotFound(w, r)
			return
		}
	case "save":
		if !s.saveTabForClose(id) {
			s.serveNav(w) // 保存先選択をキャンセル → 閉じない（ダイアログはクリア）
			return
		}
		s.state.Close(id)
	case "discard":
		s.state.Close(id)
	case "cancel":
		// 現状維持（nav でダイアログをクリア）
	}
	s.serveNav(w)
}

// serveCloseDialog は確認ダイアログを #dialog-host へ出す（HX-Retarget で本文は触らない）。
func (s *Server) serveCloseDialog(w http.ResponseWriter, dlg *dialogVM) {
	w.Header().Set("HX-Retarget", "#dialog-host")
	w.Header().Set("HX-Reswap", "innerHTML")
	htmlHeader(w)
	_ = shellTmpl.ExecuteTemplate(w, "dialog", dlg)
}

// saveTabForClose はタブを保存する（無題は保存ダイアログ）。保存できたら true、
// 保存先選択をキャンセル/失敗したら false（＝閉じない）。応答は書かない。
func (s *Server) saveTabForClose(id string) bool {
	path, fileName, content, ok := s.state.SaveInfo(id)
	if !ok {
		return false
	}
	if path == "" {
		if s.host == nil {
			return false
		}
		chosen, err := s.host.SaveFileDialog(suggestName(fileName))
		if err != nil || chosen == "" { // キャンセル
			return false
		}
		path = chosen
	}
	if s.host != nil {
		if err := s.host.WriteFile(path, content); err != nil {
			return false
		}
		s.state.MarkSaved(id, path)
	}
	return true
}

// serveMissingRetry は先頭の不在ファイルを再読込する。読めればタブを開いてアクティブ化し、
// リストから外して次へ。まだ読めなければリストに残して同じ確認を再表示する。
func (s *Server) serveMissingRetry(w http.ResponseWriter, _ *http.Request) {
	path := s.state.FirstMissing()
	if path != "" && s.host != nil {
		if name, content, ok := s.host.ReadFile(path); ok {
			tab := s.state.AddTab(path, name, content)
			s.state.Activate(tab.ID)
			s.state.RemoveMissing(path)
		}
	}
	s.serveMissingNext(w)
}

// serveMissingSkip は先頭の不在ファイルを諦めてリストから外し、次へ進む。
func (s *Server) serveMissingSkip(w http.ResponseWriter, _ *http.Request) {
	if path := s.state.FirstMissing(); path != "" {
		s.state.RemoveMissing(path)
	}
	s.serveMissingNext(w)
}

// serveMissingNext は次の不在ファイル確認を出す。無ければ通常 nav（ダイアログクリア）。
// いずれも本文・タブバーを更新するため nav で返す（再試行でタブが増えるため）。
func (s *Server) serveMissingNext(w http.ResponseWriter) {
	next := s.state.FirstMissing()
	if next == "" {
		s.serveNav(w)
		return
	}
	s.serveNavWithDialog(w, &dialogVM{Kind: "missing-file", Path: next})
}

// serveNavWithDialog は nav 応答に任意のダイアログを載せて返す（#dialog-host を OOB 更新）。
func (s *Server) serveNavWithDialog(w http.ResponseWriter, dlg *dialogVM) {
	vm := s.navVM()
	vm.Dialog = dlg
	htmlHeader(w)
	_ = shellTmpl.ExecuteTemplate(w, "nav", vm)
}

// isExternalURL は OS ブラウザ／メーラで開いてよいスキームか（http/https/mailto）を判定する。
func isExternalURL(u string) bool {
	l := strings.ToLower(strings.TrimSpace(u))
	return strings.HasPrefix(l, "http://") || strings.HasPrefix(l, "https://") || strings.HasPrefix(l, "mailto:")
}

// serveLinkConfirm はプレビュー内の外部リンククリックを受け、確認ダイアログを #dialog-host に出す。
// URL はサーバ側で保持し、「はい」で serveLinkOpen が開く（クエリ再エンコードの取り回しを避ける）。
func (s *Server) serveLinkConfirm(w http.ResponseWriter, r *http.Request) {
	url := r.FormValue("url")
	if !isExternalURL(url) {
		htmlHeader(w) // 不正スキームは開かず、ダイアログも出さない（#dialog-host は空のまま）
		return
	}
	s.state.SetPendingLink(url)
	htmlHeader(w)
	_ = shellTmpl.ExecuteTemplate(w, "dialog", &dialogVM{Kind: "link", URL: url})
}

// serveLinkOpen は保持中の外部リンクを OS ブラウザで開き、ダイアログを OOB でクリアする。
func (s *Server) serveLinkOpen(w http.ResponseWriter, _ *http.Request) {
	url := s.state.PendingLink()
	s.state.SetPendingLink("")
	if isExternalURL(url) && s.host != nil {
		s.host.OpenURL(url)
	}
	s.serveClearDialog(w)
}

// serveLinkDismiss は外部リンク確認をキャンセルし、ダイアログを OOB でクリアする。
func (s *Server) serveLinkDismiss(w http.ResponseWriter, _ *http.Request) {
	s.state.SetPendingLink("")
	s.serveClearDialog(w)
}

// serveClearDialog は #dialog-host を空に差し替える（OOB）。本文などは触らない。
func (s *Server) serveClearDialog(w http.ResponseWriter) {
	htmlHeader(w)
	_ = shellTmpl.ExecuteTemplate(w, "dialog-host-oob", shellVM{})
}

// serveQuitRequest はウィンドウを閉じる操作（app:request-quit）を受けて終了ループを開始する。
// 最初の未保存タブの確認ダイアログを出す（未保存が無ければ即終了）。
func (s *Server) serveQuitRequest(w http.ResponseWriter, _ *http.Request) {
	s.continueQuit(w)
}

// serveQuitStep は終了ループ1タブ分の選択（save/discard/cancel）を処理し、次の未保存タブへ進む。
// cancel および保存先選択のキャンセルは終了を中止する（残りのタブはそのまま）。
// 終了ループでは、対象タブを閉じずに解決する（save=保存して clean 化 / discard=quitResolved 印）。
// これにより、途中でキャンセルしても全タブが元の状態に戻り、開いていたファイルもセッションに残る。
func (s *Server) serveQuitStep(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	switch r.URL.Query().Get("choice") {
	case "save":
		if !s.saveTabForClose(id) {
			s.state.ClearQuitResolved()
			s.serveNav(w) // 保存先選択をキャンセル → 終了を中止（印も解除）
			return
		}
		// 保存で clean になるため、次の対象から自然に外れる。
	case "discard":
		s.state.MarkQuitResolved(id) // 閉じずに終了を許可（内容は保持）
	case "cancel":
		s.state.ClearQuitResolved()
		s.serveNav(w) // 終了を中止（全タブ元の状態へ）
		return
	}
	s.continueQuit(w)
}

// continueQuit は次の未確認・未保存タブがあればその確認ダイアログを出し、無ければアプリを終了する。
func (s *Server) continueQuit(w http.ResponseWriter) {
	next := s.state.FirstUnsavedUnresolvedID()
	if next == "" {
		if s.host != nil {
			s.host.Quit()
		}
		s.serveNav(w) // 閉じるまでの一瞬、表示を維持（ダイアログはクリア）
		return
	}
	_, fileName, _, _ := s.state.SaveInfo(next)
	s.serveCloseDialog(w, &dialogVM{Kind: "close-unsaved", TabID: next, FileName: fileName, Quit: true})
}

// serveFindOpen はページ内検索バーの断片を返す（#find-host に注入）。
// バーは純粋な UI でサーバ状態を持たない。走査・ハイライト・ナビ・件数表示は glue.js が担う
// （閲覧モードは .markdown-body、編集モードは .hl オーバーレイを対象にする）。
func (s *Server) serveFindOpen(w http.ResponseWriter, _ *http.Request) {
	htmlHeader(w)
	_ = shellTmpl.ExecuteTemplate(w, "find-bar", nil)
}

func (s *Server) serveSidebarToggle(w http.ResponseWriter, _ *http.Request) {
	s.state.ToggleSidebar()
	htmlHeader(w)
	_ = shellTmpl.ExecuteTemplate(w, "sidebar", shellVM{
		Tabs:        s.state.TabVMs(),
		SidebarOpen: s.state.SidebarOpen(),
	})
}

func (s *Server) serveNav(w http.ResponseWriter) {
	htmlHeader(w)
	_ = shellTmpl.ExecuteTemplate(w, "nav", s.navVM())
}

// serveOpen は OS のファイル選択を開き、選択ファイルをタブで開く（最後をアクティブ）。
func (s *Server) serveOpen(w http.ResponseWriter, _ *http.Request) {
	if s.host != nil {
		if files, err := s.host.OpenFilesDialog(); err == nil {
			var lastID string
			for _, f := range files {
				lastID = s.state.AddTab(f.Path, f.Name, f.Content).ID
			}
			if lastID != "" {
				s.state.Activate(lastID)
			}
		}
	}
	s.serveNav(w) // キャンセル・エラー時も現状を再描画
}

// serveOpenPath は指定パスのファイルを開く（単一インスタンス IPC・ファイル関連付けからの起動用）。
// 既に開いていれば再アクティブ化、無ければ読み込んでタブを追加しアクティブ化する。
func (s *Server) serveOpenPath(w http.ResponseWriter, r *http.Request) {
	path := r.FormValue("path")
	if path != "" && !s.state.ActivateByPath(path) && s.host != nil {
		if name, content, ok := s.host.ReadFile(path); ok {
			tab := s.state.AddTab(path, name, content)
			s.state.Activate(tab.ID)
		}
	}
	s.serveNav(w)
}

// serveSave はアクティブ/指定タブを保存する。無題（パス無し）なら保存ダイアログを開く。
func (s *Server) serveSave(w http.ResponseWriter, r *http.Request) {
	s.save(w, r, r.PathValue("id"), false)
}

// serveSaveAs は常に保存ダイアログを開いて保存する。
func (s *Server) serveSaveAs(w http.ResponseWriter, r *http.Request) {
	s.save(w, r, r.PathValue("id"), true)
}

// --- アクティブタブ対象の操作（ネイティブメニュー/キーボードからの glue ブリッジ用。
//     クライアントがアクティブ ID を知らなくて済む）。 ---

func (s *Server) serveActiveSave(w http.ResponseWriter, r *http.Request) {
	s.save(w, r, s.state.ActiveID(), false)
}

func (s *Server) serveActiveSaveAs(w http.ResponseWriter, r *http.Request) {
	s.save(w, r, s.state.ActiveID(), true)
}

func (s *Server) serveActiveMode(w http.ResponseWriter, r *http.Request) {
	s.state.SetMode(s.state.ActiveID()) // 読み取り専用は no-op
	s.serveNav(w)
}

// serveActivePrint は印刷向けにアクティブタブを閲覧モードにし、印刷トリガを返す（glue が window.print）。
// 印刷は印刷向け（ライト）配色にするため、mermaid をライトで描画させるべく本文を scheme=light で出す
// （コードは @media print の #print-code-theme がライトで上書きする）。
func (s *Server) serveActivePrint(w http.ResponseWriter, _ *http.Request) {
	s.state.SetViewMode(s.state.ActiveID())
	content, dlg := renderContent(s.state.RenderReqActive(), "light")
	mode, ro := s.state.ActiveMeta()
	s.syncEditMenu(mode)
	vm := shellVM{
		Tabs:           s.state.TabVMs(),
		Content:        content,
		Dialog:         dlg,
		SidebarOpen:    s.state.SidebarOpen(),
		ActiveID:       s.state.ActiveID(),
		ActiveMode:     mode,
		ActiveReadOnly: ro,
	}
	w.Header().Set("HX-Trigger-After-Settle", "do-print")
	htmlHeader(w)
	_ = shellTmpl.ExecuteTemplate(w, "nav", vm)
}

func (s *Server) save(w http.ResponseWriter, r *http.Request, id string, forceDialog bool) {
	path, fileName, content, ok := s.state.SaveInfo(id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if forceDialog || path == "" {
		if s.host == nil {
			s.serveNav(w)
			return
		}
		chosen, err := s.host.SaveFileDialog(suggestName(fileName))
		if err != nil || chosen == "" { // キャンセル
			s.serveNav(w)
			return
		}
		path = chosen
	}
	if s.host != nil {
		if err := s.host.WriteFile(path, content); err == nil {
			s.state.MarkSaved(id, path)
		}
	}
	s.serveNav(w)
}

// serveAbout は同梱 README を読み取り専用タブ（About 代わり）で開く。
func (s *Server) serveAbout(w http.ResponseWriter, _ *http.Request) {
	if s.host != nil {
		s.state.OpenDoc("Markmiru について", s.host.ReadmeMarkdown())
	}
	s.serveNav(w)
}

// serveLicense は同梱ライセンス文書を読み取り専用タブで開く。
func (s *Server) serveLicense(w http.ResponseWriter, _ *http.Request) {
	if s.host != nil {
		s.state.OpenDoc("ライセンス", s.host.LicenseMarkdown())
	}
	s.serveNav(w)
}

func (s *Server) serveSettingsToggle(w http.ResponseWriter, _ *http.Request) {
	s.state.ToggleSettings()
	var vm shellVM
	s.settingsFields(&vm)
	htmlHeader(w)
	_ = shellTmpl.ExecuteTemplate(w, "settings", vm)
}

func (s *Server) serveSetStyle(w http.ResponseWriter, r *http.Request) {
	if !s.state.SetActiveStyle(r.PathValue("id")) {
		http.NotFound(w, r)
		return
	}
	s.restyle(w)
}

func (s *Server) serveDuplicateStyle(w http.ResponseWriter, _ *http.Request) {
	s.state.DuplicateActiveStyle()
	s.restyle(w)
}

// serveField はアクティブな編集可能スタイルの1フィールドを更新する。
// フィールドの種類で応答（差し替え対象）を変える:
//   - colorScheme … コードテーマ/mermaid も変わるため restyle（本文再描画＋OOB）
//   - customCSS   … #customcss を差し替え
//   - それ以外    … #styleblock（CSS 変数）だけ差し替え＝本文再描画なしのライブプレビュー
func (s *Server) serveField(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	s.state.UpdateActiveStyleField(key, r.FormValue("value"))
	st := s.state.ActiveStyle()
	switch key {
	case "colorScheme":
		s.restyle(w)
	case "customCSS":
		htmlHeader(w)
		_ = shellTmpl.ExecuteTemplate(w, "customcss", shellVM{CustomCSS: template.CSS(st.CustomCSS)})
	default:
		htmlHeader(w)
		_ = shellTmpl.ExecuteTemplate(w, "styleblock", shellVM{StyleVars: template.CSS(style.CSS(st))})
	}
}

// serveHeadingField は見出し h{n+1} の1フィールドを更新し、#styleblock を差し替える。
func (s *Server) serveHeadingField(w http.ResponseWriter, r *http.Request) {
	n, err := strconv.Atoi(r.PathValue("n"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	s.state.UpdateActiveHeadingField(n, r.PathValue("key"), r.FormValue("value"))
	st := s.state.ActiveStyle()
	htmlHeader(w)
	_ = shellTmpl.ExecuteTemplate(w, "styleblock", shellVM{StyleVars: template.CSS(style.CSS(st))})
}

// serveRename はアクティブな編集可能スタイルを改名し、設定モーダルを差し替える。
func (s *Server) serveRename(w http.ResponseWriter, r *http.Request) {
	s.state.RenameActiveStyle(r.FormValue("value"))
	var vm shellVM
	s.settingsFields(&vm)
	htmlHeader(w)
	_ = shellTmpl.ExecuteTemplate(w, "settings", vm)
}

// serveDeleteStyle はアクティブな編集可能スタイルを削除し、light をアクティブにして再スタイルする。
func (s *Server) serveDeleteStyle(w http.ResponseWriter, _ *http.Request) {
	s.state.RemoveActiveStyle()
	s.restyle(w)
}

// serveExportStyle はアクティブスタイルを JSON でエクスポートする（OS 保存ダイアログ）。
func (s *Server) serveExportStyle(w http.ResponseWriter, _ *http.Request) {
	if s.host != nil {
		st := s.state.ActiveStyle()
		if js, err := style.SerializeStyle(st); err == nil {
			if path, err := s.host.ExportStyleDialog(safeFileName(st.Name) + ".json"); err == nil && path != "" {
				_ = s.host.WriteFile(path, js)
			}
		}
	}
	var vm shellVM
	s.settingsFields(&vm)
	htmlHeader(w)
	_ = shellTmpl.ExecuteTemplate(w, "settings", vm)
}

// serveImportStyle はスタイル JSON を読み込み、新規ユーザースタイルとして追加・アクティブ化する。
func (s *Server) serveImportStyle(w http.ResponseWriter, _ *http.Request) {
	if s.host != nil {
		if text, err := s.host.ImportStyleDialog(); err == nil && text != "" {
			if imp, err := style.ParseStyleFile(text); err == nil {
				s.state.AddImportedStyle(imp)
			}
			// 解析エラー時の通知ダイアログは仕上げで対応（現状は無視）。
		}
	}
	s.restyle(w)
}

// safeFileName はファイル名に使えない文字を _ に置換する。
func safeFileName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "style"
	}
	return strings.NewReplacer(
		`\`, "_", "/", "_", ":", "_", "*", "_", "?", "_", `"`, "_", "<", "_", ">", "_", "|", "_",
	).Replace(name)
}

// restyle はスタイル変更（選択/複製/colorScheme）後の応答: 本文を再描画（#content）し、
// styleblock・コードテーマ・上書き CSS・設定モーダルを OOB で更新する。
func (s *Server) restyle(w http.ResponseWriter) {
	st := s.state.ActiveStyle()
	content, _ := s.renderActive()
	vm := shellVM{
		Content:   content,
		StyleVars: template.CSS(style.CSS(st)),
		CodeCSS:   template.CSS(render.HighlightCSS(st.ColorScheme)),
		CustomCSS: template.CSS(st.CustomCSS),
	}
	s.settingsFields(&vm)
	htmlHeader(w)
	_ = shellTmpl.ExecuteTemplate(w, "restyle", vm)
}

// suggestName は保存ダイアログの既定ファイル名（.md 補完）。
func suggestName(fileName string) string {
	if strings.HasSuffix(strings.ToLower(fileName), ".md") {
		return fileName
	}
	return fileName + ".md"
}

// serveRemoteImages は外部画像の確認結果（choice=allow|block）を反映し、本文を再描画して返す。
func (s *Server) serveRemoteImages(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	policy := "block"
	if r.URL.Query().Get("choice") == "allow" {
		policy = "allow"
	}
	if !s.state.SetRemoteImagePolicy(id, policy) {
		http.NotFound(w, r)
		return
	}
	req, ok := s.state.RenderReq(id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	content, dlg := renderContent(req, s.state.ActiveStyle().ColorScheme) // ポリシー設定済み → dlg は nil（ダイアログをクリア）
	htmlHeader(w)
	_ = shellTmpl.ExecuteTemplate(w, "view", shellVM{Content: content, Dialog: dlg})
}

func htmlHeader(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
}
