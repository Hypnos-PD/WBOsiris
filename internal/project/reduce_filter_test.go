package project

import (
	"testing"

	"wbo/internal/ir"
)

// `reduce cost <集合> where <筛选> N minimum M`：筛选紧跟在集合之后，
// 用于"使自己的手牌中的所有梦魇·卡牌的费用-2"。
func TestFilteredReduceCostCompiles(t *testing.T) {
	body := compiledFanfare(t, `fanfare {
		reduce cost own.hand where class abysscraft 2 minimum 0;
	}`)
	if len(body) != 1 {
		t.Fatalf("expected one effect, got %d", len(body))
	}
	adjust, ok := body[0].(ir.AdjustEffect)
	if !ok || adjust.Kind != "adjust_entity_field" || adjust.Field != "cost" || adjust.Delta != -2 || adjust.Minimum != 0 {
		t.Fatalf("filtered reduce lost its shape: %#v", body[0])
	}
	filter, ok := adjust.Target.(ir.FilterRef)
	if !ok || filter.Source.(ir.ZoneRef).Zone != "hand" {
		t.Fatalf("filtered reduce lost its target: %#v", adjust.Target)
	}
}

func TestFilteredReduceRejectsBadShapes(t *testing.T) {
	for _, effect := range []string{
		"reduce cost own.hand where class abysscraft 2;",
		"reduce cost own.hand where 2 minimum 0;",
		"reduce countdown own.field where class abysscraft 2 minimum 0;",
	} {
		if _, ds := compile(t, validCard(effect)); len(ds) == 0 {
			t.Fatalf("%q must not compile", effect)
		}
	}
}
