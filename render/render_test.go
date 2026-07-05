package render

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func mustRender(t *testing.T, src string, opts Options) Result {
	t.Helper()
	r, err := RenderMarkdown(src, opts)
	if err != nil {
		t.Fatalf("RenderMarkdown error: %v", err)
	}
	return r
}

func TestGFMTableAndTaskList(t *testing.T) {
	src := "| A | B |\n|---|---|\n| 1 | 2 |\n\n- [x] done\n- [ ] todo\n"
	html := mustRender(t, src, Options{}).HTML
	if !strings.Contains(html, "<table") || !strings.Contains(html, "<td") {
		t.Errorf("expected table, got: %s", html)
	}
	if !strings.Contains(html, `type="checkbox"`) {
		t.Errorf("expected task list checkbox, got: %s", html)
	}
}

func TestFootnotes(t *testing.T) {
	src := "本文[^1]。\n\n[^1]: 脚注の内容。\n"
	html := mustRender(t, src, Options{}).HTML
	// 参照（sup#fnref → #fn へのリンク）と脚注本体（li#fn）が bluemonday を通過して残る。
	if !strings.Contains(html, `href="#fn:1"`) || !strings.Contains(html, `id="fnref:1"`) {
		t.Errorf("footnote reference not rendered/kept: %s", html)
	}
	if !strings.Contains(html, `id="fn:1"`) || !strings.Contains(html, `href="#fnref:1"`) {
		t.Errorf("footnote body/backref not rendered/kept: %s", html)
	}
	if !strings.Contains(html, "脚注の内容") {
		t.Errorf("footnote text missing: %s", html)
	}
}

func TestHeadingSlugAndDedup(t *testing.T) {
	html := mustRender(t, "# Hello World\n\n## Hello World\n", Options{}).HTML
	if !strings.Contains(html, `id="hello-world"`) {
		t.Errorf("expected id=hello-world, got: %s", html)
	}
	if !strings.Contains(html, `id="hello-world-1"`) {
		t.Errorf("expected deduped id=hello-world-1, got: %s", html)
	}
}

func TestMermaidPlaceholder(t *testing.T) {
	src := "```mermaid\ngraph TD; A-->B;\n```\n"
	html := mustRender(t, src, Options{}).HTML
	if !strings.Contains(html, `<pre class="mermaid">`) {
		t.Errorf("expected mermaid placeholder, got: %s", html)
	}
	if !strings.Contains(html, "graph TD") {
		t.Errorf("expected escaped mermaid content, got: %s", html)
	}
	if strings.Contains(html, "<svg") {
		t.Errorf("mermaid must not be rendered server-side, got: %s", html)
	}
}

func TestCodeHighlight(t *testing.T) {
	src := "```go\nfunc main() {}\n```\n"
	html := mustRender(t, src, Options{}).HTML
	if !strings.Contains(html, `class="chroma"`) {
		t.Errorf("expected chroma highlighted block, got: %s", html)
	}
	if !strings.Contains(html, "<span") {
		t.Errorf("expected token spans, got: %s", html)
	}
}

func TestScriptRemoved(t *testing.T) {
	html := mustRender(t, "<script>alert(1)</script>\n\n# ok\n", Options{}).HTML
	if strings.Contains(strings.ToLower(html), "<script") {
		t.Errorf("script must be removed, got: %s", html)
	}
}

func TestJavascriptHrefRemoved(t *testing.T) {
	html := mustRender(t, "[x](javascript:alert(1))\n", Options{}).HTML
	if strings.Contains(strings.ToLower(html), "javascript:") {
		t.Errorf("javascript: href must be stripped, got: %s", html)
	}
}

func TestExternalLinkRel(t *testing.T) {
	html := mustRender(t, "[x](https://example.com)\n", Options{}).HTML
	if !strings.Contains(html, `rel="noopener noreferrer"`) {
		t.Errorf("expected rel on external link, got: %s", html)
	}
}

func TestInternalLinkNoRel(t *testing.T) {
	html := mustRender(t, "[x](#section)\n", Options{}).HTML
	if strings.Contains(html, "noopener") {
		t.Errorf("internal link must not get rel, got: %s", html)
	}
}

func TestRemoteImageBlocked(t *testing.T) {
	r := mustRender(t, "![a](https://example.com/i.png)\n", Options{AllowRemoteImages: false})
	if !r.HasRemoteImages {
		t.Errorf("expected HasRemoteImages=true")
	}
	// 実 src 属性（先頭スペース付き）でリモート URL を指していないこと。
	// data-blocked-src="..." は許容（退避用）。
	if strings.Contains(r.HTML, ` src="https://example.com/i.png"`) {
		t.Errorf("remote image src must be blocked, got: %s", r.HTML)
	}
	if !strings.Contains(r.HTML, "data-remote-blocked") {
		t.Errorf("expected data-remote-blocked marker, got: %s", r.HTML)
	}
}

func TestRemoteImageAllowed(t *testing.T) {
	r := mustRender(t, "![a](https://example.com/i.png)\n", Options{AllowRemoteImages: true})
	if !r.HasRemoteImages {
		t.Errorf("expected HasRemoteImages=true")
	}
	if !strings.Contains(r.HTML, `src="https://example.com/i.png"`) {
		t.Errorf("expected remote src kept when allowed, got: %s", r.HTML)
	}
}

// 最小の PNG バイト列（内容は検証しないため先頭シグネチャのみで十分）。
var pngBytes = []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x01}

func TestLocalImageDataURI(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "pic.png"), pngBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	r := mustRender(t, "![a](pic.png)\n", Options{BaseDir: dir})
	if !strings.Contains(r.HTML, "src=\"data:image/png;base64,") {
		t.Errorf("expected local image inlined as data URI, got: %s", r.HTML)
	}
	if r.HasRemoteImages {
		t.Errorf("local image must not set HasRemoteImages")
	}
}

func TestLocalImageMissingNoCrash(t *testing.T) {
	dir := t.TempDir()
	r := mustRender(t, "![a](nope.png)\n", Options{BaseDir: dir})
	// 不在ファイルは data URI 化されず、元の（相対）src のまま残るか src が無いだけ。クラッシュしないこと。
	if strings.Contains(r.HTML, "data:image") {
		t.Errorf("missing image should not produce data URI, got: %s", r.HTML)
	}
}

// isCrossHostUNC: 文書と異なるホストの UNC のみ true（＝読み込み遮断対象）。
// 同一ホスト UNC・ローカルパス・拡張長ローカルパスは false（読み込み許容）。
func TestIsCrossHostUNC(t *testing.T) {
	cases := []struct {
		baseDir, src string
		want         bool
	}{
		{`\\fileserver\docs`, `\\attacker\s\x.png`, true},        // 別ホスト UNC → 遮断
		{`\\fileserver\docs`, `\\fileserver\other\x.png`, false}, // 同一ホスト UNC → 許容
		{`\\FILESERVER\docs`, `\\fileserver\x.png`, false},       // ホスト比較は大小無視
		{`C:\Users\me`, `\\attacker\s\x.png`, true},              // ローカル文書 → 任意 UNC は別ホスト
		{`C:\Users\me`, `..\..\secret.png`, false},               // ローカル相対 → UNC ではない
		{`C:\Users\me`, `C:\Windows\win.ini`, false},             // ローカル絶対 → UNC ではない
		{`C:\Users\me`, `%5C%5Cattacker%5Cs%5Cx.png`, true},      // percent-encode 版 UNC → 遮断
		{`\\fileserver\docs`, `\\?\UNC\attacker\s\x.png`, true},  // 拡張長 UNC・別ホスト → 遮断
		{`C:\Users\me`, `\\?\C:\secret`, false},                  // 拡張長ローカル → UNC ではない
		{`\\fileserver\docs`, `\\attacker/s/x.png`, true},        // スラッシュ混在 UNC → 遮断
	}
	for _, c := range cases {
		if got := isCrossHostUNC(c.baseDir, c.src); got != c.want {
			t.Errorf("isCrossHostUNC(%q, %q)=%v want %v", c.baseDir, c.src, got, c.want)
		}
	}
}

// クロスホスト UNC 画像（生 HTML・percent-encode）は表示確認を出さず遮断し、読み込まない。
func TestCrossHostUNCImageBlocked(t *testing.T) {
	for _, src := range []string{
		`\\attacker.example.com\share\x.png`,
		`%5C%5Cattacker.example.com%5Cshare%5Cx.png`,
	} {
		r := mustRender(t, `<img alt="a" src="`+src+`">`+"\n", Options{BaseDir: `C:\Users\me\docs`})
		if !strings.Contains(r.HTML, "data-remote-blocked") {
			t.Errorf("cross-host UNC must be blocked, got: %s", r.HTML)
		}
		if strings.Contains(r.HTML, "attacker.example.com") && strings.Contains(r.HTML, ` src=`) {
			t.Errorf("cross-host UNC must not remain as a live src, got: %s", r.HTML)
		}
		if strings.Contains(r.HTML, "data:") {
			t.Errorf("cross-host UNC must not be read into a data URI, got: %s", r.HTML)
		}
		if r.HasRemoteImages {
			t.Errorf("cross-host UNC must not trigger the remote-image confirm dialog")
		}
	}
}

// 同一ホスト UNC 画像はクロスホスト遮断の対象外（＝文書と同じサーバの画像は許容）。
// ホストは 127.0.0.1（存在しない共有への読み取りは即座に失敗し、テストが速い）。
func TestSameHostUNCImageNotBlocked(t *testing.T) {
	r := mustRender(t, `<img alt="a" src="\\127.0.0.1\other\x.png">`+"\n", Options{BaseDir: `\\127.0.0.1\docs`})
	if strings.Contains(r.HTML, "data-remote-blocked") {
		t.Errorf("same-host UNC must not be blocked as cross-host, got: %s", r.HTML)
	}
	if r.HasRemoteImages {
		t.Errorf("same-host UNC must not trigger the remote-image confirm dialog")
	}
}

// srcset の「先頭以外」のリモート候補もゲート対象（Fix B）。先頭リモートも従来どおり遮断。
func TestSrcsetRemoteGatedAllCandidates(t *testing.T) {
	cases := []string{
		`<img srcset="./a.png 1x, https://attacker/y.png 2x">`,                                       // 2 番目がリモート
		`<img srcset="https://attacker/y.png 2x">`,                                                   // 先頭がリモート（回帰）
		`<img srcset="data:image/gif;base64,R0lGODlhAQABAAAAACw= 1w, https://attacker/y.png 9999w">`, // width 記述子
	}
	for _, in := range cases {
		r := mustRender(t, in+"\n", Options{AllowRemoteImages: false})
		if !r.HasRemoteImages {
			t.Errorf("remote srcset candidate must be detected: %q", in)
		}
		if strings.Contains(r.HTML, "attacker") {
			t.Errorf("remote srcset candidate must be stripped, got: %s (in=%q)", r.HTML, in)
		}
	}
}

// 先頭空白付きのリモート src も遮断する（ブラウザは前後空白を除去して取得するため）。
// data-blocked-src には退避で残るが、ライブな src 属性からは除去されていること。
func TestRemoteImageLeadingWhitespaceBlocked(t *testing.T) {
	r := mustRender(t, "<img src=\"  https://attacker/x.png\">\n", Options{AllowRemoteImages: false})
	if !r.HasRemoteImages {
		t.Errorf("leading-whitespace remote src must be detected as remote, got: %s", r.HTML)
	}
	if !strings.Contains(r.HTML, "data-remote-blocked") {
		t.Errorf("expected data-remote-blocked marker, got: %s", r.HTML)
	}
	if strings.Contains(r.HTML, ` src="  https://attacker/x.png"`) {
		t.Errorf("leading-whitespace remote src must be stripped from the live src, got: %s", r.HTML)
	}
}

func TestContentHasRemoteImages(t *testing.T) {
	cases := map[string]bool{
		"![a](https://x/y.png)":       true,
		"![a](//x/y.png)":             true,
		`<img src="https://x/y.png">`: true,
		"![a](./local.png)":           false,
		"no images here":              false,
	}
	for in, want := range cases {
		if got := ContentHasRemoteImages(in); got != want {
			t.Errorf("ContentHasRemoteImages(%q)=%v want %v", in, got, want)
		}
	}
}

func TestHighlightCSSNonEmpty(t *testing.T) {
	if HighlightCSS("light") == "" {
		t.Errorf("expected non-empty light highlight CSS")
	}
	if HighlightCSS("dark") == "" {
		t.Errorf("expected non-empty dark highlight CSS")
	}
}
