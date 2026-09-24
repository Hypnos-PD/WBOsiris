package runner

import (
	"strings"
	"testing"

	"wbo/internal/engine/ir"
)

// `halve cost 集合` 与 `reduce cost 集合 N minimum M` 都按当前费用作用于整副牌组，
// 且只影响对应成员（followers 不会碰法术）。重复减半基于已经改变的费用：
// 官方 FAQ 对『绚丽凤凰·小凤』写明奇数向上取整、第二次减半读当前费用。
func TestDeckWideCostChangesStack(t *testing.T) {
	const follower, spell = 70000001, 70000002
	pack := &ir.CardPack{Cards: []ir.Card{
		{ID: follower, CardType: "follower", Cost: 5, Stats: &ir.Stats{Attack: 1, Life: 1}},
		{ID: spell, CardType: "spell", Cost: 5},
	}}
	followerID, spellID, selfID := strings.Repeat("1", 32), strings.Repeat("2", 32), strings.Repeat("3", 32)
	state := testState()
	own := state.Players["own"]
	own = withInstance(own, "deck", ir.TestInstance{InstanceID: followerID, CardID: follower, DeclaredType: "follower"})
	own = withInstance(own, "deck", ir.TestInstance{InstanceID: spellID, CardID: spell, DeclaredType: "spell"})
	own = withInstance(own, "field", ir.TestInstance{InstanceID: selfID, CardID: follower, DeclaredType: "follower"})
	state.Players["own"] = own
	session, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	self := session.g.instances[selfID]
	deckFollowers := ir.ZoneRef{Kind: "zone", Side: "own", Zone: "deck", Member: "follower"}
	halve := ir.AdjustEffect{Kind: "halve_cost", Field: "cost", Target: deckFollowers}
	session.g.execAdjust(halve, self, frame{})
	if got := session.g.instances[followerID].cost; got != 3 {
		t.Fatalf("odd cost halved to %d, want 3", got)
	}
	if got := session.g.instances[spellID].cost; got != 5 {
		t.Fatalf("spell cost changed to %d", got)
	}
	session.g.execAdjust(halve, self, frame{})
	if got := session.g.instances[followerID].cost; got != 2 {
		t.Fatalf("second halving used %d, want 2", got)
	}
	session.g.execAdjust(ir.AdjustEffect{Kind: "adjust_entity_field", Field: "cost", Target: deckFollowers, Delta: -3, Minimum: 0}, self, frame{})
	if got := session.g.instances[followerID].cost; got != 0 {
		t.Fatalf("deck-wide reduction clamped to %d, want 0", got)
	}
	if got := session.g.instances[spellID].cost; got != 5 {
		t.Fatalf("spell cost changed by follower-only reduction: %d", got)
	}
}
