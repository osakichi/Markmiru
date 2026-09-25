// Package render は Markdown を安全な HTML へ変換する閲覧モードの描画パイプライン。
//
// 描画の流れ:
//
//	goldmark（CommonMark + GFM ＋ 脚注）
//	  → 見出しに GitHub 互換スラッグ id 付与
//	  → ```mermaid フェンスは <pre class="mermaid"> プレースホルダ（描画は WebView の mermaid.js）
//	  → その他のコードは chroma で色付け（クラス方式。CSS は HighlightCSS が出力）
//	  → ローカル画像（相対/絶対/ルート相対/Windows パス）は Go が data URI 化
//	  → 外部リンクへ rel="noopener noreferrer" 付与、リモート画像は既定で遮断
//	  → bluemonday でサニタイズ（script 等の危険要素を除去）
//
// 設計: docs/アーキテクチャ・画面設計.md §3
package render

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"html"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"github.com/alecthomas/chroma/v2"
	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	gmhtml "github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
	xhtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// Options は RenderMarkdown の挙動を制御する。
type Options struct {
	// BaseDir はローカル画像を解決する基点（開いているファイルのディレクトリ）。
	// 空（無題ファイル等）の場合はローカル画像の data URI 化を行わない。
	BaseDir string
	// AllowRemoteImages が false（既定）の場合、リモート画像（http/https/プロトコル相対）は読み込まれない。
	AllowRemoteImages bool
}

// Result は RenderMarkdown の戻り値。
type Result struct {
	HTML            string
	HasRemoteImages bool // リモート画像が含まれていたか（遮断・許可に関わらず検出時 true）
}

// imageMaxBytes はローカル画像として読み込む最大サイズ（50 MB）。app.go と同値。
const imageMaxBytes = 50 * 1024 * 1024

// remoteURLRe は http/https/プロトコル相対（//）を外部リソースとみなす。
var remoteURLRe = regexp.MustCompile(`(?i)^(https?:)?//`)

// --- goldmark / bluemonday は一度だけ構築して使い回す ---

var md = goldmark.New(
	goldmark.WithExtensions(extension.GFM, extension.Footnote), // 表・打消し線・自動リンク・タスクリスト・脚注
	goldmark.WithParserOptions(
		parser.WithASTTransformers(util.Prioritized(headingIDTransformer{}, 100)),
	),
	goldmark.WithRendererOptions(
		gmhtml.WithUnsafe(), // 生 HTML を許可（後段の bluemonday で無害化）
		renderer.WithNodeRenderers(util.Prioritized(&codeRenderer{}, 100)),
	),
)

var policy = buildPolicy()

// RenderMarkdown は Markdown を安全な HTML へ変換する（mermaid はプレースホルダのまま返る）。
func RenderMarkdown(src string, opts Options) (Result, error) {
	var buf bytes.Buffer
	if err := md.Convert([]byte(src), &buf); err != nil {
		return Result{}, err
	}
	// サニタイズ前: ローカル画像の data URI 化・リモート画像遮断を行う。
	// data URI 化を先に済ませることで、bluemonday が Windows 絶対パス（C:\...）の src を
	// 未知スキームとして除去する問題を回避する。
	pre, saw := preTransform(buf.String(), opts)
	safe := policy.Sanitize(pre)
	// サニタイズ後: 外部リンクへ rel="noopener noreferrer" を付与する。
	// bluemonday は rel を許可すると nofollow を自動付与するため、ここで確定値を与える。
	final := addExternalLinkRel(safe)
	return Result{HTML: final, HasRemoteImages: saw}, nil
}

// ContentHasRemoteImages は Markdown にリモート画像が含まれるか簡易判定する（事前確認用）。
var (
	mdRemoteImgRe   = regexp.MustCompile(`(?i)!\[[^\]]*\]\(\s*(?:https?:)?//`)
	htmlRemoteImgRe = regexp.MustCompile(`(?i)<img\b[^>]*\bsrc\s*=\s*["'](?:https?:)?//`)
)

func ContentHasRemoteImages(content string) bool {
	return mdRemoteImgRe.MatchString(content) || htmlRemoteImgRe.MatchString(content)
}

// editorMarkdownLexer は編集オーバーレイ専用の簡易 Markdown レキサ。
//
// chroma 標準の markdown レキサは、行内コード規則（`[^`]+`）が改行を除外していないため、
// レキサが認識しないフェンス（リスト内でインデントされた ```lang 等）のバッククォートと
// 改行をまたいで対になり、複数行〜文書後半までを行内コードとして丸呑みして、以降の
// 見出し・強調のハイライトが失われることがある。
//
// オーバーレイの目的は「控えめなハイライト」（見出し・強調・引用・行内コード等）だけなので、
// **すべてのトークンが 1 行内に閉じる**規則のみで定義し、この種の巻き込みを構造的に排除する。
// フェンスはフェンス行だけを文字列色にし、中身は素のテキストのまま（言語別の委譲はしない）。
// 単独アンダースコアの斜体（_x_）は snake_case の誤検出を避けるため対象外。
var editorMarkdownLexer = chroma.Coalesce(chroma.MustNewLexer(
	&chroma.Config{Name: "markdown-overlay"},
	func() chroma.Rules {
		return chroma.Rules{
			"root": {
				{Pattern: `^#[^#\n].*\n?`, Type: chroma.GenericHeading},
				{Pattern: `^#{2,6}[^\n]*\n?`, Type: chroma.GenericSubheading},
				{Pattern: `^(\s*>[ \t]?)([^\n]*\n?)`, Type: chroma.ByGroups(chroma.Keyword, chroma.GenericEmph)},
				{Pattern: "^\\s*```[^\\n]*\\n?", Type: chroma.LiteralString},
				{Pattern: `^(\s*)([*-] |\d+\. )`, Type: chroma.ByGroups(chroma.Text, chroma.Keyword)},
				chroma.Include("inline"),
			},
			"inline": {
				{Pattern: "`[^`\\n]+`", Type: chroma.LiteralStringBacktick},
				{Pattern: `\*\*[^*\n]+\*\*`, Type: chroma.GenericStrong},
				{Pattern: `__[^_\n]+__`, Type: chroma.GenericStrong},
				{Pattern: `\*[^*\n]+\*`, Type: chroma.GenericEmph},
				{Pattern: `~~[^~\n]+~~`, Type: chroma.GenericDeleted},
				{Pattern: `(!?\[)([^]\n]+)(\])(\()([^)\n]+)(\))`, Type: chroma.ByGroups(chroma.Text, chroma.NameTag, chroma.Text, chroma.Text, chroma.NameAttribute, chroma.Text)},
				{Pattern: "[^`*_~\\[\\n]+", Type: chroma.Text},
				{Pattern: `.|\n`, Type: chroma.Text},
			},
		}
	},
))

// HighlightInner は編集オーバーレイ用に、Markdown を専用の簡易レキサ（editorMarkdownLexer）で
// クラス付きトークン列（<pre> 無し）にして返す。呼び出し側が <pre class="chroma hl"> で包む。
// 色は #markmiru-code-theme（chroma クラス）が付ける。
func HighlightInner(code string) string {
	iterator, err := editorMarkdownLexer.Tokenise(nil, code)
	if err != nil {
		return html.EscapeString(code)
	}
	f := chromahtml.New(chromahtml.WithClasses(true), chromahtml.PreventSurroundingPre(true))
	var b strings.Builder
	if err := f.Format(&b, styles.Fallback, iterator); err != nil {
		return html.EscapeString(code)
	}
	return b.String()
}

// HighlightCSS は chroma のクラスに対応する CSS を返す（colorScheme に応じて github / github-dark）。
func HighlightCSS(scheme string) string {
	name := "github"
	if scheme == "dark" {
		name = "github-dark"
	}
	style := styles.Get(name) // 見つからなければ Fallback を返す
	formatter := chromahtml.New(chromahtml.WithClasses(true))
	var buf bytes.Buffer
	_ = formatter.WriteCSS(&buf, style)
	return buf.String()
}

// --- 見出しスラッグ id ---

// headingIDTransformer は見出しに GitHub 互換のスラッグ id を付与する。
// 規則: トリム → 小文字化 → 文字/数字/結合文字/空白/_/- 以外を除去 → 連続空白を - に。
// 同一スラッグは 2 件目以降に -1, -2… を付けて重複を解決する。
type headingIDTransformer struct{}

func (headingIDTransformer) Transform(doc *ast.Document, reader text.Reader, _ parser.Context) {
	src := reader.Source()
	seen := map[string]int{}
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		h, ok := n.(*ast.Heading)
		if !ok {
			return ast.WalkContinue, nil
		}
		base := slugify(nodeText(h, src))
		if base == "" {
			base = "section"
		}
		idx := seen[base]
		seen[base] = idx + 1
		id := base
		if idx > 0 {
			id = fmt.Sprintf("%s-%d", base, idx)
		}
		h.SetAttributeString("id", []byte(id))
		return ast.WalkSkipChildren, nil
	})
}

func nodeText(n ast.Node, src []byte) string {
	var b strings.Builder
	_ = ast.Walk(n, func(c ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch t := c.(type) {
		case *ast.Text:
			b.Write(t.Segment.Value(src))
		case *ast.String:
			b.Write(t.Value)
		}
		return ast.WalkContinue, nil
	})
	return b.String()
}

var wsRe = regexp.MustCompile(`\s+`)

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		switch {
		case unicode.IsSpace(r):
			b.WriteByte(' ') // 全角空白等も半角に寄せてから連続空白を - に畳む
		case unicode.IsLetter(r), unicode.IsNumber(r), unicode.IsMark(r), r == '_', r == '-':
			b.WriteRune(r)
		}
	}
	return wsRe.ReplaceAllString(b.String(), "-")
}

// --- コードブロック（mermaid プレースホルダ / chroma ハイライト） ---

type codeRenderer struct{}

func (r *codeRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindFencedCodeBlock, r.renderFenced)
	reg.Register(ast.KindCodeBlock, r.renderIndented)
}

func (r *codeRenderer) renderFenced(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	n := node.(*ast.FencedCodeBlock)
	info := ""
	if n.Info != nil {
		info = strings.TrimSpace(string(n.Info.Segment.Value(source)))
	}
	code := linesText(n, source)
	if strings.EqualFold(info, "mermaid") {
		w.WriteString(`<pre class="mermaid">`)
		w.WriteString(html.EscapeString(code))
		w.WriteString("</pre>")
		return ast.WalkSkipChildren, nil
	}
	lang := info
	if i := strings.IndexAny(lang, " \t"); i >= 0 {
		lang = lang[:i]
	}
	writeHighlighted(w, code, lang)
	return ast.WalkSkipChildren, nil
}

func (r *codeRenderer) renderIndented(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	w.WriteString(`<pre class="chroma"><code>`)
	w.WriteString(html.EscapeString(linesText(node, source)))
	w.WriteString("</code></pre>")
	return ast.WalkSkipChildren, nil
}

func linesText(n ast.Node, source []byte) string {
	var b strings.Builder
	l := n.Lines()
	for i := 0; i < l.Len(); i++ {
		seg := l.At(i)
		b.Write(seg.Value(source))
	}
	return b.String()
}

func writeHighlighted(w util.BufWriter, code, lang string) {
	lexer := lexers.Get(lang)
	if lexer == nil {
		fmt.Fprintf(w, `<pre class="chroma"><code>%s</code></pre>`, html.EscapeString(code))
		return
	}
	lexer = chroma.Coalesce(lexer)
	iterator, err := lexer.Tokenise(nil, code)
	if err != nil {
		fmt.Fprintf(w, `<pre class="chroma"><code>%s</code></pre>`, html.EscapeString(code))
		return
	}
	formatter := chromahtml.New(chromahtml.WithClasses(true))
	if err := formatter.Format(w, styles.Fallback, iterator); err != nil {
		fmt.Fprintf(w, `<pre class="chroma"><code>%s</code></pre>`, html.EscapeString(code))
	}
}

// --- サニタイズ前の意味的変換（画像 data URI 化・外部リンク rel・リモート画像遮断） ---

// walkFragment は HTML 断片をパースし、各要素ノードに visit を適用して再シリアライズする。
// パース失敗時は入力をそのまま返す。
func walkFragment(htmlStr string, visit func(n *xhtml.Node)) string {
	ctx := &xhtml.Node{Type: xhtml.ElementNode, Data: "body", DataAtom: atom.Body}
	nodes, err := xhtml.ParseFragment(strings.NewReader(htmlStr), ctx)
	if err != nil {
		return htmlStr
	}
	var rec func(n *xhtml.Node)
	rec = func(n *xhtml.Node) {
		if n.Type == xhtml.ElementNode {
			visit(n)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			rec(c)
		}
	}
	for _, n := range nodes {
		rec(n)
	}
	var buf bytes.Buffer
	for _, n := range nodes {
		_ = xhtml.Render(&buf, n)
	}
	return buf.String()
}

// preTransform はサニタイズ前に画像を処理する（ローカル→data URI、リモート→遮断）。
func preTransform(htmlStr string, opts Options) (string, bool) {
	saw := false
	out := walkFragment(htmlStr, func(n *xhtml.Node) {
		if n.Data == "img" || n.Data == "source" {
			processImage(n, opts, &saw)
		}
	})
	return out, saw
}

// addExternalLinkRel はサニタイズ後に外部リンクへ rel="noopener noreferrer" を付与する。
func addExternalLinkRel(htmlStr string) string {
	return walkFragment(htmlStr, func(n *xhtml.Node) {
		if n.Data == "a" && remoteURLRe.MatchString(getAttr(n, "href")) {
			setAttr(n, "rel", "noopener noreferrer")
		}
	})
}

func processImage(n *xhtml.Node, opts Options, saw *bool) {
	src := getAttr(n, "src")
	// リモート（http/https/プロトコル相対）: src か srcset の「いずれかの候補」に含まれれば
	// ファイル単位の表示確認ゲートにかける（srcset は全候補を走査。先頭空白等も正規化）。
	if isRemoteURL(src) || srcsetHasRemote(getAttr(n, "srcset")) {
		*saw = true
		if !opts.AllowRemoteImages {
			// 遮断: 退避してから src/srcset を除去（読み込みを発生させない）。
			if src != "" {
				setAttr(n, "data-blocked-src", src)
			}
			delAttr(n, "src")
			delAttr(n, "srcset")
			setAttr(n, "data-remote-blocked", "")
		}
		return
	}
	// クロスホスト UNC（\\別ホスト\...）: 読み込むと Windows が別サーバへ自動で認証情報
	// （NTLM ハッシュ）を送出するため、表示確認は出さず常に遮断する。同一ホスト UNC
	// （文書と同じサーバ）はローカルとして下で読み込む。
	if isCrossHostUNC(opts.BaseDir, src) {
		if src != "" {
			setAttr(n, "data-blocked-src", src)
		}
		delAttr(n, "src")
		setAttr(n, "data-remote-blocked", "")
		return
	}
	// ローカル画像（<img> のみ）を data URI 化する。
	if n.Data != "img" || src == "" || strings.HasPrefix(src, "data:") || opts.BaseDir == "" {
		return
	}
	if dataURI := imageToDataURI(opts.BaseDir, src); dataURI != "" {
		setAttr(n, "src", dataURI)
	}
}

// imageToDataURI はローカル画像を読み込み data URI を返す（失敗・不在・サイズ超過は空文字）。
func imageToDataURI(baseDir, src string) string {
	// セキュリティ要: クロスホスト UNC は決して os.ReadFile しない（別サーバへの SMB 接続＝
	// 認証情報漏洩を防ぐ）。percent-encode 版 UNC（%5C%5C...）も isCrossHostUNC が
	// デコードして判定するため、processImage をすり抜けてもここで確実に遮断する。
	if isCrossHostUNC(baseDir, src) {
		return ""
	}
	// goldmark は画像 URL を percent-encode するため（C:\ → C:%5C）デコードする。
	if d, err := url.PathUnescape(src); err == nil {
		src = d
	}
	abs := resolveImagePath(baseDir, src)
	info, err := os.Stat(abs)
	if err != nil || info.Size() > imageMaxBytes {
		return ""
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return ""
	}
	return "data:" + imageMIME(filepath.Ext(abs)) + ";base64," + base64.StdEncoding.EncodeToString(data)
}

// resolveImagePath は src を baseDir 基点の絶対パスに解決する（app.go と同じ規則）。
func resolveImagePath(baseDir, src string) string {
	if filepath.IsAbs(src) {
		return filepath.Clean(src)
	}
	if len(src) > 0 && os.IsPathSeparator(src[0]) {
		vol := filepath.VolumeName(baseDir) // Windows: "C:" / 他: ""
		return filepath.Clean(vol + src)
	}
	return filepath.Clean(filepath.Join(baseDir, src))
}

// isRemoteURL は http/https/プロトコル相対 URL かを判定する。ブラウザは属性値の前後空白を
// 除去して解決するため、先頭空白による remoteURLRe 回避を防ぐべく TrimSpace してから判定する。
func isRemoteURL(s string) bool {
	return remoteURLRe.MatchString(strings.TrimSpace(s))
}

// srcsetHasRemote は srcset の「いずれかの候補」がリモート URL かを判定する。srcset は
// 「URL 記述子, URL 記述子, …」形式で、先頭候補だけを見るとゲートを回避されるため全候補を走査する。
func srcsetHasRemote(srcset string) bool {
	for _, cand := range strings.Split(srcset, ",") {
		cand = strings.TrimSpace(cand)
		if i := strings.IndexAny(cand, " \t\n\r\f"); i >= 0 {
			cand = cand[:i] // 記述子（1x / 2x / 100w 等）を落とし URL だけにする
		}
		if isRemoteURL(cand) {
			return true
		}
	}
	return false
}

// isCrossHostUNC は src（percent-encode されている可能性あり）が baseDir と異なるホストの
// UNC（\\host\...）を指すかを判定する。true のとき読み込みは別サーバへの接続＝認証情報漏洩に
// なるため、呼び出し側は常に遮断する。baseDir 自体が同一ホストの UNC 上にある場合のみ許可。
func isCrossHostUNC(baseDir, src string) bool {
	if d, err := url.PathUnescape(src); err == nil {
		src = d
	}
	host, isUNC := uncHost(src)
	if !isUNC {
		return false
	}
	baseHost, baseIsUNC := uncHost(baseDir)
	return !baseIsUNC || !strings.EqualFold(host, baseHost)
}

// uncHost は Windows UNC パス（\\host\share… / \\?\UNC\host\share…、スラッシュ混在も可）の
// ホスト名を返す。UNC でなければ ("", false)。拡張長ローカルパス（\\?\C:\…）は UNC ではない。
func uncHost(p string) (string, bool) {
	s := strings.ReplaceAll(p, "/", `\`)
	if rest, ok := cutPrefixFold(s, `\\?\UNC\`); ok {
		return uncFirstComponent(rest), true
	}
	if _, ok := cutPrefixFold(s, `\\?\`); ok {
		return "", false // \\?\C:\… 等の拡張長ローカルパス（UNC ではない）
	}
	if strings.HasPrefix(s, `\\`) {
		return uncFirstComponent(s[2:]), true
	}
	return "", false
}

func uncFirstComponent(s string) string {
	if i := strings.IndexByte(s, '\\'); i >= 0 {
		return s[:i]
	}
	return s
}

func cutPrefixFold(s, prefix string) (string, bool) {
	if len(s) >= len(prefix) && strings.EqualFold(s[:len(prefix)], prefix) {
		return s[len(prefix):], true
	}
	return "", false
}

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

// --- x/net/html 属性ヘルパ ---

func getAttr(n *xhtml.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func setAttr(n *xhtml.Node, key, val string) {
	for i := range n.Attr {
		if n.Attr[i].Key == key {
			n.Attr[i].Val = val
			return
		}
	}
	n.Attr = append(n.Attr, xhtml.Attribute{Key: key, Val: val})
}

func delAttr(n *xhtml.Node, key string) {
	out := n.Attr[:0]
	for _, a := range n.Attr {
		if a.Key != key {
			out = append(out, a)
		}
	}
	n.Attr = out
}

// --- bluemonday ポリシー ---

func buildPolicy() *bluemonday.Policy {
	p := bluemonday.NewPolicy()
	p.AllowElements(
		"p", "br", "hr",
		"h1", "h2", "h3", "h4", "h5", "h6",
		"blockquote", "strong", "b", "em", "i", "del", "s", "sub", "sup", "mark", "kbd", "abbr",
		"ul", "ol", "li",
		"table", "thead", "tbody", "tr", "th", "td",
		"pre", "code", "span", "div", "input",
	)
	p.AllowAttrs("id").OnElements("h1", "h2", "h3", "h4", "h5", "h6", "sup", "li") // 見出し＋脚注アンカー（fnref/fn）
	p.AllowAttrs("class").OnElements("pre", "code", "span", "ul", "ol", "li", "div", "table", "a", "sup")
	p.AllowAttrs("role").OnElements("a", "li", "div", "sup") // 脚注の a11y（doc-noteref / doc-endnote 等）
	p.AllowAttrs("tabindex").OnElements("pre")
	// 表の配置
	p.AllowAttrs("align").OnElements("td", "th", "tr")
	p.AllowAttrs("colspan", "rowspan").OnElements("td", "th")
	p.AllowStyles("text-align").MatchingEnum("left", "right", "center").OnElements("td", "th")
	// 明示的な改ページ（Typora・ブラウザ印刷全般の書き方）。div のみ・ページを改める値のみ許可し、
	// 他の宣言は除去する。Markdown PDF の書き方（class="page"）は class で通り、印刷用 CSS が効かせる。
	// 閲覧モードの目印（markdown.css）は「style 付きの div ＝改ページ」を前提にしている。
	p.AllowStyles("break-before", "break-after").MatchingEnum("page").OnElements("div")
	p.AllowStyles("page-break-before", "page-break-after").MatchingEnum("always").OnElements("div")
	// リンク（rel は transform で付与済み）
	// rel は許可しない（サニタイズ後の addExternalLinkRel で確定値を付与するため）。
	// bluemonday は rel を許可すると外部リンクへ nofollow を自動付与してしまう。
	p.AllowAttrs("href", "title").OnElements("a")
	p.AllowURLSchemes("http", "https", "mailto")
	p.AllowRelativeURLs(true)
	// 画像（ローカルは data URI 化済み、リモートは許可時のみ src が残る）
	p.AllowAttrs("alt", "title", "width", "height", "srcset", "data-remote-blocked", "data-blocked-src").OnElements("img")
	p.AllowImages()
	p.AllowDataURIImages()
	// タスクリストのチェックボックス
	p.AllowAttrs("type", "checked", "disabled").OnElements("input")
	return p
}
