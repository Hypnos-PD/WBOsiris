package project

import (
	"testing"

	"wbo/internal/ir"
)

// S-62：抽牌事件。`when own|oppo card drawn` 绑定 `drawn`；`when self drawn` 监听抽到的是本卡牌。
func TestDrawnEventCompiles(t *testing.T) {
	pack, ds := compile(t, validCard(`
		when own card drawn during own turn {
			damage oppo.field.followers drawn.cost;
		}
		when self drawn {
			set cost self 3 until turn ends;
		}`))
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	playerListener, ok := pack.Cards[0].Abilities[0].Trigger.(ir.EventTrigger)
	if !ok || playerListener.Event != "card_drawn" || playerListener.Side != "own" || playerListener.DuringTurn != "own" {
		t.Fatalf("draw listener did not compile: %#v", pack.Cards[0].Abilities[0].Trigger)
	}
	selfListener, ok := pack.Cards[0].Abilities[1].Trigger.(ir.EventTrigger)
	if !ok || selfListener.Event != "card_drawn" || !selfListener.SelfOnly || selfListener.SourceZone != "hand" {
		t.Fatalf("self draw listener did not compile: %#v", pack.Cards[0].Abilities[1].Trigger)
	}
	body := pack.Cards[0].Abilities[0].Body
	damage, ok := body[0].(ir.TargetEffect)
	if !ok || damage.AmountExpr == nil {
		t.Fatalf("drawn binding was not usable as an amount: %#v", body[0])
	}
	if _, ds := compile(t, validCard(`when own card fused during own turn { draw 1; }`)); len(ds) == 0 {
		t.Fatal("during turn scope was accepted for a fused listener")
	}
}
