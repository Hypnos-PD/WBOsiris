package project

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"wbo/internal/ir"
	"wbo/internal/syntax"
)

func TestRestoreResourceCompilationAndFormatting(t *testing.T) {
	for _, side := range []string{"own", "oppo"} {
		file, ds := syntax.Parse("12345678.wbo", []byte(validCard(`fanfare { restore `+side+`.pp; }`)))
		if len(ds) != 0 || hasErrors(ValidateFile(file)) {
			t.Fatal("valid restoration rejected")
		}
		formatted := syntax.Format(file)
		again, ds := syntax.Parse("12345678.wbo", formatted)
		if len(ds) != 0 || !bytes.Equal(formatted, syntax.Format(again)) {
			t.Fatal("unstable restoration formatting")
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
		effect := pack.Cards[0].Abilities[0].Body[0].(ir.AdjustEffect)
		if effect.Kind != "restore_resource" || effect.Owner != side || effect.Resource != "pp" {
			t.Fatal("restoration lost resource or owner", effect)
		}
	}
	for _, body := range []string{`fanfare { restore pp; }`, `fanfare { restore own.ep; }`, `fanfare { restore own.pp 10; }`, `fanfare { restore field.pp; }`, `restore own.pp;`} {
		file, ds := syntax.Parse("12345678.wbo", []byte(validCard(body)))
		if len(ds) == 0 && !hasErrors(ValidateFile(file)) {
			t.Fatalf("accepted ambiguous restoration %q", body)
		}
	}
}
