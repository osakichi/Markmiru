// Package style は閲覧モードのスタイル（構造化データ）と CSS 変数への変換を担う。
//
// フロントの frontend/src/lib/style/styleDef.ts を Go へ移植したもの。JSON フィールド名は
// TS 側と完全一致させており（config の stylesJson 互換）、styleToVars / serializeStyle /
// parseStyleFile / normalizeStyle / プリセットの出力が一致する。
//
// 設計: docs/スタイル設定設計.md §1, §2, §6 / docs/Go中心化移行設計.md §8
package style

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// HeadingStyle は見出し h1〜h6 各レベルのスタイル。
type HeadingStyle struct {
	FontFamily   string  `json:"fontFamily"`
	FontSize     float64 `json:"fontSize"`   // px
	FontWeight   float64 `json:"fontWeight"` // 100〜900
	Color        string  `json:"color"`
	MarginTop    float64 `json:"marginTop"`    // em
	MarginBottom float64 `json:"marginBottom"` // em
	Border       bool    `json:"border"`       // 下線（水平線）の有無
}

// Style は1つのスタイル定義。数値は TS の number に合わせて float64。
type Style struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Builtin     bool   `json:"builtin"`
	ColorScheme string `json:"colorScheme"`

	// 本文
	FontFamily   string  `json:"fontFamily"`
	FontSize     float64 `json:"fontSize"` // px
	LineHeight   float64 `json:"lineHeight"`
	Color        string  `json:"color"`
	Background   string  `json:"background"`
	MaxWidth     float64 `json:"maxWidth"`     // px
	MaxWidthFull bool    `json:"maxWidthFull"` // true でウィンドウ幅追従

	// 見出し（h1〜h6、長さ6）
	Headings []HeadingStyle `json:"headings"`

	// リンク
	LinkColor     string `json:"linkColor"`
	LinkUnderline string `json:"linkUnderline"` // always / hover / none

	// リスト
	ListIndent     float64 `json:"listIndent"` // px
	MarkerColor    string  `json:"markerColor"`
	MarkerSize     float64 `json:"markerSize"`     // em
	MarkerPosition string  `json:"markerPosition"` // outside / inside

	// 引用
	QuoteColor       string  `json:"quoteColor"`
	QuoteBg          string  `json:"quoteBg"`
	QuoteBorder      string  `json:"quoteBorder"`
	QuoteBorderWidth float64 `json:"quoteBorderWidth"` // px
	QuoteItalic      bool    `json:"quoteItalic"`

	// コード
	CodeFontFamily string  `json:"codeFontFamily"`
	CodeBlockBg    string  `json:"codeBlockBg"`
	CodeFontSize   float64 `json:"codeFontSize"` // px
	CodeBg         string  `json:"codeBg"`

	// 水平線
	HrColor     string  `json:"hrColor"`
	HrThickness float64 `json:"hrThickness"` // px

	// 表
	BorderColor   string `json:"borderColor"`
	TableHeaderBg string `json:"tableHeaderBg"`
	RowOddBg      string `json:"rowOddBg"`
	RowEvenBg     string `json:"rowEvenBg"`

	CustomCSS string `json:"customCSS"`
}

// clone は headings スライスを含めて深いコピーを返す。
func (s Style) clone() Style {
	c := s
	c.Headings = append([]HeadingStyle(nil), s.Headings...)
	return c
}

// 同梱フォント（@fontsource）。styleDef.ts と同一。
const (
	SANS       = `"Noto Sans JP", -apple-system, BlinkMacSystemFont, "Segoe UI", "Hiragino Kaku Gothic ProN", "Yu Gothic UI", Meiryo, sans-serif`
	SERIF      = `"Noto Serif JP", "Hiragino Mincho ProN", "Yu Mincho", serif`
	MONO       = `"Noto Sans Mono", "Noto Sans JP", ui-monospace, SFMono-Regular, Menlo, Consolas, monospace`
	systemSans = `-apple-system, BlinkMacSystemFont, "Segoe UI", "Hiragino Kaku Gothic ProN", "Yu Gothic UI", Meiryo, sans-serif`
)

// num は number → CSS 値文字列（TS のテンプレートリテラルと同じ最短表現）。
func num(f float64) string {
	return strconv.FormatFloat(f, 'g', -1, 64)
}

// defaultHeadings は既定の見出し定義（色を与えて生成）。styleDef.ts と同一。
func defaultHeadings(color string) []HeadingStyle {
	sizes := []float64{32, 24, 20, 16, 14, 13}
	hs := make([]HeadingStyle, len(sizes))
	for i, fs := range sizes {
		hs[i] = HeadingStyle{
			FontFamily:   SANS,
			FontSize:     fs,
			FontWeight:   600,
			Color:        color,
			MarginTop:    1.5,
			MarginBottom: 0.6,
			Border:       i < 2, // h1, h2 に下線
		}
	}
	return hs
}

// KV は CSS 変数の1項目（順序を保つためスライスで持つ）。
type KV struct {
	Key   string
	Value string
}

// Vars は Style から CSS 変数（順序付き）を生成する。styleDef.ts の styleToVars と同一。
func Vars(p Style) []KV {
	maxWidth := "none"
	if !p.MaxWidthFull {
		maxWidth = num(p.MaxWidth) + "px"
	}
	linkDeco := "none"
	if p.LinkUnderline == "always" {
		linkDeco = "underline"
	}
	linkDecoHover := "underline"
	if p.LinkUnderline == "none" {
		linkDecoHover = "none"
	}
	quoteStyle := "normal"
	if p.QuoteItalic {
		quoteStyle = "italic"
	}
	vars := []KV{
		{"--md-font", p.FontFamily},
		{"--md-font-size", num(p.FontSize) + "px"},
		{"--md-line-height", num(p.LineHeight)},
		{"--md-color", p.Color},
		{"--md-bg", p.Background},
		{"--md-max-width", maxWidth},
		{"--md-link-color", p.LinkColor},
		{"--md-link-deco", linkDeco},
		{"--md-link-deco-hover", linkDecoHover},
		{"--md-list-indent", num(p.ListIndent) + "px"},
		{"--md-marker-color", p.MarkerColor},
		{"--md-marker-size", num(p.MarkerSize) + "em"},
		{"--md-list-position", p.MarkerPosition},
		{"--md-quote-color", p.QuoteColor},
		{"--md-quote-bg", p.QuoteBg},
		{"--md-quote-border", p.QuoteBorder},
		{"--md-quote-border-width", num(p.QuoteBorderWidth) + "px"},
		{"--md-quote-style", quoteStyle},
		{"--md-code-font", p.CodeFontFamily},
		{"--md-pre-bg", p.CodeBlockBg},
		{"--md-code-size", num(p.CodeFontSize) + "px"},
		{"--md-code-bg", p.CodeBg},
		{"--md-hr-color", p.HrColor},
		{"--md-hr-thickness", num(p.HrThickness) + "px"},
		{"--md-border", p.BorderColor},
		{"--md-th-bg", p.TableHeaderBg},
		{"--md-row-odd-bg", p.RowOddBg},
		{"--md-row-even-bg", p.RowEvenBg},
	}
	for i, h := range p.Headings {
		n := i + 1
		border := "none"
		if h.Border {
			border = "1px solid " + p.BorderColor
		}
		vars = append(vars,
			KV{fmt.Sprintf("--md-h%d-font", n), h.FontFamily},
			KV{fmt.Sprintf("--md-h%d-size", n), num(h.FontSize) + "px"},
			KV{fmt.Sprintf("--md-h%d-weight", n), num(h.FontWeight)},
			KV{fmt.Sprintf("--md-h%d-color", n), h.Color},
			KV{fmt.Sprintf("--md-h%d-mt", n), num(h.MarginTop) + "em"},
			KV{fmt.Sprintf("--md-h%d-mb", n), num(h.MarginBottom) + "em"},
			KV{fmt.Sprintf("--md-h%d-border", n), border},
		)
	}
	return vars
}

// CSS は Style を CSS 変数のインライン文字列（"k:v;k:v" 形式）にする。
func CSS(p Style) string {
	vars := Vars(p)
	var b strings.Builder
	for i, kv := range vars {
		if i > 0 {
			b.WriteByte(';')
		}
		b.WriteString(kv.Key)
		b.WriteByte(':')
		b.WriteString(kv.Value)
	}
	return b.String()
}

// PresetNameSuffix はプリセット表示名の接尾辞。
const PresetNameSuffix = " (プリセット)"

// BaseStyleName は表示名から末尾の「 (プリセット)」を除いた基底名を返す。
func BaseStyleName(name string) string {
	if strings.HasSuffix(name, PresetNameSuffix) {
		return name[:len(name)-len(PresetNameSuffix)]
	}
	return name
}

// Presets は標準プリセット（複製・編集できる雛形）を新しいスライスとして返す。
// styleDef.ts の PRESETS と同一値。呼び出しごとに独立したコピーを返す。
func Presets() []Style {
	return []Style{
		{
			ID: "light", Name: "ライト" + PresetNameSuffix, Builtin: true, ColorScheme: "light",
			FontFamily: SANS, FontSize: 16, LineHeight: 1.7, Color: "#24292f", Background: "#ffffff",
			MaxWidth: 820, MaxWidthFull: false, Headings: defaultHeadings("#1f2328"),
			LinkColor: "#0969da", LinkUnderline: "hover",
			ListIndent: 28, MarkerColor: "#24292f", MarkerSize: 1, MarkerPosition: "outside",
			QuoteColor: "#57606a", QuoteBg: "#f6f8fa", QuoteBorder: "#d0d7de", QuoteBorderWidth: 4, QuoteItalic: false,
			CodeFontFamily: MONO, CodeBlockBg: "#f6f8fa", CodeFontSize: 14, CodeBg: "#eff1f3",
			HrColor: "#d0d7de", HrThickness: 1,
			BorderColor: "#d0d7de", TableHeaderBg: "#f6f8fa", RowOddBg: "transparent", RowEvenBg: "#f6f8fa",
			CustomCSS: "",
		},
		{
			ID: "dark", Name: "ダーク" + PresetNameSuffix, Builtin: true, ColorScheme: "dark",
			FontFamily: SANS, FontSize: 16, LineHeight: 1.7, Color: "#c9d1d9", Background: "#0d1117",
			MaxWidth: 820, MaxWidthFull: false, Headings: defaultHeadings("#e6edf3"),
			LinkColor: "#4493f8", LinkUnderline: "hover",
			ListIndent: 28, MarkerColor: "#c9d1d9", MarkerSize: 1, MarkerPosition: "outside",
			QuoteColor: "#8b949e", QuoteBg: "#161b22", QuoteBorder: "#30363d", QuoteBorderWidth: 4, QuoteItalic: false,
			CodeFontFamily: MONO, CodeBlockBg: "#161b22", CodeFontSize: 14, CodeBg: "#161b22",
			HrColor: "#30363d", HrThickness: 1,
			BorderColor: "#30363d", TableHeaderBg: "#161b22", RowOddBg: "transparent", RowEvenBg: "#161b22",
			CustomCSS: "",
		},
		{
			ID: "github", Name: "GitHub 風" + PresetNameSuffix, Builtin: true, ColorScheme: "light",
			FontFamily: SANS, FontSize: 16, LineHeight: 1.6, Color: "#1f2328", Background: "#ffffff",
			MaxWidth: 980, MaxWidthFull: false, Headings: defaultHeadings("#1f2328"),
			LinkColor: "#0969da", LinkUnderline: "hover",
			ListIndent: 32, MarkerColor: "#1f2328", MarkerSize: 1, MarkerPosition: "outside",
			QuoteColor: "#59636e", QuoteBg: "#ffffff", QuoteBorder: "#d1d9e0", QuoteBorderWidth: 4, QuoteItalic: false,
			CodeFontFamily: MONO, CodeBlockBg: "#f6f8fa", CodeFontSize: 14, CodeBg: "rgba(175,184,193,0.2)",
			HrColor: "#d1d9e0", HrThickness: 1,
			BorderColor: "#d1d9e0", TableHeaderBg: "#f6f8fa", RowOddBg: "transparent", RowEvenBg: "#f6f8fa",
			CustomCSS: "",
		},
		{
			ID: "sepia", Name: "セピア" + PresetNameSuffix, Builtin: true, ColorScheme: "light",
			FontFamily: SANS, FontSize: 17, LineHeight: 1.8, Color: "#5b4636", Background: "#f4ecd8",
			MaxWidth: 760, MaxWidthFull: false, Headings: defaultHeadings("#43352a"),
			LinkColor: "#8a5a2b", LinkUnderline: "hover",
			ListIndent: 28, MarkerColor: "#5b4636", MarkerSize: 1, MarkerPosition: "outside",
			QuoteColor: "#6b5a47", QuoteBg: "#ece0c8", QuoteBorder: "#cbb994", QuoteBorderWidth: 4, QuoteItalic: false,
			CodeFontFamily: MONO, CodeBlockBg: "#ece0c8", CodeFontSize: 14, CodeBg: "#ece0c8",
			HrColor: "#cbb994", HrThickness: 1,
			BorderColor: "#cbb994", TableHeaderBg: "#ece0c8", RowOddBg: "transparent", RowEvenBg: "#e9dcc0",
			CustomCSS: "",
		},
	}
}

// lightBase は normalizeStyle の雛形（light プリセット）。
func lightBase() Style {
	return Presets()[0].clone()
}

// normalizeRaw は style の生 JSON を light プリセットへ重ねて補完する。
// Go の json.Unmarshal は「JSON に存在するフィールドのみ上書き」するため、
// TS の {...light, ...partial} と同じセマンティクスになる。
func normalizeRaw(raw json.RawMessage) (Style, error) {
	s := lightBase()
	if err := json.Unmarshal(raw, &s); err != nil {
		return Style{}, errors.New("スタイル形式ではありません。")
	}
	s.Builtin = false
	if len(s.Headings) != 6 {
		c := s.Color
		if c == "" {
			c = lightBase().Color
		}
		s.Headings = defaultHeadings(c)
	}
	return s, nil
}

// NormalizeStyle は欠損のあるスタイルを既定値で補完して整形する（復元時の安全策）。
func NormalizeStyle(s Style) Style {
	raw, _ := json.Marshal(s)
	out, _ := normalizeRaw(raw)
	return out
}

// エクスポート/インポートのファイル形式（メタ情報付き封筒）。設計: docs/スタイル設定設計.md §6
const (
	styleFileApp     = "Markmiru"
	styleFileKind    = "style"
	styleFileVersion = 1
)

type envelope struct {
	App     string `json:"app"`
	Kind    string `json:"kind"`
	Version int    `json:"version"`
	Style   Style  `json:"style"`
}

// SerializeStyle はスタイルを封筒に包んで整形済み JSON 文字列にする。
func SerializeStyle(s Style) (string, error) {
	env := envelope{App: styleFileApp, Kind: styleFileKind, Version: styleFileVersion, Style: s}
	b, err := json.MarshalIndent(env, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// ParseStyleFile は封筒形式の JSON を検証し、補完済み Style を返す。
// 形式不正時はエラー（呼び出し側でエラー表示）。ID 再割り当ては呼び出し側が行う。
func ParseStyleFile(text string) (Style, error) {
	var env struct {
		Kind  string          `json:"kind"`
		Style json.RawMessage `json:"style"`
	}
	if err := json.Unmarshal([]byte(text), &env); err != nil {
		return Style{}, errors.New("JSON として解釈できません。")
	}
	if env.Kind != styleFileKind || len(env.Style) == 0 {
		return Style{}, errors.New("Markmiru のスタイルではありません。")
	}
	return normalizeRaw(env.Style)
}

// --- 名前ヘルパ（[]Style に対する純関数。style.svelte.ts の対応ロジック） ---

// NameTaken は name が既存（exceptID 以外）と重複するか（前後空白を無視）。
func NameTaken(styles []Style, name, exceptID string) bool {
	n := strings.TrimSpace(name)
	for _, p := range styles {
		if p.ID != exceptID && strings.TrimSpace(p.Name) == n {
			return true
		}
	}
	return false
}

// FindByName は name で検索する（前後空白を無視）。
func FindByName(styles []Style, name string) (Style, bool) {
	n := strings.TrimSpace(name)
	for _, p := range styles {
		if strings.TrimSpace(p.Name) == n {
			return p, true
		}
	}
	return Style{}, false
}

// UniqueName は base を起点に既存と衝突しない一意な名前を返す（必要なら「 2」「 3」…）。
func UniqueName(styles []Style, base, exceptID string) string {
	b := strings.TrimSpace(base)
	if b == "" {
		b = "スタイル"
	}
	if !NameTaken(styles, b, exceptID) {
		return b
	}
	for i := 2; ; i++ {
		cand := fmt.Sprintf("%s %d", b, i)
		if !NameTaken(styles, cand, exceptID) {
			return cand
		}
	}
}
