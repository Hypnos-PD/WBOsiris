package project

import (
	"testing"

	"wbo/internal/ir"
)

// `when own card played [other] [where …]`：监听自己使用的卡牌，支持 `other`（不含本卡牌）
// 与筛选（类型、trait、费用…）。监听对象绑成 `played`。
func TestCardPlayedEventCompiles(t *testing.T) {
	pack, ds := compile(t, validCard(`
		when own card played where trait loot {
			draw 1;
		}
		when own card played other {
			damage oppo.leader 1;
		}`))
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	ability := pack.Cards[0].Abilities[0]
	trigger, ok := ability.Trigger.(ir.EventTrigger)
	if !ok || trigger.Event != "card_played" || trigger.Side != "own" || trigger.SubjectType != "" {
		t.Fatalf("card played trigger not preserved: %#v", ability.Trigger)
	}
	if trigger.Predicate == nil {
		t.Fatalf("trait filter was dropped: %#v", trigger)
	}
	exclude := pack.Cards[0].Abilities[1].Trigger.(ir.EventTrigger)
	if !exclude.ExcludeSelf {
		t.Fatalf("other was dropped: %#v", exclude)
	}
}

func TestCardPlayedEventRejectsBadShapes(t *testing.T) {
	for _, line := range []string{
		"when own card play { draw 1; }",
		"when card played { draw 1; }",
	} {
		if _, ds := compile(t, validCard(line)); len(ds) == 0 {
			t.Fatalf("%q must not compile", line)
		}
	}
}
