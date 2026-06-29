package style

import (
	"encoding/json"
	"strings"
	"testing"
)

// varsMap は順序付き Vars を検索しやすい map に変換する。
func varsMap(p Style) map[string]string {
	m := map[string]string{}
	for _, kv := range Vars(p) {
		m[kv.Key] = kv.Value
	}
	return m
}

func TestPresets(t *testing.T) {
	ps := Presets()
	if len(ps) != 4 {
		t.Fatalf("expected 4 presets, got %d", len(ps))
	}
	wantIDs := []string{"light", "dark", "github", "sepia"}
	for i, id := range wantIDs {
		if ps[i].ID != id {
			t.Errorf("preset %d id=%q want %q", i, ps[i].ID, id)
		}
		if !ps[i].Builtin {
			t.Errorf("preset %q must be builtin", id)
		}
		if len(ps[i].Headings) != 6 {
			t.Errorf("preset %q must have 6 headings", id)
		}
	}
	if ps[0].Color != "#24292f" || ps[0].Background != "#ffffff" {
		t.Errorf("light preset colors mismatch: %q / %q", ps[0].Color, ps[0].Background)
	}
	// 返すたびに独立コピー（headings 変更が他に波及しない）。
	ps[0].Headings[0].Color = "#000000"
	if Presets()[0].Headings[0].Color == "#000000" {
		t.Errorf("Presets() must return independent copies")
	}
}

func TestVarsLight(t *testing.T) {
	m := varsMap(Presets()[0]) // light
	want := map[string]string{
		"--md-font-size":       "16px",
		"--md-line-height":     "1.7",
		"--md-color":           "#24292f",
		"--md-bg":              "#ffffff",
		"--md-max-width":       "820px",
		"--md-link-deco":       "none",      // hover → 通常は none
		"--md-link-deco-hover": "underline", // hover → ホバーで underline
		"--md-marker-size":     "1em",
		"--md-quote-style":     "normal",
		"--md-code-size":       "14px",
		"--md-h1-size":         "32px",
		"--md-h1-weight":       "600",
		"--md-h1-mt":           "1.5em",
		"--md-h1-mb":           "0.6em",
		"--md-h1-border":       "1px solid #d0d7de", // h1 は下線あり
		"--md-h3-border":       "none",              // h3 は下線なし
		"--md-h6-size":         "13px",
	}
	for k, v := range want {
		if got := m[k]; got != v {
			t.Errorf("%s = %q, want %q", k, got, v)
		}
	}
}

func TestVarsMaxWidthFull(t *testing.T) {
	p := Presets()[0]
	p.MaxWidthFull = true
	if varsMap(p)["--md-max-width"] != "none" {
		t.Errorf("maxWidthFull should yield none")
	}
}

func TestVarsLinkUnderlineModes(t *testing.T) {
	p := Presets()[0]
	p.LinkUnderline = "always"
	m := varsMap(p)
	if m["--md-link-deco"] != "underline" || m["--md-link-deco-hover"] != "underline" {
		t.Errorf("always: %q / %q", m["--md-link-deco"], m["--md-link-deco-hover"])
	}
	p.LinkUnderline = "none"
	m = varsMap(p)
	if m["--md-link-deco"] != "none" || m["--md-link-deco-hover"] != "none" {
		t.Errorf("none: %q / %q", m["--md-link-deco"], m["--md-link-deco-hover"])
	}
}

func TestCSSStringOrderAndJoin(t *testing.T) {
	css := CSS(Presets()[0])
	if !strings.HasPrefix(css, "--md-font:") {
		t.Errorf("CSS should start with --md-font, got: %.40s", css)
	}
	if strings.Contains(css, ";;") || strings.HasSuffix(css, ";") {
		t.Errorf("CSS join malformed: %.80s", css)
	}
}

func TestSerializeParseRoundTrip(t *testing.T) {
	src := Presets()[1] // dark
	src.ID = "user-1"
	src.Name = "マイダーク"
	src.Builtin = false
	js, err := SerializeStyle(src)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(js, `"kind": "style"`) || !strings.Contains(js, `"app": "Markmiru"`) {
		t.Errorf("envelope missing meta: %s", js)
	}
	got, err := ParseStyleFile(js)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "マイダーク" || got.Color != src.Color || got.ColorScheme != "dark" {
		t.Errorf("round trip mismatch: %+v", got)
	}
	if got.Builtin {
		t.Errorf("parsed style must not be builtin")
	}
}

func TestParseInvalid(t *testing.T) {
	if _, err := ParseStyleFile("not json"); err == nil {
		t.Errorf("expected error for invalid JSON")
	}
	if _, err := ParseStyleFile(`{"kind":"other","style":{}}`); err == nil {
		t.Errorf("expected error for wrong kind")
	}
}

func TestNormalizePartialMergesLightBase(t *testing.T) {
	// fontSize/headings 等を欠いた部分スタイル → light の既定で補完される。
	file := `{"app":"Markmiru","kind":"style","version":1,"style":{"name":"部分","color":"#111111"}}`
	got, err := ParseStyleFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "部分" || got.Color != "#111111" {
		t.Errorf("provided fields should win: %+v", got)
	}
	if got.FontSize != 16 || got.LineHeight != 1.7 {
		t.Errorf("missing fields should default from light: fontSize=%v lineHeight=%v", got.FontSize, got.LineHeight)
	}
	if len(got.Headings) != 6 {
		t.Errorf("headings should be regenerated to 6, got %d", len(got.Headings))
	}
}

func TestJSONFieldNamesMatchTS(t *testing.T) {
	// camelCase のキー名がフロント（config の stylesJson）と一致すること。
	b, _ := json.Marshal(Presets()[0])
	s := string(b)
	for _, key := range []string{`"colorScheme"`, `"fontFamily"`, `"maxWidthFull"`, `"codeBlockBg"`, `"customCSS"`, `"quoteBorderWidth"`} {
		if !strings.Contains(s, key) {
			t.Errorf("expected JSON key %s in %s", key, s)
		}
	}
}

func TestNameHelpers(t *testing.T) {
	styles := []Style{{ID: "a", Name: "X"}, {ID: "b", Name: "Y"}}
	if !NameTaken(styles, " X ", "") {
		t.Errorf("X should be taken (trim)")
	}
	if NameTaken(styles, "X", "a") {
		t.Errorf("X should be free when excepting its own id")
	}
	if UniqueName(styles, "X", "") != "X 2" {
		t.Errorf("UniqueName(X) = %q want %q", UniqueName(styles, "X", ""), "X 2")
	}
	if UniqueName(styles, "Z", "") != "Z" {
		t.Errorf("UniqueName(Z) should be Z")
	}
	if _, ok := FindByName(styles, "Y"); !ok {
		t.Errorf("FindByName(Y) should succeed")
	}
}

func TestBaseStyleName(t *testing.T) {
	if BaseStyleName("ライト"+PresetNameSuffix) != "ライト" {
		t.Errorf("suffix not stripped")
	}
	if BaseStyleName("マイ") != "マイ" {
		t.Errorf("non-preset name unchanged")
	}
}
