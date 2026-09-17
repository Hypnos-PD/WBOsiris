package runner

import (
	"strings"
	"testing"

	"wbo/internal/ir"
)

// 真理的研究设施：「自己使用费用发生变化的随从时，抽 1 张并让本护符的倒计数 -1」。
// 场景测试一个动作只能做一件事（改费用或打出随从），所以整条链路由 Go 测试覆盖。
func TestCostChangedPlayTrigger(t *testing.T) {
	pack := loadCardsForTest(t,
		"10003/10332210", "10003/10333310", "10000/10001110", "90000/90011110")
	amuletID, spellID, followerID := strings.Repeat("1", 32), strings.Repeat("2", 32), strings.Repeat("3", 32)
	deckID := strings.Repeat("4", 32)
	state := testState()
	own := state.Players["own"]
	own.PP, own.MaxPP = 5, 5
	own = withInstance(own, "field", ir.TestInstance{InstanceID: amuletID, CardID: 10332210, DeclaredType: "amulet", Overrides: ir.InstanceOverrides{Countdown: ptrInt(5)}})
	own = withInstance(own, "hand", ir.TestInstance{InstanceID: spellID, CardID: 10333310, DeclaredType: "spell"})
	own = withInstance(own, "hand", ir.TestInstance{InstanceID: followerID, CardID: 10001110, DeclaredType: "follower"})
	own = withInstance(own, "deck", ir.TestInstance{InstanceID: deckID, CardID: 90011110, DeclaredType: "follower"})
	state.Players["own"] = own
	session, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	// 先用一张法术给手牌随从加费（费用发生变化），再打出它。
	if result := session.Begin(strings.Repeat("a", 32), ir.SourceAction{Kind: "play", Actor: "own", Source: spellID}); result.Status != StatusSuspended {
		t.Fatalf("cost changing spell did not ask for a target: %#v", result)
	}
	choice := session.PendingChoice()
	if result := session.Resume(ChoiceResponse{RequestID: choice.RequestID, ActionID: choice.ActionID, StateRevision: choice.StateRevision, SelectedInstanceIDs: []string{followerID}}); result.Status != StatusCompleted {
		t.Fatalf("cost changing spell failed: %#v", result)
	}
	if follower := session.g.instances[followerID]; !follower.costChanged || follower.cost != 3 {
		t.Fatalf("follower cost was not changed: cost=%d changed=%v", follower.cost, follower.costChanged)
	}
	before := handCardCount(session.g.own.hand, 90011110)
	if result := session.Begin(strings.Repeat("b", 32), ir.SourceAction{Kind: "play", Actor: "own", Source: followerID}); result.Status != StatusCompleted {
		t.Fatalf("playing the follower failed: %#v", result)
	}
	if after := handCardCount(session.g.own.hand, 90011110); after != before+1 {
		t.Fatalf("trigger did not draw a card: %d -> %d", before, after)
	}
	if countdown := session.g.instances[amuletID].countdown; countdown != 4 {
		t.Fatalf("countdown = %d, want 4", countdown)
	}
}

func ptrInt(value int) *int { return &value }
