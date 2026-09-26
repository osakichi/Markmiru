package web

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"Markmiru/style"
)

// Tab は開いている1ドキュメントの状態。
// 設計: docs/アーキテクチャ・画面設計.md §2
type Tab struct {
	ID                string
	FilePath          string // 空 = 無題（新規）
	FileName          string
	Content           string
	SavedContent      string
	Edited            bool   // 一度でも編集されたか（内容が元に戻っても未保存として扱う）
	Mode              string // "view" | "source"
	ReadOnly          bool
	RemoteImagePolicy string     // "" 未確認 / "allow" / "block"
	Format            textFormat // 元ファイルの BOM・改行コード（保存時に戻す。新規はゼロ値＝BOM なし・LF）
	quitResolved      bool       // 終了ループで確認済み（破棄選択）。終了を妨げない印。永続化しない
}

func (t *Tab) dir() string {
	if t.FilePath == "" {
		return ""
	}
	return filepath.Dir(t.FilePath)
}

// dirty は未保存かを返す。一度でも編集された（Edited）タブは、内容が保存済みと一致していても
// 未保存として扱う（誤って変更を失わないため）。
func (t *Tab) dirty() bool { return t.Edited || t.Content != t.SavedContent }

// State は単一ウィンドウのアプリ状態（タブ集合とアクティブタブ）。
// Wails のハンドラ呼び出しは並行し得るため mutex で保護する。
type State struct {
	mu       sync.Mutex
	tabs     []*Tab
	activeID string
	counter  int

	styles        []style.Style // プリセット＋ユーザースタイル
	activeStyleID string

	sidebarOpen  bool
	settingsOpen bool

	// pendingMissing は起動時に開けなかった（不在の）ファイルパス。シェル表示時に
	// 1件ずつ確認ダイアログ（再試行/スキップ）で処理する（§5.6）。
	pendingMissing []string

	// pendingLinkURL は外部リンク確認ダイアログで開く予定の URL（表示中のみ非空）。
	pendingLinkURL string

	// dropQueue はドラッグ&ドロップされたファイルのうち未処理のパス（先頭が処理中）。
	// 未保存の変更があるタブの再読み込み確認のように、途中でユーザーの選択を待つために持つ（§5.13）。
	dropQueue []string
}

// NewState はプリセットを読み込んだ初期状態を返す（アクティブスタイル = light、サイドバー開）。
func NewState() *State {
	return &State{
		styles:        style.Presets(),
		activeStyleID: "light",
		sidebarOpen:   true,
	}
}

// SidebarOpen はサイドバーの開閉状態を返す。
func (s *State) SidebarOpen() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sidebarOpen
}

// ToggleSidebar は開閉を切り替え、切替後の状態を返す。
func (s *State) ToggleSidebar() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sidebarOpen = !s.sidebarOpen
	return s.sidebarOpen
}

// NewUntitled は無題タブを追加してアクティブにする。
func (s *State) NewUntitled() *Tab {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.counter++
	t := &Tab{ID: fmt.Sprintf("t%d", s.counter), FileName: "無題", Mode: "source"} // 新規は編集モードで開く
	s.tabs = append(s.tabs, t)
	s.activeID = t.ID
	return t
}

// OpenDoc は読み取り専用タブ（About／ライセンス等）を開く。同名が既にあればそれをアクティブ化する。
// FilePath を持たないためセッション対象外・保存不可。
func (s *State) OpenDoc(name, content string) *Tab {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, t := range s.tabs {
		if t.ReadOnly && t.FileName == name {
			s.activeID = t.ID
			return t
		}
	}
	s.counter++
	t := &Tab{
		ID:           fmt.Sprintf("t%d", s.counter),
		FileName:     name,
		Content:      content,
		SavedContent: content,
		Mode:         "view",
		ReadOnly:     true,
	}
	s.tabs = append(s.tabs, t)
	s.activeID = t.ID
	return t
}

// Close はタブを閉じる。アクティブを閉じた場合は隣接タブをアクティブにする。
func (s *State) Close(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx := -1
	for i, t := range s.tabs {
		if t.ID == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		return false
	}
	wasActive := s.tabs[idx].ID == s.activeID
	s.tabs = append(s.tabs[:idx], s.tabs[idx+1:]...)
	if wasActive {
		switch {
		case len(s.tabs) == 0:
			s.activeID = ""
		case idx < len(s.tabs):
			s.activeID = s.tabs[idx].ID // 閉じた位置の次（同じ index）
		default:
			s.activeID = s.tabs[len(s.tabs)-1].ID // 末尾を閉じたら前
		}
	}
	return true
}

// IsDirty はタブが未保存（dirty: Edited または Content != SavedContent）かを返す。読み取り専用・不在は false。
func (s *State) IsDirty(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := s.find(id)
	return t != nil && !t.ReadOnly && t.dirty()
}

// HasUnsaved は未保存タブが1つでもあるかを返す（終了時の確認判定に使う）。
func (s *State) HasUnsaved() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, t := range s.tabs {
		if !t.ReadOnly && t.dirty() {
			return true
		}
	}
	return false
}

// FirstUnsavedUnresolvedID は、終了ループでまだ確認していない未保存タブの ID を返す（無ければ ""）。
// 終了中に「破棄」した（quitResolved）タブは閉じずに印だけ付け、次の対象から除外する。
func (s *State) FirstUnsavedUnresolvedID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, t := range s.tabs {
		if !t.ReadOnly && t.dirty() && !t.quitResolved {
			return t.ID
		}
	}
	return ""
}

// MarkQuitResolved は終了ループでタブを「破棄（保存せず終了を許可）」として印を付ける。
// タブは閉じず内容も保持するため、終了をキャンセルすれば元の未保存状態に戻せる。
func (s *State) MarkQuitResolved(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if t := s.find(id); t != nil {
		t.quitResolved = true
	}
}

// ClearQuitResolved は全タブの終了確認印を解除する（終了をキャンセルしたとき）。
func (s *State) ClearQuitResolved() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, t := range s.tabs {
		t.quitResolved = false
	}
}

// SetSidebarOpen はサイドバー開閉状態を設定する（セッション復元用）。
func (s *State) SetSidebarOpen(open bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sidebarOpen = open
}

// SessionFiles は復元対象（パスを持つ通常タブ）のパス列と、その中でのアクティブ位置を返す。
// 無題・読み取り専用（About/ライセンス）は対象外。
func (s *State) SessionFiles() (files []string, activeIndex int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	activeIndex = -1
	for _, t := range s.tabs {
		if t.FilePath == "" || t.ReadOnly {
			continue
		}
		if t.ID == s.activeID {
			activeIndex = len(files)
		}
		files = append(files, t.FilePath)
	}
	return files, activeIndex
}

// UserStylesJSON はユーザー定義（非 builtin）スタイルの JSON 配列を返す（設定永続化用）。無ければ ""。
func (s *State) UserStylesJSON() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var user []style.Style
	for _, p := range s.styles {
		if !p.Builtin {
			user = append(user, p)
		}
	}
	if len(user) == 0 {
		return ""
	}
	data, err := json.Marshal(user)
	if err != nil {
		return ""
	}
	return string(data)
}

// RestoreStylesJSON はユーザースタイル JSON 配列を復元して追加し、activeID が存在すればアクティブにする
// （セッション復元用）。ID は保持し、採番カウンタを衝突しない値へ進める。
func (s *State) RestoreStylesJSON(stylesJSON, activeID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if stylesJSON != "" {
		var userStyles []style.Style
		if err := json.Unmarshal([]byte(stylesJSON), &userStyles); err == nil {
			for _, us := range userStyles {
				us = style.NormalizeStyle(us) // Builtin=false・見出し6件補完
				if us.ID == "" {
					s.counter++
					us.ID = fmt.Sprintf("user-%d", s.counter)
				} else if n := userIDNum(us.ID); n > s.counter {
					s.counter = n
				}
				s.styles = append(s.styles, us)
			}
		}
	}
	if activeID != "" {
		for _, p := range s.styles {
			if p.ID == activeID {
				s.activeStyleID = activeID
				break
			}
		}
	}
}

// SetPendingMissing は起動時に開けなかったファイルパス群を保留する（確認ダイアログ用）。
func (s *State) SetPendingMissing(paths []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pendingMissing = paths
}

// FirstMissing は保留中の不在ファイルの先頭パスを返す（無ければ ""）。
func (s *State) FirstMissing() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.pendingMissing) == 0 {
		return ""
	}
	return s.pendingMissing[0]
}

// RemoveMissing は保留リストから指定パスを取り除く（スキップ／再オープン成功時）。
func (s *State) RemoveMissing(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.pendingMissing[:0]
	for _, p := range s.pendingMissing {
		if p != path {
			out = append(out, p)
		}
	}
	s.pendingMissing = out
}

// SetPendingLink は外部リンク確認ダイアログで開く予定の URL を保持する（"" でクリア）。
func (s *State) SetPendingLink(url string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pendingLinkURL = url
}

// PendingLink は保持中の外部リンク URL を返す（無ければ ""）。
func (s *State) PendingLink() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pendingLinkURL
}

// userIDNum は "user-N" 形式の ID から N を取り出す（該当しなければ 0）。採番カウンタの衝突回避に使う。
func userIDNum(id string) int {
	if !strings.HasPrefix(id, "user-") {
		return 0
	}
	n, _ := strconv.Atoi(id[len("user-"):])
	return n
}

// ActiveStyle は現在アクティブなスタイルを返す（無ければ先頭）。
func (s *State) ActiveStyle() style.Style {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.activeStyleLocked()
}

func (s *State) activeStyleLocked() style.Style {
	for _, p := range s.styles {
		if p.ID == s.activeStyleID {
			return p
		}
	}
	if len(s.styles) > 0 {
		return s.styles[0]
	}
	return style.Style{}
}

// activePtrLocked はアクティブスタイルへのポインタ（mutex 保持中に呼ぶ）。
func (s *State) activePtrLocked() *style.Style {
	for i := range s.styles {
		if s.styles[i].ID == s.activeStyleID {
			return &s.styles[i]
		}
	}
	return nil
}

// Styles はスタイル一覧のコピーを返す（設定パネルのセレクタ用）。
func (s *State) Styles() []style.Style {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]style.Style(nil), s.styles...)
}

// SettingsOpen は設定モーダルの開閉状態を返す（SidebarOpen と同様）。
func (s *State) SettingsOpen() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.settingsOpen
}

func (s *State) ToggleSettings() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.settingsOpen = !s.settingsOpen
	return s.settingsOpen
}

// SetActiveStyle はアクティブスタイルを切り替える（存在しない id は false）。
func (s *State) SetActiveStyle(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.styles {
		if p.ID == id {
			s.activeStyleID = id
			return true
		}
	}
	return false
}

// DuplicateActiveStyle はアクティブスタイルを複製して編集可能なユーザースタイルにし、アクティブ化する。
func (s *State) DuplicateActiveStyle() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	src := s.activePtrLocked()
	if src == nil {
		return false
	}
	s.counter++
	cp := src.Clone()
	cp.ID = fmt.Sprintf("user-%d", s.counter)
	cp.Name = style.UniqueName(s.styles, style.BaseStyleName(src.Name)+" のコピー", "")
	cp.Builtin = false
	s.styles = append(s.styles, cp)
	s.activeStyleID = cp.ID
	return true
}

// UpdateActiveStyleField はアクティブな（編集可能な）スタイルの1フィールドを更新する。
func (s *State) UpdateActiveStyleField(key, value string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.activePtrLocked()
	if p == nil || p.Builtin {
		return false
	}
	num := func() float64 { f, _ := strconv.ParseFloat(value, 64); return f }
	b := value == "true"
	switch key {
	// 本文
	case "fontFamily":
		p.FontFamily = value
	case "fontSize":
		p.FontSize = num()
	case "lineHeight":
		p.LineHeight = num()
	case "color":
		p.Color = value
	case "background":
		p.Background = value
	case "maxWidth":
		p.MaxWidth = num()
	case "maxWidthFull":
		p.MaxWidthFull = b
	case "colorScheme":
		p.ColorScheme = value
	// リンク
	case "linkColor":
		p.LinkColor = value
	case "linkUnderline":
		p.LinkUnderline = value
	// リスト
	case "listIndent":
		p.ListIndent = num()
	case "markerColor":
		p.MarkerColor = value
	case "markerSize":
		p.MarkerSize = num()
	case "markerPosition":
		p.MarkerPosition = value
	// 引用
	case "quoteColor":
		p.QuoteColor = value
	case "quoteBg":
		p.QuoteBg = value
	case "quoteBorder":
		p.QuoteBorder = value
	case "quoteBorderWidth":
		p.QuoteBorderWidth = num()
	case "quoteItalic":
		p.QuoteItalic = b
	// コード
	case "codeFontFamily":
		p.CodeFontFamily = value
	case "codeBlockBg":
		p.CodeBlockBg = value
	case "codeFontSize":
		p.CodeFontSize = num()
	case "codeBg":
		p.CodeBg = value
	// 水平線
	case "hrColor":
		p.HrColor = value
	case "hrThickness":
		p.HrThickness = num()
	// 表
	case "borderColor":
		p.BorderColor = value
	case "tableHeaderBg":
		p.TableHeaderBg = value
	case "rowOddBg":
		p.RowOddBg = value
	case "rowEvenBg":
		p.RowEvenBg = value
	// 上書き CSS
	case "customCSS":
		p.CustomCSS = value
	default:
		return false
	}
	return true
}

// UpdateActiveHeadingField は見出し（index=0..5 が h1..h6）の1フィールドを更新する。
func (s *State) UpdateActiveHeadingField(index int, key, value string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.activePtrLocked()
	if p == nil || p.Builtin || index < 0 || index >= len(p.Headings) {
		return false
	}
	h := &p.Headings[index]
	num := func() float64 { f, _ := strconv.ParseFloat(value, 64); return f }
	switch key {
	case "fontFamily":
		h.FontFamily = value
	case "fontSize":
		h.FontSize = num()
	case "fontWeight":
		h.FontWeight = num()
	case "color":
		h.Color = value
	case "marginTop":
		h.MarginTop = num()
	case "marginBottom":
		h.MarginBottom = num()
	case "border":
		h.Border = value == "true"
	default:
		return false
	}
	return true
}

// RenameActiveStyle はアクティブな編集可能スタイルを改名する（空・重複は拒否し false）。
func (s *State) RenameActiveStyle(name string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.activePtrLocked()
	if p == nil || p.Builtin {
		return false
	}
	n := strings.TrimSpace(name)
	if n == "" || style.NameTaken(s.styles, n, p.ID) {
		return false
	}
	p.Name = n
	return true
}

// RemoveActiveStyle はアクティブな編集可能スタイルを削除し、light をアクティブにする。
func (s *State) RemoveActiveStyle() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.activePtrLocked()
	if p == nil || p.Builtin {
		return false
	}
	id := p.ID
	out := s.styles[:0]
	for _, x := range s.styles {
		if x.ID != id {
			out = append(out, x)
		}
	}
	s.styles = out
	s.activeStyleID = "light"
	return true
}

// AddImportedStyle はインポートしたスタイルを新規ユーザースタイルとして追加・アクティブ化する。
// ID は新規採番、名前は一意化、builtin は false。
func (s *State) AddImportedStyle(imp style.Style) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.counter++
	imp.ID = fmt.Sprintf("user-%d", s.counter)
	imp.Name = style.UniqueName(s.styles, strings.TrimSpace(imp.Name), "")
	imp.Builtin = false
	s.styles = append(s.styles, imp)
	s.activeStyleID = imp.ID
}

// AddTab はファイルから読んだ内容（raw）でタブを追加して返す。最初のタブは自動的にアクティブになる。
// 本文は BOM を除き改行を LF にそろえ、BOM・改行コードは Format に記録して保存時に戻す（textformat.go）。
func (s *State) AddTab(filePath, fileName, raw string) *Tab {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.counter++
	content, format := decodeText(raw)
	t := &Tab{
		ID:           fmt.Sprintf("t%d", s.counter),
		FilePath:     filePath,
		FileName:     fileName,
		Content:      content,
		SavedContent: content,
		Mode:         "view",
		Format:       format,
	}
	s.tabs = append(s.tabs, t)
	if s.activeID == "" {
		s.activeID = t.ID
	}
	return t
}

// Activate はアクティブタブを切り替える。存在しない id は false。
func (s *State) Activate(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.find(id) == nil {
		return false
	}
	s.activeID = id
	return true
}

// ActivateByPath は指定パスのタブが既にあればアクティブ化して true を返す（IPC/ファイル関連付け用）。
func (s *State) ActivateByPath(path string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, t := range s.tabs {
		if t.FilePath == path {
			s.activeID = t.ID
			return true
		}
	}
	return false
}

// TabIDByPath は指定パスのファイルを開いているタブの ID を返す（無ければ ""）。
func (s *State) TabIDByPath(path string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, t := range s.tabs {
		if t.FilePath == path {
			return t.ID
		}
	}
	return ""
}

// SetDropQueue はドロップされたファイルのパス列を処理待ちとして保持する（前回の残りは捨てる）。
func (s *State) SetDropQueue(paths []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dropQueue = append([]string(nil), paths...)
}

// DropHead は処理中（先頭）のドロップされたファイルのパスを返す（無ければ ""）。
func (s *State) DropHead() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.dropQueue) == 0 {
		return ""
	}
	return s.dropQueue[0]
}

// PopDrop は処理中（先頭）のパスを処理済みとして取り除く。
func (s *State) PopDrop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.dropQueue) > 0 {
		s.dropQueue = s.dropQueue[1:]
	}
}

// SetRemoteImagePolicy はタブのリモート画像ポリシー（"allow"/"block"）を設定する。
func (s *State) SetRemoteImagePolicy(id, policy string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := s.find(id)
	if t == nil {
		return false
	}
	t.RemoteImagePolicy = policy
	return true
}

// ActiveID は現在のアクティブタブ ID を返す（無ければ ""）。
func (s *State) ActiveID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.activeID
}

// SaveInfo は保存に必要なタブ情報を返す。
func (s *State) SaveInfo(id string) (filePath, fileName, content string, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := s.find(id)
	if t == nil {
		return "", "", "", false
	}
	return t.FilePath, t.FileName, t.Content, true
}

// MarkSaved は保存後のパス・名前を反映し、savedContent を「実際にファイルへ書き込んだ内容」
// （saved）に更新する。書き込み I/O 中に編集が届き現在の Content が saved と異なる場合は
// dirty のまま残す——現在値を保存済みとみなすと、書き込まれていない編集が clean 扱いになり
// 再保存もできず終了確認も出ない（サイレント消失）ため。
func (s *State) MarkSaved(id, path, saved string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := s.find(id)
	if t == nil {
		return false
	}
	t.FilePath = path
	t.FileName = filepath.Base(path)
	t.SavedContent = saved
	if t.Content == saved {
		t.Edited = false // 書き込んだ内容から変化していなければ clean に戻す
	}
	return true
}

// ReloadInfo は再読み込み（§5.4）に必要なタブ情報を返す。ファイルを持たない無題・読み取り専用
// （About/ライセンス）・不在のタブは ok=false（再読み込みできない）。dirty は未保存の変更があるか
// （再読み込みで破棄される変更の有無＝確認ダイアログを出すか）。
func (s *State) ReloadInfo(id string) (path, fileName string, dirty, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := s.find(id)
	if t == nil || t.ReadOnly || t.FilePath == "" {
		return "", "", false, false
	}
	return t.FilePath, t.FileName, t.dirty(), true
}

// Reload はファイルから読み直した内容（raw）でタブの本文を置き換え、保存済み（clean）にする。
// 未保存の変更は破棄される（呼び出し側が確認済みであること）。モード（閲覧/編集）と
// 外部画像の表示ポリシーは維持する（同じファイルを読み直すだけのため）。BOM・改行コードは
// 読み直したファイルのものに更新する（外部のエディタで書式ごと変えられた場合に合わせる）。
func (s *State) Reload(id, raw string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := s.find(id)
	if t == nil || t.ReadOnly || t.FilePath == "" {
		return false
	}
	content, format := decodeText(raw)
	t.Content = content
	t.SavedContent = content
	t.Format = format
	t.Edited = false
	return true
}

// EncodeForSave は本文をタブの元ファイルの書式（BOM・改行コード）に戻した保存用の内容を返す。
// 新規作成（無題）のタブは BOM なし・LF。タブが無ければ本文をそのまま返す。
func (s *State) EncodeForSave(id, content string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if t := s.find(id); t != nil {
		return t.Format.encode(content)
	}
	return content
}

func (s *State) find(id string) *Tab {
	for _, t := range s.tabs {
		if t.ID == id {
			return t
		}
	}
	return nil
}

// TabVMs はタブバー描画用のビューモデル列を返す。
func (s *State) TabVMs() []tabVM {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]tabVM, len(s.tabs))
	for i, t := range s.tabs {
		out[i] = tabVM{ID: t.ID, FileName: t.FileName, Active: t.ID == s.activeID, Dirty: t.dirty()}
	}
	return out
}

// RenderReqActive はアクティブタブの描画要求を返す（無ければ Empty）。
func (s *State) RenderReqActive() renderReq {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := s.find(s.activeID)
	if t == nil {
		return renderReq{Empty: true}
	}
	return reqOf(t)
}

// RenderReq は指定タブの描画要求を返す（無ければ ok=false）。
func (s *State) RenderReq(id string) (renderReq, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := s.find(id)
	if t == nil {
		return renderReq{}, false
	}
	return reqOf(t), true
}

// reqOf は Tab の描画に必要な値を mutex 内でコピーする（描画はロック外で行うため）。
func reqOf(t *Tab) renderReq {
	return renderReq{
		TabID:       t.ID,
		FileName:    t.FileName,
		Content:     t.Content,
		Dir:         t.dir(),
		Mode:        t.Mode,
		AllowRemote: t.RemoteImagePolicy == "allow",
		PolicyUnset: t.RemoteImagePolicy == "",
	}
}

// SetMode はタブの閲覧/編集モードを切り替える（トグル）。読み取り専用タブは変更しない。
func (s *State) SetMode(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := s.find(id)
	if t == nil || t.ReadOnly {
		return false
	}
	if t.Mode == "source" {
		t.Mode = "view"
	} else {
		t.Mode = "source"
	}
	return true
}

// SetViewMode はタブを閲覧モードにする（印刷前などに使う）。
func (s *State) SetViewMode(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if t := s.find(id); t != nil {
		t.Mode = "view"
	}
}

// UpdateContent は編集中の本文を更新する（一度でも変われば Edited を立て、以降は dirty 扱い）。
func (s *State) UpdateContent(id, content string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := s.find(id)
	if t == nil || t.ReadOnly {
		return false
	}
	if content != t.Content {
		t.Content = content
		t.Edited = true // 一度編集したら、後で元に戻しても未保存扱いを維持する
	}
	return true
}

// MenuStates はネイティブメニューの有効条件を 1 回のロックでまとめて返す。
//   - canEdit: アクティブタブが編集モード（手組み編集メニューの編集専用項目用）
//   - canSave: アクティブタブが dirty、または保存先未定の無題（「保存」メニュー用。
//     無題は初回保存＝保存ダイアログに繋がるため、未編集でも保存可能とする）
//   - canToggle: 閲覧/編集を切り替えられるか（「表示 → 閲覧/編集切替」メニュー用）
//   - canReload: ファイルから読み直せるか（「ファイル → 再読み込み」メニュー用。無題は不可）
//
// 読み取り専用タブ（About/ライセンス。FilePath は空）とタブ無しはすべて無効。
func (s *State) MenuStates() (canEdit, canSave, canToggle, canReload bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := s.find(s.activeID)
	if t == nil || t.ReadOnly {
		return false, false, false, false
	}
	return t.Mode == "source", t.dirty() || t.FilePath == "", true, t.FilePath != ""
}

// CanReloadActive はアクティブタブを再読み込みできるか（右クリックメニューの活性判定用）。
func (s *State) CanReloadActive() bool {
	_, _, _, canReload := s.MenuStates()
	return canReload
}

// ActiveMeta はアクティブタブのモードと読み取り専用フラグを返す。
func (s *State) ActiveMeta() (mode string, readOnly bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := s.find(s.activeID)
	if t == nil {
		return "view", false
	}
	return t.Mode, t.ReadOnly
}
