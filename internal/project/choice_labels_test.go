package project

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"wbo/internal/ir"
	"wbo/internal/syntax"
)

func TestModeLabelsCompileWithoutExecutableStatements(t *testing.T) {
	source := validCard(`fanfare { mode {
        option 7 { label chs "Draw label"; label eng "Draw a card."; draw 1; }
        option 2 { label chs """Restore
two life."""; heal own.leader 2; }
    } }`)
	file, ds := syntax.Parse("12345678.wbo", []byte(source))
	if len(ds) != 0 || hasErrors(ValidateFile(file)) {
		t.Fatal(ds, ValidateFile(file))
	}
	formatted := syntax.Format(file)
	again, ds := syntax.Parse("12345678.wbo", formatted)
	if len(ds) != 0 || !bytes.Equal(formatted, syntax.Format(again)) {
		t.Fatal("mode labels changed during formatting")
	}
	root := t.TempDir()
	dir := filepath.Join(root, "12345")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "12345678.wbo")
	if err := os.WriteFile(path, formatted, 0600); err != nil {
		t.Fatal(err)
	}
	loaded := LoadWithRoot([]string{path}, true, root)
	pack, _, err := BuildRuntimePacks(loaded)
	if err != nil {
		t.Fatal(err, loaded.Diagnostics)
	}
	mode := pack.Cards[0].Abilities[0].Body[0].(ir.ModeEffect)
	if mode.Options[0].ID != 7 || mode.Options[1].ID != 2 || mode.Options[0].Labels["eng"] != "Draw a card." || mode.Options[1].Labels["chs"] != "Restore\ntwo life." || len(mode.Options[0].Body) != 1 || len(mode.Options[1].Body) != 1 {
		t.Fatalf("labels changed execution: %#v", mode)
	}
	a, err := Compile(loaded, false)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Compile(loaded, false)
	if err != nil || !bytes.Equal(a, b) {
		t.Fatal("mode labels compile nondeterministically", err)
	}
}

func TestModeLabelsRejectAmbiguousPlacementAndContent(t *testing.T) {
	for _, body := range []string{
		`label chs "outside";`,
		`mode { option 1 { draw 1; label chs "late"; } option 2 {} }`,
		`mode { option 1 { label chs "a"; label chs "b"; } option 2 {} }`,
		`mode { option 1 { label fra "a"; } option 2 {} }`,
		`mode { option 1 { label chs " "; } option 2 {} }`,
		`mode { option 1 { label chs 123; } option 2 {} }`,
		`mode { option 1 { label "chs" "a"; } option 2 {} }`,
		`mode { option 1 { label chs "a" {} } option 2 {} }`,
		`mode { option 1 { if overflow { label chs "nested"; } } option 2 {} }`,
	} {
		file, ds := syntax.Parse("12345678.wbo", []byte(validCard("fanfare { "+body+" }")))
		if len(ds) == 0 && !hasErrors(ValidateFile(file)) {
			t.Fatalf("accepted invalid label: %s", body)
		}
	}
}
