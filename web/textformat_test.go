package web

import "testing"

const bom = "\uFEFF"

func TestDecodeText(t *testing.T) {
	cases := []struct {
		name, raw, content string
		format             textFormat
	}{
		{"LF・BOM なし", "a\nb\n", "a\nb\n", textFormat{}},
		{"CRLF・BOM なし", "a\r\nb\r\n", "a\nb\n", textFormat{CRLF: true}},
		{"LF・BOM あり", bom + "a\nb", "a\nb", textFormat{BOM: true}},
		{"CRLF・BOM あり", bom + "a\r\nb\r\n", "a\nb\n", textFormat{BOM: true, CRLF: true}},
		{"混在・CRLF が多い", "a\r\nb\r\nc\nd", "a\nb\nc\nd", textFormat{CRLF: true}},
		{"混在・LF が多い", "a\r\nb\nc\nd", "a\nb\nc\nd", textFormat{}},
		{"混在・同数は LF", "a\r\nb\nc", "a\nb\nc", textFormat{}},
		{"単独 CR は LF にそろえ数に入れない", "a\rb\r\nc", "a\nb\nc", textFormat{CRLF: true}},
		{"改行なし", "abc", "abc", textFormat{}},
		{"空", "", "", textFormat{}},
	}
	for _, c := range cases {
		content, format := decodeText(c.raw)
		if content != c.content || format != c.format {
			t.Errorf("%s: decodeText(%q) = (%q, %+v), want (%q, %+v)", c.name, c.raw, content, format, c.content, c.format)
		}
	}
}

func TestEncodeText(t *testing.T) {
	cases := []struct {
		name    string
		format  textFormat
		content string
		want    string
	}{
		{"新規（ゼロ値）は BOM なし・LF", textFormat{}, "a\nb\n", "a\nb\n"},
		{"CRLF に戻す", textFormat{CRLF: true}, "a\nb\n", "a\r\nb\r\n"},
		{"BOM を付け直す", textFormat{BOM: true}, "a\nb", bom + "a\nb"},
		{"BOM・CRLF", textFormat{BOM: true, CRLF: true}, "a\nb", bom + "a\r\nb"},
		{"貼り付けで紛れた BOM は二重にしない", textFormat{BOM: true}, bom + "a", bom + "a"},
		{"BOM なしのファイルに紛れた BOM は除く", textFormat{}, bom + "a", "a"},
		{"紛れた CRLF・CR は書式に合わせる", textFormat{}, "a\r\nb\rc", "a\nb\nc"},
		{"紛れた CRLF を二重にしない", textFormat{CRLF: true}, "a\r\nb", "a\r\nb"},
	}
	for _, c := range cases {
		if got := c.format.encode(c.content); got != c.want {
			t.Errorf("%s: encode(%q) = %q, want %q", c.name, c.content, got, c.want)
		}
	}
}

// 読み込み→（編集なしで）保存用に戻すと、元のファイルとバイト単位で一致する。
func TestTextFormatRoundTrip(t *testing.T) {
	for _, raw := range []string{"a\nb\n", "a\r\nb\r\n", bom + "a\nb", bom + "a\r\nb\r\n", "", bom} {
		content, format := decodeText(raw)
		if got := format.encode(content); got != raw {
			t.Errorf("round trip of %q = %q", raw, got)
		}
	}
}
