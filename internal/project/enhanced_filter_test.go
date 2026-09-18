package project

import (
	"testing"

	"wbo/internal/ir"
)

// S-77：`where enhanced` 匹配"本次通过【爆能强化】打出的卡牌"。
func TestEnhancedFilterCompiles(t *testing.T) {
	pack, ds := compile(t, validCard(`
		when own card played where enhanced {
			draw 1;
		}`))
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	trigger, ok := pack.Cards[0].Abilities[0].Trigger.(ir.EventTrigger)
	if !ok || trigger.Event != "card_played" || trigger.Predicate == nil {
		t.Fatalf("play listener did not compile: %#v", pack.Cards[0].Abilities[0].Trigger)
	}
	predicate, ok := trigger.Predicate.(ir.FieldPredicate)
	if !ok || predicate.Kind != "was_enhanced" {
		t.Fatalf("enhanced filter did not compile: %#v", trigger.Predicate)
	}
	// 同一词条也能用在普通筛选中。
	if _, ds := compile(t, validCard(`
		fanfare {
			damage oppo.field.followers 1 where enhanced;
		}`)); len(ds) != 0 {
		t.Fatal(ds)
	}
}
