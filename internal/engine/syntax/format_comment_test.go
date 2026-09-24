package syntax

import (
	"strings"
	"testing"
)

func TestFormatPreservesLineComments(t *testing.T) {
	source := []byte("<< 文件说明\nwbo 0.1.0;\ncard 12345678 {\n<< 字段说明\ntype spell;\n<< 空块说明\n}\n<< 文件结尾")
	file, diagnostics := Parse("comment.wbo", source)
	if len(diagnostics) != 0 {
		t.Fatalf("parse comments: %#v", diagnostics)
	}
	formatted := Format(file)
	for _, comment := range []string{"<< 文件说明", "    << 字段说明", "    << 空块说明", "<< 文件结尾"} {
		if !strings.Contains(string(formatted), comment) {
			t.Fatalf("formatted source lost %q:\n%s", comment, formatted)
		}
	}
	again, diagnostics := Parse("comment.wbo", formatted)
	if len(diagnostics) != 0 || string(Format(again)) != string(formatted) {
		t.Fatalf("comment formatting is not idempotent: diagnostics=%#v\n%s", diagnostics, Format(again))
	}
}

func TestFormatPreservesCommentsAroundPunctuationAndElse(t *testing.T) {
	source := []byte("effect << 块前\n{\nif overflow {\ndraw 1;\n<< 块尾\n} << 分支间\nelse {\ndraw 1 << 分号前\n;\n}\n// 兼容注释\n}")
	file, diagnostics := Parse("edge.wbo", source)
	if len(diagnostics) != 0 {
		t.Fatalf("parse comments: %#v", diagnostics)
	}
	formatted := Format(file)
	wantOrder := []string{"effect\n<< 块前\n{", "        << 块尾\n    }\n    << 分支间\n    else {", "draw 1\n        << 分号前\n        ;", "    // 兼容注释\n}"}
	for _, want := range wantOrder {
		if !strings.Contains(string(formatted), want) {
			t.Fatalf("formatted source lost ordered fragment %q:\n%s", want, formatted)
		}
	}
	again, diagnostics := Parse("edge.wbo", formatted)
	if len(diagnostics) != 0 || string(Format(again)) != string(formatted) {
		t.Fatalf("edge comment formatting is not idempotent: diagnostics=%#v\n%s", diagnostics, Format(again))
	}
}
