package project

import (
	"testing"

	"wbo/internal/engine/ir"
)

// `halve cost 集合`：把目标当前费用变为向上取整的一半（官方 FAQ：9 → 5）。
func TestHalveCostCompiles(t *testing.T) {
	body := compiledFanfare(t, `fanfare {
		halve cost own.deck;
	}`)
	if len(body) != 1 {
		t.Fatalf("effects = %d", len(body))
	}
	effect, ok := body[0].(ir.AdjustEffect)
	if !ok || effect.Kind != "halve_cost" || effect.Field != "cost" {
		t.Fatalf("halve cost did not compile: %#v", body[0])
	}
	if target, ok := effect.Target.(ir.ZoneRef); !ok || target.Zone != "deck" || target.Member != "card" {
		t.Fatalf("halve cost lost its deck target: %#v", effect.Target)
	}
}

func TestHalveCostRejectsBadShapes(t *testing.T) {
	for _, line := range []string{
		"halve cost;",
		"halve deck;",
		"halve cost own.deck extra;",
		"halve countdown own.field;",
	} {
		if _, ds := compile(t, validCard("fanfare { "+line+" }")); len(ds) == 0 {
			t.Fatalf("%q must not compile", line)
		}
	}
}
