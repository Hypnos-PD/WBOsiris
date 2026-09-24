package syntax

import (
	"bytes"
	"testing"
)

func TestParserBuildsStatementsAndSpans(t *testing.T) {
	src := []byte("wbo 0.1.0; card 12345678 { effect { if overflow { draw 1; } else { summon 1 card 12345678; } } }")
	f, ds := Parse("card.wbo", src)
	if len(ds) != 0 {
		t.Fatalf("unexpected diagnostics: %#v", ds)
	}
	if len(f.Statements) != 2 || len(f.Statements[1].Blocks()) != 1 {
		t.Fatalf("unexpected AST: %#v", f.Statements)
	}
	effect := f.Statements[1].Blocks()[0][0]
	if effect.Word(0) != "effect" || len(effect.Blocks()[0][0].Blocks()) != 2 {
		t.Fatalf("if/else was not structurally parsed")
	}
	if f.Statements[1].Span.Start.Line != 1 || f.Statements[1].Span.End.Byte != len(src) {
		t.Fatalf("bad span: %#v", f.Statements[1].Span)
	}
}

func TestParserRejectsMissingSemicolon(t *testing.T) {
	_, ds := Parse("bad.wbo", []byte("card 12345678 { effect { draw 1 } }"))
	if len(ds) == 0 {
		t.Fatal("expected syntax diagnostic")
	}
}

func TestFormatIdempotent(t *testing.T) {
	src := []byte("wbo 0.1.0;card 12345678{effect{buff self +1/+2;}locale chs{name \"x\";text \"\"\"\n  a\n  b\n\"\"\";}}")
	f, ds := Parse("x.wbo", src)
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	one := Format(f)
	f2, ds := Parse("x.wbo", one)
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	two := Format(f2)
	if !bytes.Equal(one, two) {
		t.Fatalf("format not idempotent:\n%s\n---\n%s", one, two)
	}
}

func TestFormatStringControlsAndTripleCRLF(t *testing.T) {
	src := []byte("wbo 0.1.0; name \"a\\n\\t\\u{1}\"; text \"\"\"\r\n    first\r\n\r\n      second\r\n    \"\"\";")
	f, ds := Parse("x.wbo", src)
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	before := []string{f.Statements[1].Word(1), f.Statements[2].Word(1)}
	one := Format(f)
	f2, ds := Parse("x.wbo", one)
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	after := []string{f2.Statements[1].Word(1), f2.Statements[2].Word(1)}
	if before[0] != after[0] || before[1] != after[1] {
		t.Fatalf("semantic text changed: %q / %q", before, after)
	}
	if !bytes.Equal(one, Format(f2)) {
		t.Fatal("format not idempotent")
	}
	if bytes.Contains(one, []byte(`\x01`)) {
		t.Fatal("formatter emitted unsupported Go escape")
	}
}
