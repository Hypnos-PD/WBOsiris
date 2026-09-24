package project

import (
	"testing"

	"wbo/internal/engine/ir"
)

// `own|oppo.deck has [no] duplicates` 条件与 `banish duplicates in own|oppo.deck` 操作。
func TestDeckDuplicatesConditionAndDedupe(t *testing.T) {
	pack, ds := compile(t, validCard(`
		fanfare {
			if own.deck has no duplicates {
				gain own.pp 3;
			}
			if oppo.deck has duplicates {
				draw 1;
			}
			banish duplicates in own.deck;
			banish duplicates in oppo.deck;
		}`))
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	body := pack.Cards[0].Abilities[0].Body
	own := body[0].(ir.IfEffect).Condition.(ir.DeckDuplicatesCondition)
	if own.Kind != "deck_duplicates" || own.Side != "own" || !own.Unique {
		t.Fatalf("unique deck condition not preserved: %#v", own)
	}
	oppo := body[1].(ir.IfEffect).Condition.(ir.DeckDuplicatesCondition)
	if oppo.Side != "oppo" || oppo.Unique {
		t.Fatalf("duplicate deck condition not preserved: %#v", oppo)
	}
	ownDedupe := body[2].(ir.CardEffect)
	if ownDedupe.Kind != "banish_duplicates" || ownDedupe.Owner != "own" || ownDedupe.Destination != "deck" {
		t.Fatalf("own dedupe not preserved: %#v", ownDedupe)
	}
	if oppoDedupe := body[3].(ir.CardEffect); oppoDedupe.Owner != "oppo" {
		t.Fatalf("opponent dedupe not preserved: %#v", oppoDedupe)
	}
}

func TestDeckShapeRejectsBadShapes(t *testing.T) {
	for _, line := range []string{
		"if own.deck has duplicates extra { draw 1; }",
		"if deck has no duplicates { draw 1; }",
		"banish duplicates in deck;",
		"banish duplicates from own.deck;",
	} {
		if _, ds := compile(t, validCard("fanfare { "+line+" }")); len(ds) == 0 {
			t.Fatalf("%q must not compile", line)
		}
	}
}
