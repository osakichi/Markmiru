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
