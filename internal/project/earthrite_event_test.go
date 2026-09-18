package project

import (
	"testing"

	"wbo/internal/ir"
)

// S-82：`when own earthrite while self in hand` —— 自己发动【土之秘术】时，
// 手牌中的卡牌（召唤仆从、饕餮魔咒）减费。
func TestEarthriteEventCompilesIntoHandListener(t *testing.T) {
	pack, ds := compile(t, validCard(`when own earthrite while self in hand {
		reduce cost self 1 minimum 0;
	}`))
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	ability := pack.Cards[0].Abilities[0]
	trigger, ok := ability.Trigger.(ir.EventTrigger)
	if !ok || trigger.Event != "earthrite" || trigger.Side != "own" || trigger.SourceZone != "hand" {
		t.Fatalf("earthrite listener lost its shape: %#v", ability.Trigger)
	}
}

func TestEarthriteEventRejectsExtraClauses(t *testing.T) {
	for _, effect := range []string{
		"when own earthrite during own turn while self in hand { draw 1; }",
		"when self earthrite while self in hand { draw 1; }",
		"when own earthrite while self in deck { draw 1; }",
	} {
		if _, ds := compile(t, validCard(effect)); len(ds) == 0 {
			t.Fatalf("%q must not compile", effect)
		}
	}
}
