package project

import (
	"os"
	"path/filepath"
	"testing"
	"wbo/internal/ir"
	"wbo/internal/syntax"
)

func TestCombatOpponentCompilesAndRoundTrips(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "12345")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "12345678.wbo")
	source := validCard(`attack { destroy opponent; }
clash { damage opponent 3; }
attack { if own.combo >= 0 { repeat 2 { damage opponent 1; } } }
clash { mode { option 1 { destroy opponent; } option 2 { damage opponent 1; } } }`)
	if err := os.WriteFile(path, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	loaded := LoadWithRoot([]string{path}, true, root)
	pack, _, err := BuildRuntimePacks(loaded)
	if err != nil {
		t.Fatal(err, loaded.Diagnostics)
	}
	data, err := ir.EncodeCardPack(*pack)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := ir.DecodeCardPack(data)
	if err != nil {
		t.Fatal(err)
	}
	for _, ability := range decoded.Cards[0].Abilities[:2] {
		if ref := ability.Body[0].(ir.TargetEffect).Target.(ir.BindingRef); ref.Name != "opponent" {
			t.Fatal(ref)
		}
	}
}

func TestCombatOpponentDoesNotEscapeAbilityScope(t *testing.T) {
	for _, body := range []string{
		`destroy opponent;`,
		`fanfare { destroy opponent; }`,
		`attack { destroy opponent; } lastwords { destroy opponent; }`,
		`attack { grant self { lastwords { destroy opponent; } } }`,
	} {
		f, ds := syntax.Parse("12345678.wbo", []byte(validCard(body)))
		if len(ds) == 0 && !hasErrors(ValidateFile(f)) {
			t.Fatal("accepted out-of-scope opponent", body)
		}
	}
}
