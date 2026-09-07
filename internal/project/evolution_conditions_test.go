package project

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"wbo/internal/ir"
	"wbo/internal/syntax"
)

func TestCompileEvolutionConditions(t *testing.T) {
	for _, tc := range []struct {
		source string
		want   ir.Condition
	}{
		{"self form unevolved", ir.SelfFormCondition{Kind: "self_form", Form: "unevolved"}},
		{"self form evolved", ir.SelfFormCondition{Kind: "self_form", Form: "evolved"}},
		{"self form super_evolved", ir.SelfFormCondition{Kind: "self_form", Form: "super_evolved"}},
		{"own.evolve_unlocked", ir.EvolutionUnlockedCondition{Kind: "evolution_unlocked", Side: "own", Form: "evolved"}},
		{"oppo.evolve_unlocked", ir.EvolutionUnlockedCondition{Kind: "evolution_unlocked", Side: "oppo", Form: "evolved"}},
		{"own.superevolve_unlocked", ir.EvolutionUnlockedCondition{Kind: "evolution_unlocked", Side: "own", Form: "super_evolved"}},
		{"oppo.superevolve_unlocked", ir.EvolutionUnlockedCondition{Kind: "evolution_unlocked", Side: "oppo", Form: "super_evolved"}},
	} {
		t.Run(tc.source, func(t *testing.T) {
			root := t.TempDir()
			dir := filepath.Join(root, "12345")
			if err := os.Mkdir(dir, 0700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "12345678.wbo")
			source := validCard(`fanfare { if ` + tc.source + ` { heal own.leader 4; } else { heal own.leader 2; } }`)
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
			e := decoded.Cards[0].Abilities[0].Body[0].(ir.IfEffect)
			if !reflect.DeepEqual(e.Condition, tc.want) || len(e.Then) != 1 || len(e.Else) != 1 {
				t.Fatal("condition or branches changed", e)
			}
		})
	}
}

func TestRejectMalformedEvolutionConditions(t *testing.T) {
	for _, condition := range []string{
		"self form", "self form normal", "target form evolved", "self form evolved extra",
		"self.form == super_evolved", "own.evolve_unlocked == 1", "own.superevolve_unlocked extra",
		"self.evolve_unlocked", "enemy.evolve_unlocked", "evolve_unlocked",
	} {
		f, ds := syntax.Parse("12345678.wbo", []byte(validCard("fanfare { if "+condition+" { draw 1; } }")))
		if len(ds) == 0 && !hasErrors(ValidateFile(f)) {
			t.Fatal("accepted invalid condition", condition)
		}
	}
}
