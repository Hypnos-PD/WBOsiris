package runner

import (
	"reflect"
	"strings"
	"testing"

	"wbo/internal/ir"
	"wbo/internal/ruleset"
)

func TestInstructionBudgetFaultIsDeterministicAndTerminal(t *testing.T) {
	sourceID := strings.Repeat("1", 32)
	leader := ir.LeaderRef{Kind: "leader", Side: "own", ValueType: "leader"}
	pack := &ir.CardPack{Cards: []ir.Card{{
		ID: 12345678, CardType: "spell", PlayEffects: []ir.Effect{
			ir.TargetEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("2", 32)}, Kind: "heal", Target: leader, Amount: 1},
			ir.TargetEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("3", 32)}, Kind: "heal", Target: leader, Amount: 10},
		},
	}}}
	state := testState()
	own := state.Players["own"]
	own.Leader = ir.Leader{Life: 5, MaxLife: 20}
	state.Players["own"] = withInstance(own, "hand", ir.TestInstance{InstanceID: sourceID, Alias: "source", CardID: 12345678, DeclaredType: "spell"})

	run := func() (*Session, StepResult) {
		session, err := NewSession(pack, state, 1)
		if err != nil {
			t.Fatal(err)
		}
		session.budgetPolicy.Instructions = 1
		step := session.Begin(strings.Repeat("a", 32), ir.SourceAction{Kind: "play", Actor: "own", Source: sourceID})
		return session, step
	}
	first, firstStep := run()
	second, secondStep := run()
	if firstStep.Status != StatusFault || firstStep.ErrorCode != executionBudgetExceeded || first.g.own.leaderLife != 6 {
		t.Fatalf("unexpected budget result: step=%#v life=%d", firstStep, first.g.own.leaderLife)
	}
	if secondStep != firstStep || !reflect.DeepEqual(first.g.snapshot(), second.g.snapshot()) || first.BudgetState() != second.BudgetState() {
		t.Fatal("identical budget exhaustion diverged")
	}
	if retry := first.Begin(strings.Repeat("b", 32), ir.SourceAction{Kind: "play", Actor: "own", Source: sourceID}); retry.Status != StatusFault || retry.ErrorCode != executionBudgetExceeded {
		t.Fatalf("faulted session accepted another command: %#v", retry)
	}
}

func TestBudgetTrackerRejectsEveryLimit(t *testing.T) {
	policy := ruleset.ExecutionBudget{Instructions: 1, QueryVisits: 1, StackDepth: 1, Candidates: 1, Events: 1, Triggers: 1, CreatedInstances: 1, ContinuationBytes: 1}
	tests := []struct {
		name string
		use  func(*budgetTracker) bool
	}{
		{"instructions", func(b *budgetTracker) bool { return b.chargeInstructions(2) }},
		{"query visits", func(b *budgetTracker) bool { return b.chargeQueryVisits(2) }},
		{"stack depth", func(b *budgetTracker) bool { return b.observeStack(2) }},
		{"candidates", func(b *budgetTracker) bool { return b.chargeCandidates(2) }},
		{"events", func(b *budgetTracker) bool { return b.chargeEvents(2) }},
		{"triggers", func(b *budgetTracker) bool { return b.chargeTriggers(2) }},
		{"created instances", func(b *budgetTracker) bool { return b.chargeCreatedInstances(2) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var budget budgetTracker
			budget.reset(policy)
			if test.use(&budget) || !budget.exceeded {
				t.Fatal("over-limit charge was accepted")
			}
		})
	}
}

func TestQueryBudgetFaultDoesNotPublishPartialCandidates(t *testing.T) {
	sourceID, firstTargetID, secondTargetID := strings.Repeat("4", 32), strings.Repeat("5", 32), strings.Repeat("6", 32)
	pack := &ir.CardPack{Cards: []ir.Card{
		{ID: 23456789, CardType: "spell", PlayEffects: []ir.Effect{
			ir.SelectionEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("7", 32)}, Kind: "require", Policy: "required", Binding: "target", Source: ir.ZoneRef{Kind: "zone", Side: "oppo", Zone: "field", Member: "follower"}},
		}},
		{ID: 34567890, CardType: "follower", Stats: &ir.Stats{Attack: 1, Life: 1}},
	}}
	state := testState()
	state.Players["own"] = withInstance(state.Players["own"], "hand", ir.TestInstance{InstanceID: sourceID, Alias: "source", CardID: 23456789, DeclaredType: "spell"})
	oppo := state.Players["oppo"]
	oppo = withInstance(oppo, "field", ir.TestInstance{InstanceID: firstTargetID, Alias: "first", CardID: 34567890, DeclaredType: "follower"})
	oppo = withInstance(oppo, "field", ir.TestInstance{InstanceID: secondTargetID, Alias: "second", CardID: 34567890, DeclaredType: "follower"})
	state.Players["oppo"] = oppo

	session, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	session.budgetPolicy.QueryVisits = 1
	step := session.Begin(strings.Repeat("b", 32), ir.SourceAction{Kind: "play", Actor: "own", Source: sourceID})
	if step.Status != StatusFault || step.Choice != nil || session.BudgetState().QueryVisits != 1 || session.BudgetState().Candidates != 0 {
		t.Fatalf("query budget exposed a partial request: step=%#v budget=%#v", step, session.BudgetState())
	}
	if session.g.instances[firstTargetID].zone != "field" || session.g.instances[secondTargetID].zone != "field" {
		t.Fatal("query budget fault mutated candidates")
	}
}

func TestCreatedInstanceBudgetStopsBeforeOverLimitCreation(t *testing.T) {
	sourceID := strings.Repeat("8", 32)
	pack := &ir.CardPack{Cards: []ir.Card{
		{ID: 45678901, CardType: "spell", PlayEffects: []ir.Effect{
			ir.CardEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("9", 32)}, Kind: "add_card", Owner: "own", CardID: 56789012, Count: 2, Destination: "hand"},
		}},
		{ID: 56789012, CardType: "spell"},
	}}
	state := testState()
	state.Players["own"] = withInstance(state.Players["own"], "hand", ir.TestInstance{InstanceID: sourceID, Alias: "source", CardID: 45678901, DeclaredType: "spell"})
	session, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	session.budgetPolicy.CreatedInstances = 1
	step := session.Begin(strings.Repeat("c", 32), ir.SourceAction{Kind: "play", Actor: "own", Source: sourceID})
	if step.Status != StatusFault || session.g.serial != 1 || len(session.g.own.hand) != 1 || session.g.own.hand[0].id != "added-1" {
		t.Fatalf("instance budget committed an over-limit creation: step=%#v serial=%d hand=%#v", step, session.g.serial, session.g.own.hand)
	}
}
