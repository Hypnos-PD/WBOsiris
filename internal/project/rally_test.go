package project

import (
	"testing"

	"wbo/internal/ir"
)

// `rally >= N`（协作）：条件读取本方"进入过战场的随从数量"计数器。
// 官方术语是 Rally，官方 QA 明确打出本卡牌自身不计入本次判断。
func TestRallyConditionAndStateCompile(t *testing.T) {
	pack, ds := compile(t, validCard(`
		fanfare {
			if rally >= 20 {
				draw 1;
			}
			summon 1 card 12345678 for oppo;
		}`))
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	body := pack.Cards[0].Abilities[0].Body
	condition, ok := body[0].(ir.IfEffect).Condition.(ir.CompareCondition)
	if !ok || condition.Left.Field != "rally" || condition.Left.Side != "own" || condition.Op != "ge" || condition.Right != 20 {
		t.Fatalf("rally condition not preserved: %#v", body[0])
	}
	summon := body[1].(ir.CardEffect)
	if summon.Kind != "summon" || summon.Owner != "oppo" {
		t.Fatalf("cross-side summon not preserved: %#v", summon)
	}
}

func TestRallyAndCrossSideSummonRejectBadShapes(t *testing.T) {
	for _, line := range []string{
		"if rally { draw 1; }",
		"if rally >= ;",
		"summon 1 card 12345678 for;",
		"summon 1 card 12345678 for team;",
	} {
		if _, ds := compile(t, validCard("fanfare { "+line+" }")); len(ds) == 0 {
			t.Fatalf("%q must not compile", line)
		}
	}
}
