package project

import (
	"os"
	"path/filepath"
	"testing"

	"wbo/internal/ir"
	"wbo/internal/syntax"
)

func TestCompileSpellboostFilter(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "12345")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "12345678.wbo")
	source := validCard(`fanfare {
        require boosted from own.hand where spellboost and type spell;
        spellboost boosted 1;
    }`)
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
	selection := decoded.Cards[0].Abilities[0].Body[0].(ir.SelectionEffect)
	predicate := selection.Source.(ir.FilterRef).Predicate.(ir.AndPredicate)
	if predicate.Terms[0].(ir.FieldPredicate).Kind != "has_spellboost" || len(predicate.Terms) != 2 {
		t.Fatal("lost spellboost filter", predicate)
	}
	boost := decoded.Cards[0].Abilities[0].Body[1].(ir.AdjustEffect)
	if boost.Times != 1 || boost.Target.(ir.BindingRef).Name != "boosted" {
		t.Fatal("lost selected boost target", boost)
	}
}

func TestSpellboostSelectionRejectsUndefinedBindingAndMalformedFilter(t *testing.T) {
	for _, body := range []string{
		"spellboost missing 1;",
		"choose target from own.hand where spellboost true;",
		"choose target from own.hand where spellboost and;",
		"choose target from own.hand where spellboost == 1;",
	} {
		f, ds := syntax.Parse("12345678.wbo", []byte(validCard("fanfare { "+body+" }")))
		if len(ds) == 0 && !hasErrors(ValidateFile(f)) {
			t.Fatal("accepted invalid spellboost operation", body)
		}
	}
}
