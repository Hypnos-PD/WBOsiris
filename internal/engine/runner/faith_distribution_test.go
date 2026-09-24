package runner

import (
	"strings"
	"testing"

	"wbo/internal/engine/ir"
)

// 信仰值的逐点随机分配：每一点都恰好落在某个选项上（合计等于信仰值），且不会消费信仰值。
func TestFaithDistributionAssignsEveryPoint(t *testing.T) {
	sourceID, spellID := 70000001, 70000002
	pack := &ir.CardPack{Cards: []ir.Card{
		{
			ID: sourceID, CardType: "follower", Stats: &ir.Stats{Attack: 1, Life: 1},
			Faith: &ir.CrestDefinition{Counters: map[string]int{"value": 0}, Locales: map[string]ir.Locale{}, Abilities: []ir.Ability{}},
		},
		{
			ID: spellID, CardType: "spell", PlayEffects: []ir.Effect{
				ir.DistributeFaithEffect{Kind: "distribute_faith", FaithID: sourceID, Options: []ir.ModeOption{
					{ID: 1, Body: []ir.Effect{ir.TargetEffect{Kind: "damage", Target: ir.LeaderRef{Kind: "leader", Side: "oppo"}, Amount: 1}}},
					{ID: 2, Body: []ir.Effect{ir.TargetEffect{Kind: "damage", Target: ir.LeaderRef{Kind: "leader", Side: "own"}, Amount: 1}}},
				}},
			},
		},
	}}
	spell, faith := strings.Repeat("1", 32), strings.Repeat("2", 32)
	state := testState()
	own := state.Players["own"]
	own = withInstance(own, "deck", ir.TestInstance{InstanceID: strings.Repeat("3", 32), CardID: sourceID, DeclaredType: "follower"})
	own = withInstance(own, "hand", ir.TestInstance{InstanceID: spell, CardID: spellID, DeclaredType: "spell"})
	own = withInstance(own, "crests", ir.TestInstance{InstanceID: faith, CardID: sourceID, DeclaredType: "faith", Overrides: ir.InstanceOverrides{Counters: map[string]int{"value": 5}}})
	state.Players["own"] = own
	session, err := NewSession(pack, state, 7)
	if err != nil {
		t.Fatal(err)
	}
	if result := session.Begin(strings.Repeat("a", 32), ir.SourceAction{Kind: "play", Actor: "own", Source: spell}); result.Status != StatusCompleted {
		t.Fatalf("play did not complete: %#v", result)
	}
	assigned := (20 - session.g.own.leaderLife) + (20 - session.g.oppo.leaderLife)
	if assigned != 5 {
		t.Fatalf("assigned points = %d, want 5 (own=%d oppo=%d)", assigned, session.g.own.leaderLife, session.g.oppo.leaderLife)
	}
	if got := session.g.instances[faith].counters["value"]; got != 5 {
		t.Fatalf("faith value = %d, want 5 (distribution must not consume it)", got)
	}
}
