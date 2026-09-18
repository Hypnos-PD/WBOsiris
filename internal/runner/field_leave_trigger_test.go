package runner

import (
	"strings"
	"testing"

	"wbo/internal/ir"
)

// fieldLeaveFixture 造两张卡：own 打出的进攻者，以及 oppo 场上的监听者。
// 监听者的能力是"对手的随从进入战场时，对对手的主战者造成 3 点伤害"。
// 进攻者的【入场曲】可以选择是否破坏对方的战场随从（也就是监听者自己）。
func fieldLeaveFixture(listenerID, invaderID int, destroys bool) (*ir.CardPack, ir.TestInstance, ir.TestInstance) {
	listener := ir.Card{ID: listenerID, CardType: "follower", Cost: 1, Stats: &ir.Stats{Attack: 1, Life: 1}, Abilities: []ir.Ability{{
		ID: strings.Repeat("a", 32), Trigger: ir.EventTrigger{Kind: "event", Event: "follower_summoned", Side: "oppo"},
		Body: []ir.Effect{ir.TargetEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("b", 32)}, Kind: "damage", Target: ir.LeaderRef{Kind: "leader", Side: "oppo"}, Amount: 3}},
	}}}
	invader := ir.Card{ID: invaderID, CardType: "follower", Cost: 0, Stats: &ir.Stats{Attack: 1, Life: 1}}
	if destroys {
		invader.Abilities = []ir.Ability{{
			ID: strings.Repeat("c", 32), Trigger: ir.SimpleTrigger{Kind: "fanfare"},
			Body: []ir.Effect{ir.TargetEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("d", 32)}, Kind: "destroy",
				Target: ir.ZoneRef{Kind: "zone", Side: "oppo", Zone: "field", Member: "follower"}}},
		}}
	}
	pack := &ir.CardPack{Cards: []ir.Card{listener, invader}}
	return pack, ir.TestInstance{InstanceID: strings.Repeat("1", 32), CardID: invaderID, DeclaredType: "follower"},
		ir.TestInstance{InstanceID: strings.Repeat("2", 32), CardID: listenerID, DeclaredType: "follower"}
}

func playInvader(t *testing.T, pack *ir.CardPack, invader, listener ir.TestInstance) *Session {
	t.Helper()
	state := testState()
	state.Turn.Active = "own"
	state.Players["own"] = withInstance(state.Players["own"], "hand", invader)
	state.Players["oppo"] = withInstance(state.Players["oppo"], "field", listener)
	session, err := NewSession(pack, state, 3)
	if err != nil {
		t.Fatal(err)
	}
	if result := session.SubmitAs(strings.Repeat("9", 32), "own", SimulatorCommand{Kind: "play", Source: invader.InstanceID}); result.Status != StatusCompleted {
		t.Fatal(result)
	}
	return session
}

// 官方 QA（2hv4yt1j8）：『雷维翁超越者·尤里乌斯』被对手随从的【入场曲】破坏时，
// 它那条"对手的随从进入战场时"的触发已经入队，但结算时它已经离开战场，所以不会发动。
func TestQueuedTriggerIsDroppedWhenSourceLeavesField(t *testing.T) {
	pack, invader, listener := fieldLeaveFixture(77882001, 77882002, true)
	session := playInvader(t, pack, invader, listener)
	if session.g.own.leaderLife != 20 {
		t.Fatalf("destroyed listener still resolved its queued trigger: leader life %d", session.g.own.leaderLife)
	}
	if session.g.instances[listener.InstanceID].zone != "graveyard" {
		t.Fatalf("listener was not destroyed: %s", session.g.instances[listener.InstanceID].zone)
	}
}

// 反例：监听者活下来时，同一条触发照常结算。
func TestQueuedTriggerResolvesWhileSourceStaysOnField(t *testing.T) {
	pack, invader, listener := fieldLeaveFixture(77882003, 77882004, false)
	session := playInvader(t, pack, invader, listener)
	if session.g.own.leaderLife != 17 {
		t.Fatalf("surviving listener did not resolve: leader life %d", session.g.own.leaderLife)
	}
}

// 官方 QA（mxtwkh_pd）：失去能力不影响已经入队的触发，处理途中的能力会继续完成结算。
func TestRemovingAbilitiesKeepsQueuedTriggers(t *testing.T) {
	const listener, remover = 77882005, 77882006
	pack, _, _ := fieldLeaveFixture(listener, remover, false)
	pack.Cards[1].Abilities = []ir.Ability{{
		ID: strings.Repeat("c", 32), Trigger: ir.SimpleTrigger{Kind: "fanfare"},
		Body: []ir.Effect{
			ir.SelectionEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("d", 32)}, Kind: "choose", Binding: "victim",
				Source: ir.ZoneRef{Kind: "zone", Side: "oppo", Zone: "field", Member: "follower"}},
			ir.TargetEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("e", 32)}, Kind: "remove_ability", Keyword: "all", Target: ir.BindingRef{Kind: "binding", Name: "victim"}},
		},
	}}
	state := testState()
	state.Turn.Active = "own"
	state.Players["own"] = withInstance(state.Players["own"], "hand", ir.TestInstance{InstanceID: strings.Repeat("1", 32), CardID: remover, DeclaredType: "follower"})
	state.Players["oppo"] = withInstance(state.Players["oppo"], "field", ir.TestInstance{InstanceID: strings.Repeat("2", 32), CardID: listener, DeclaredType: "follower"})
	session, err := NewSession(pack, state, 3)
	if err != nil {
		t.Fatal(err)
	}
	result := session.SubmitAs(strings.Repeat("9", 32), "own", SimulatorCommand{Kind: "play", Source: strings.Repeat("1", 32)})
	if result.Status != StatusSuspended {
		t.Fatalf("expected the choose request, got %#v", result)
	}
	choice := result.Choice
	response := ChoiceResponse{RequestID: choice.RequestID, ActionID: choice.ActionID, StateRevision: choice.StateRevision}
	for _, candidate := range choice.Candidates {
		response.SelectedInstanceIDs = append(response.SelectedInstanceIDs, candidate.InstanceID)
	}
	if step := session.Resume(response); step.Status != StatusCompleted {
		t.Fatalf("resume failed: %#v", step)
	}
	if session.g.own.leaderLife != 17 {
		t.Fatalf("queued trigger was dropped with the abilities: leader life %d", session.g.own.leaderLife)
	}
}
