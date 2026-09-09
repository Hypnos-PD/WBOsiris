package project

import (
	"os"
	"path/filepath"
	"testing"

	"wbo/internal/ir"
	"wbo/internal/syntax"
)

func TestCompileSetDamageReduction(t *testing.T) {
	f, ds := syntax.Parse("12345678.wbo", []byte(validCard("fanfare { choose target from own.field.followers; set_damage_reduction target 3; }")))
	if len(ds) != 0 || hasErrors(ValidateFile(f)) {
		t.Fatalf("diagnostics: %v", append(ds, ValidateFile(f)...))
	}
	root := t.TempDir()
	path := filepath.Join(root, "12345", "12345678.wbo")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(validCard("fanfare { set_damage_reduction self 3; }")), 0600); err != nil {
		t.Fatal(err)
	}
	pack, _, err := BuildRuntimePacks(LoadWithRoot([]string{path}, true, root))
	if err != nil {
		t.Fatal(err)
	}
	if len(pack.Cards) != 1 {
		t.Fatalf("compiled cards = %d", len(pack.Cards))
	}
	if len(pack.Cards[0].Abilities) != 1 || len(pack.Cards[0].Abilities[0].Body) != 1 {
		t.Fatalf("compiled effects = %d", len(pack.Cards[0].Abilities))
	}
	e := pack.Cards[0].Abilities[0].Body[0].(ir.TargetEffect)
	if e.Kind != "set_damage_reduction" || e.Amount != 3 {
		t.Fatalf("compiled effect = %#v", e)
	}
}

func TestRejectNegativeDamageReduction(t *testing.T) {
	f, ds := syntax.Parse("12345678.wbo", []byte(validCard("fanfare { set_damage_reduction self -1; }")))
	if len(ds) == 0 && !hasErrors(ValidateFile(f)) {
		t.Fatal("accepted negative damage reduction")
	}
}
