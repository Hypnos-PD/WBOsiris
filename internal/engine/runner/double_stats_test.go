package runner

import (
	"strings"
	"testing"

	"wbo/internal/engine/ir"
)

// 翻倍按每个目标的当前数值计算：已受伤害一并翻倍，因此上限与当前生命一起变化；
// 同集合里的护符不受影响。
func TestDoubleStatsUsesCurrentValues(t *testing.T) {
	const follower, amulet = 70000001, 70000002
	pack := &ir.CardPack{Cards: []ir.Card{
		{ID: follower, CardType: "follower", Stats: &ir.Stats{Attack: 3, Life: 6}},
		{ID: amulet, CardType: "amulet"},
	}}
	followerID, amuletID := strings.Repeat("1", 32), strings.Repeat("2", 32)
	state := testState()
	own := state.Players["own"]
	own = withInstance(own, "field", ir.TestInstance{InstanceID: followerID, CardID: follower, DeclaredType: "follower", Overrides: ir.InstanceOverrides{Stats: &ir.Stats{Attack: 3, Life: 6}, DamageTaken: ptr(4)}})
	own = withInstance(own, "field", ir.TestInstance{InstanceID: amuletID, CardID: amulet, DeclaredType: "amulet"})
	state.Players["own"] = own
	session, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	self := session.g.instances[followerID]
	session.g.execAdjust(ir.AdjustEffect{Kind: "double_stats", Target: ir.ZoneRef{Kind: "zone", Side: "own", Zone: "field"}}, self, frame{})
	unit := session.g.instances[followerID]
	if unit.attack != 6 || unit.life != 4 || unit.damageTaken != 8 {
		t.Fatalf("doubling = %d/%d with %d damage", unit.attack, unit.life, unit.damageTaken)
	}
	if amulet := session.g.instances[amuletID]; amulet.attack != 0 || amulet.life != 0 {
		t.Fatalf("amulet changed: %d/%d", amulet.attack, amulet.life)
	}
}

func ptr(value int) *int { return &value }
