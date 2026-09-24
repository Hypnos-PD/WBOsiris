package runner

import (
	"strings"
	"testing"

	"wbo/internal/engine/ir"
)

// 「使自己的信仰获得「…」」：把事件监听附加到信仰实体上，随后的事件会照常触发它。
func TestGrantedFaithAbilityListensForLaterEvents(t *testing.T) {
	sourceID, spellID, summonID := 71000001, 71000002, 71000003
	pack := &ir.CardPack{Cards: []ir.Card{
		{
			ID: sourceID, CardType: "follower", Stats: &ir.Stats{Attack: 1, Life: 1},
			Faith: &ir.CrestDefinition{Counters: map[string]int{"value": 0}, Locales: map[string]ir.Locale{}, Abilities: []ir.Ability{}},
		},
		{ID: summonID, CardType: "follower", Stats: &ir.Stats{Attack: 1, Life: 1}},
		{
			ID: spellID, CardType: "spell",
			Faith: &ir.CrestDefinition{Counters: map[string]int{"value": 0}, Locales: map[string]ir.Locale{}, Abilities: []ir.Ability{}},
			PlayEffects: []ir.Effect{
				ir.GrantEffect{
					NodeBase: ir.NodeBase{ID: strings.Repeat("8", 32)},
					Kind:   "grant_ability",
					Target: ir.FaithRef{Kind: "faith", ValueType: "entity"},
					Ability: ir.Ability{
						ID:      strings.Repeat("9", 32),
						Trigger: ir.EventTrigger{Kind: "event", Event: "follower_summoned", Side: "own", SubjectType: "follower"},
						Body:    []ir.Effect{ir.TargetEffect{Kind: "damage", Target: ir.LeaderRef{Kind: "leader", Side: "oppo"}, Amount: 1}},
					},
				},
				ir.CardEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("7", 32)}, Kind: "summon", Owner: "own", Count: 1, CardID: summonID, Output: "summoned"},
			},
		},
	}}
	spell, faith := strings.Repeat("1", 32), strings.Repeat("2", 32)
	state := testState()
	own := state.Players["own"]
	own = withInstance(own, "hand", ir.TestInstance{InstanceID: spell, CardID: spellID, DeclaredType: "spell"})
	own = withInstance(own, "crests", ir.TestInstance{InstanceID: faith, CardID: spellID, DeclaredType: "faith"})
	state.Players["own"] = own
	session, err := NewSession(pack, state, 3)
	if err != nil {
		t.Fatal(err)
	}
	if result := session.Begin(strings.Repeat("a", 32), ir.SourceAction{Kind: "play", Actor: "own", Source: spell}); result.Status != StatusCompleted {
		t.Fatalf("play did not complete: %#v", result)
	}
	if session.g.oppo.leaderLife != 19 {
		t.Fatalf("granted faith listener did not fire: oppo life = %d", session.g.oppo.leaderLife)
	}
	if len(session.g.instances[faith].grants) != 1 {
		t.Fatalf("grant was not stored on the faith: %#v", session.g.instances[faith].grants)
	}
}
