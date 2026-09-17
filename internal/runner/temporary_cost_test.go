package runner

import (
	"strings"
	"testing"

	"wbo/internal/ir"
)

// 「回合结束前，使其费用变为 0」：临时费用修改在持有者回合结束时按差量还原。
// 场景测试只能写一个动作（打牌或结束回合二选一），所以这条链路由 Go 测试覆盖。
func TestTemporaryCostRevertsAtTurnEnd(t *testing.T) {
	pack := loadCardsForTest(t,
		"10003/10334120", "10001/10122310", "90000/90034310",
		"90000/90021110", "90000/90021120")
	sourceID, spellID := strings.Repeat("1", 32), strings.Repeat("2", 32)
	state := testState()
	own := state.Players["own"]
	own.PP, own.MaxPP = 9, 9
	own = withInstance(own, "hand", ir.TestInstance{InstanceID: sourceID, CardID: 10334120, DeclaredType: "follower"})
	own = withInstance(own, "hand", ir.TestInstance{InstanceID: spellID, CardID: 10122310, DeclaredType: "spell"})
	state.Players["own"] = own
	session, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	if result := session.Begin(strings.Repeat("a", 32), ir.SourceAction{Kind: "play", Actor: "own", Source: sourceID}); result.Status != StatusCompleted {
		t.Fatalf("playing the card failed: %#v", result)
	}
	transformed := session.g.instances[spellID]
	if transformed.card.ID != 90034310 || transformed.cost != 0 {
		t.Fatalf("spell was not transformed and costed: card=%d cost=%d", transformed.card.ID, transformed.cost)
	}
	if result := session.Begin(strings.Repeat("b", 32), ir.SourceAction{Kind: "end_turn", Actor: "own"}); result.Status != StatusCompleted {
		t.Fatalf("ending the turn failed: %#v", result)
	}
	if got := session.g.instances[spellID].cost; got != 4 {
		t.Fatalf("temporary cost did not revert: %d", got)
	}
	if len(session.g.instances[spellID].temporaryCost) != 0 {
		t.Fatalf("temporary cost was not cleared: %#v", session.g.instances[spellID].temporaryCost)
	}
}
