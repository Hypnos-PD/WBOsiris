package runner

import (
	"strings"
	"testing"

	"wbo/internal/engine/ir"
)

// 官方 QA（eilph_9kmewn）：『绚丽凤凰·小凤』只会把"发动时在牌组里"的卡牌费用减半；
// 被减费的『天司长的继承者·圣德芬』瞬念召唤后返回手牌时，费用回到原始费用 6。
func TestInvokeRestoresDeckScopedCost(t *testing.T) {
	const source = 77883001
	pack := &ir.CardPack{Cards: []ir.Card{{
		ID: source, CardType: "follower", Cost: 6, Stats: &ir.Stats{Attack: 3, Life: 3},
		Abilities: []ir.Ability{{
			ID: strings.Repeat("a", 32), Trigger: ir.EventTrigger{Kind: "event", Event: "turn_started", Side: "own", SourceZone: "deck"},
			Body: []ir.Effect{ir.InvokeEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("b", 32)}, Kind: "invoke", Target: ir.SelfRef{}}},
		}},
	}}}
	state := testState()
	sourceID := strings.Repeat("1", 32)
	state.Players["own"] = withInstance(state.Players["own"], "deck", ir.TestInstance{InstanceID: sourceID, CardID: source, DeclaredType: "follower"})
	session, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	card := session.g.instances[sourceID]
	// 『绚丽凤凰·小凤』的【入场曲】：使自己的牌组中的所有卡牌的费用变为一半。
	session.g.execAdjust(ir.AdjustEffect{
		NodeBase: ir.NodeBase{ID: strings.Repeat("c", 32)}, Kind: "halve_cost",
		Target: ir.ZoneRef{Kind: "zone", Side: "own", Zone: "deck"},
	}, card, frame{})
	if card.cost != 3 {
		t.Fatalf("deck halving = %d, want 3", card.cost)
	}
	session.g.invokeInstance(card)
	if card.zone != "field" || card.cost != 6 {
		t.Fatalf("invoked card zone=%s cost=%d, want field/6", card.zone, card.cost)
	}
}
