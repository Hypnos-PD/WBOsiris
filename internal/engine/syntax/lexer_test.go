package syntax

import "testing"

func TestLexerUTF8BOMStringsCommentsAndLongestMatch(t *testing.T) {
	src := []byte("\xef\xbb\xbf/* 外 /* 内 */ 层 */ << 这是 WBO 行注释\nname \"中文\\n\\u{41}\"; text \"\"\"\n    一行\n    二行\n    \"\"\"; a<=2; b!=3;")
	tokens, ds := Lex("sample.wbo", src)
	if len(ds) != 0 {
		t.Fatalf("unexpected diagnostics: %#v", ds)
	}
	var values []string
	for _, tok := range tokens {
		values = append(values, tok.Value)
	}
	if tokens[1].Value != "中文\nA" {
		t.Fatalf("decoded string = %q", tokens[1].Value)
	}
	if len(tokens[0].LeadingComments) != 1 || tokens[0].LeadingComments[0] != "<< 这是 WBO 行注释" {
		t.Fatalf("line comment was not retained: %#v", tokens[0].LeadingComments)
	}
	if tokens[4].Kind != TripleString || tokens[4].Value != "一行\n二行" {
		t.Fatalf("triple string = %#v", tokens[4])
	}
	foundLE, foundNE := false, false
	for _, v := range values {
		foundLE = foundLE || v == "<="
		foundNE = foundNE || v == "!="
	}
	if !foundLE || !foundNE {
		t.Fatalf("operators were not longest-matched: %v", values)
	}
}

func TestLexerInvalidEscapeAndNestedCommentDiagnostic(t *testing.T) {
	_, ds := Lex("bad.wbo", []byte("\"\\q\" /* open"))
	if len(ds) != 2 {
		t.Fatalf("got %d diagnostics: %#v", len(ds), ds)
	}
	for _, d := range ds {
		if d.Code != "WBO-E001-SYNTAX" || d.Severity != "错误" {
			t.Fatalf("bad diagnostic: %#v", d)
		}
	}
}
