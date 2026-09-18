package runner

import (
	"strings"
	"testing"

	"wbo/internal/ir"
)

// S-56：`raise cost T N until ...` 是临时加费——到期按差量还原，不覆盖之后的永久加减费。
func TestTemporaryCostRaiseExpiresAtTurnEnd(t *testing.T) {
	g := &game{}
	card := &ir.Card{ID: 12345678, CardType: "follower", Cost: 2, Stats: &ir.Stats{Attack: 2, Life: 2}}
	held := &instance{id: "held", zone: "hand", card: card, cost: 2, abilities: map[string]bool{}}
	g.own.hand = []*instance{held}
	g.instances = map[string]*instance{"held": held}
	session := &Session{g: g, actionID: strings.Repeat("a", 32)}
	target := ir.ZoneRef{Kind: "zone", Side: "own", Zone: "hand", Member: "card"}
	session.g.execAdjust(ir.AdjustEffect{Kind: "adjust_entity_field", Field: "cost", Target: target, Delta: 1, Until: "oppo_turn_end"}, nil, frame{})
	if held.cost != 3 {
		t.Fatalf("temporary raise did not apply: cost=%d", held.cost)
	}
	if held.temporaryCost["oppo"] != 1 {
		t.Fatalf("temporary diff not recorded: %#v", held.temporaryCost)
	}
	// 到期前又发生一次永久加费，到期只应撤销自己的差量。
	session.g.execAdjust(ir.AdjustEffect{Kind: "adjust_entity_field", Field: "cost", Target: target, Delta: 2}, nil, frame{})
	if held.cost != 5 {
		t.Fatalf("permanent raise did not apply: cost=%d", held.cost)
	}
	if !g.expireTurnEffects("oppo") {
		t.Fatal("expiry was rejected")
	}
	if held.cost != 4 || len(held.temporaryCost) != 0 {
		t.Fatalf("temporary cost did not expire cleanly: cost=%d temporary=%#v", held.cost, held.temporaryCost)
	}
}
