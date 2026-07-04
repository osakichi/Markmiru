package render

import (
	"strings"
	"testing"
)

// 編集オーバーレイの簡易レキサ: すべてのトークンが 1 行内に閉じること（＝インデントされた
// フェンス等が供給する余分なバッククォートに引きずられて、後続の見出し・強調のハイライトが
// 失われないこと）の回帰テスト。chroma 標準 markdown レキサでは行内コード規則が改行を
// またぐため、この入力で「### B」以降がトークン化されなくなっていた。
func TestHighlightInnerLineBoundTokens(t *testing.T) {
	src := "### A\n" +
		"- リスト内のインデントされたフェンス:\n" +
		"  ```bash\n" +
		"  xattr -dr com.apple.quarantine /Applications/App.app\n" +
		"  ```\n" +
		"\n" +
		"**太字** と `code` と [リンク](https://example.com/)\n" +
		"\n" +
		"### B\n"
	h := HighlightInner(src)
	if !strings.Contains(h, `class="gu">### A`) || !strings.Contains(h, `class="gu">### B`) {
		t.Errorf("headings before/after the indented fence must both be tokenized: %s", h)
	}
	if !strings.Contains(h, `class="gs">**太字**`) {
		t.Errorf("bold must be tokenized after the fence: %s", h)
	}
	if !strings.Contains(h, `class="sb">`) {
		t.Errorf("inline code must be tokenized: %s", h)
	}
	// トークンが改行をまたがない＝span 内に改行が入るのは行末の 1 個のみ。
	for _, span := range strings.Split(h, "<span") {
		if i := strings.Index(span, "</span>"); i >= 0 {
			if n := strings.Count(span[:i], "\n"); n > 1 {
				t.Errorf("token spans multiple lines (%d newlines): %q", n, span[:i])
			}
		}
	}
	// 出力テキスト（タグ除去相当）が入力と一致する＝ハイライトが文字を欠落・追加しない。
	plain := h
	for _, tag := range []string{"</span>"} {
		plain = strings.ReplaceAll(plain, tag, "")
	}
	for {
		i := strings.Index(plain, "<span")
		if i < 0 {
			break
		}
		j := strings.Index(plain[i:], ">")
		plain = plain[:i] + plain[i+j+1:]
	}
	plain = strings.NewReplacer("&lt;", "<", "&gt;", ">", "&amp;", "&", "&#34;", `"`, "&#39;", "'").Replace(plain)
	if plain != src {
		t.Errorf("highlight output must preserve the source text exactly:\n got=%q\nwant=%q", plain, src)
	}
}
