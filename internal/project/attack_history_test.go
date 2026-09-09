package project

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"wbo/internal/ir"
	"wbo/internal/syntax"
)

func TestAttackHistoryConditionsCompileInBranchesAndEvents(t *testing.T) {
	for _, side := range []string{"own", "oppo"} {
		for _, prefix := range []string{"", "not "} {
			condition := prefix + side + ".attacked_this_turn"
			root := t.TempDir()
			path := filepath.Join(root, "12345", "12345678.wbo")
			if err := os.Mkdir(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}
			source := validCard(`fanfare { if ` + condition + ` { heal own.leader 4; } else { heal own.leader 2; } }
when own turn ends if ` + condition + ` { heal own.leader 1; }`)
			if err := os.WriteFile(path, []byte(source), 0600); err != nil {
				t.Fatal(err)
			}
			loaded := LoadWithRoot([]string{path}, true, root)
			pack, _, err := BuildRuntimePacks(loaded)
			if err != nil {
				t.Fatal(err, loaded.Diagnostics)
			}
			encoded, err := ir.EncodeCardPack(*pack)
			if err != nil {
				t.Fatal(err)
			}
			decoded, err := ir.DecodeCardPack(encoded)
			if err != nil {
				t.Fatal(err)
			}
			want := ir.AttackHistoryCondition{Kind: "attack_history", Side: side, Attacked: prefix == ""}
			branch := decoded.Cards[0].Abilities[0].Body[0].(ir.IfEffect)
			event := decoded.Cards[0].Abilities[1].Trigger.(ir.EventTrigger)
			if !reflect.DeepEqual(branch.Condition, want) || !reflect.DeepEqual(event.Condition, want) || len(branch.Else) != 1 {
				t.Fatal("attack history changed across compilation", condition)
			}
		}
	}
}

func TestAttackHistoryRejectsAmbiguousSyntax(t *testing.T) {
	for _, condition := range []string{
		"own.attacked_this_turn == false", "own.attacked_this_turn == 0", "not not own.attacked_this_turn",
		"self.attacked_this_turn", "enemy.attacked_this_turn", "own.attacked_this_turn extra",
		"not own", "not own.evolve_unlocked", "own.attack_this_turn",
	} {
		f, ds := syntax.Parse("12345678.wbo", []byte(validCard("fanfare { if "+condition+" { draw 1; } }")))
		if len(ds) == 0 && !hasErrors(ValidateFile(f)) {
			t.Fatal("accepted invalid condition", condition)
		}
	}
}
