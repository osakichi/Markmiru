package web

import "strings"

// utf8BOM は UTF-8 の BOM（U+FEFF）。
const utf8BOM = string(rune(0xFEFF))

// textFormat はファイルの「本文以外の書式」（先頭の BOM と改行コード）。Markmiru は UTF-8 のみを扱う。
// 読み込み時に本文から取り除いて（BOM を除き、改行を LF にそろえて）タブに記録し、保存時に元へ戻す
// ことで、編集操作に関係なく元のファイルと同じ BOM・改行コードで書き込む。
// 本文を正規化しておく理由: 編集モードの textarea は HTML の仕様で改行を LF にそろえる（CRLF / CR →
// LF）ため、CRLF のまま保持すると、未編集でも入力欄の内容を送り直しただけで本文が変わったとみなされ
// （未保存扱い・保存で改行が LF に変わる）、BOM も見えない先頭文字として編集で消え得るため。
// ゼロ値（BOM なし・LF）は Markmiru で新規作成したファイルの書式。
type textFormat struct {
	BOM  bool // 先頭に UTF-8 の BOM があった
	CRLF bool // 改行コードが CRLF（混在時は数の多い方。同数なら LF）
}

// decodeText はファイルから読んだ内容を、BOM を除き改行を LF にそろえた本文と、その書式に分ける。
// 改行コードは CRLF と LF（CR を伴わない LF）の数を比べて多い方とする（同数なら LF）。
// CR だけの改行（旧 Mac 形式）は数に入れず、本文では LF にそろえる（textarea と同じ扱い）。
func decodeText(raw string) (string, textFormat) {
	var f textFormat
	if rest, ok := strings.CutPrefix(raw, utf8BOM); ok {
		f.BOM = true
		raw = rest
	}
	crlf := strings.Count(raw, "\r\n")
	lf := strings.Count(raw, "\n") - crlf
	f.CRLF = crlf > lf
	return normalizeNewlines(raw), f
}

// encode は本文を保存用の内容に戻す（改行を記録した改行コードに、BOM を記録どおりに付け直す）。
// 本文に BOM や CR が紛れ込んでいても（貼り付け等）、書式は記録の方に合わせる。
func (f textFormat) encode(content string) string {
	content = normalizeNewlines(strings.TrimPrefix(content, utf8BOM))
	if f.CRLF {
		content = strings.ReplaceAll(content, "\n", "\r\n")
	}
	if f.BOM {
		content = utf8BOM + content
	}
	return content
}

// normalizeNewlines は改行を LF にそろえる（CRLF → LF、単独の CR → LF）。
func normalizeNewlines(s string) string {
	if !strings.Contains(s, "\r") {
		return s
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.ReplaceAll(s, "\r", "\n")
}
