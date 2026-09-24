package project

import (
	"testing"

	"wbo/internal/engine/ir"
)

// `double stats 集合`：按每个目标自己的当前数值翻倍攻击力与生命值。
func TestDoubleStatsCompiles(t *testing.T) {
	body := compiledFanfare(t, `fanfare {
		double stats own.field.followers;
	}`)
	if len(body) != 1 {
		t.Fatalf("effects = %d", len(body))
	}
	effect, ok := body[0].(ir.AdjustEffect)
	if !ok || effect.Kind != "double_stats" {
		t.Fatalf("double stats did not compile: %#v", body[0])
	}
	if target, ok := effect.Target.(ir.ZoneRef); !ok || target.Side != "own" || target.Zone != "field" || target.Member != "follower" {
		t.Fatalf("double stats lost its target set: %#v", effect.Target)
	}
}

func TestDoubleStatsRejectsBadShapes(t *testing.T) {
	for _, line := range []string{
		"double stats;",
		"double cost own.deck;",
		"double stats own.field extra;",
	} {
		if _, ds := compile(t, validCard("fanfare { "+line+" }")); len(ds) == 0 {
			t.Fatalf("%q must not compile", line)
		}
	}
}
