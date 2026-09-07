package runner

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"wbo/internal/ir"
	"wbo/internal/ruleset"
)

func TestExtremumUsesCurrentFieldsAndKeepsAllTies(t *testing.T) {
	follower := &ir.Card{ID: 77774001, CardType: "follower"}
	amulet := &ir.Card{ID: 77774002, CardType: "amulet"}
	a := &instance{id: "a", card: follower, attack: 5, life: 1, cost: 3}
	b := &instance{id: "b", card: follower, attack: 5, life: 2, cost: 1}
	c := &instance{id: "c", card: follower, attack: 2, life: 1, cost: 2}
	d := &instance{id: "d", card: amulet, cost: 0}
	g := &game{}
	items := []*instance{a, d, b, c}
	for _, tc := range []struct {
		direction, field string
		want             []*instance
	}{
		{"highest", "attack", []*instance{a, b}}, {"lowest", "attack", []*instance{c}},
		{"highest", "life", []*instance{b}}, {"lowest", "life", []*instance{a, c}},
		{"highest", "cost", []*instance{a}}, {"lowest", "cost", []*instance{d}},
	} {
		got := g.extremumCandidates(items, &ir.SelectionExtremum{Direction: tc.direction, Field: tc.field})
		if !reflect.DeepEqual(got, tc.want) {
			t.Fatal(tc.direction, tc.field, got)
		}
	}
	a.attack = 1
	if got := g.extremumCandidates(items, &ir.SelectionExtremum{Direction: "highest", Field: "attack"}); len(got) != 1 || got[0] != b {
		t.Fatal("used a stale attack value")
	}
	policy := ruleset.Default().ExecutionBudget
	policy.QueryVisits = 1
	g.budget = &budgetTracker{}
	g.budget.reset(policy)
	if got := g.extremumCandidates(items, &ir.SelectionExtremum{Direction: "highest", Field: "attack"}); len(got) != 0 || !g.budget.exceeded {
		t.Fatal("returned a partial extremum after budget failure")
	}
}

func TestExtremumChoiceRestoresAndRejectsLowerCandidate(t *testing.T) {
	pack := repeatCardPack(t)
	pack.Cards = append(pack.Cards, ir.Card{ID: 77774003, CardType: "spell", PlayEffects: []ir.Effect{
		ir.SelectionEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("f", 32)}, Kind: "choose", Policy: "optional", Binding: "target", Source: ir.ZoneRef{Kind: "zone", Side: "oppo", Zone: "field", Member: "follower"}, Extremum: &ir.SelectionExtremum{Direction: "highest", Field: "attack"}},
		ir.TargetEffect{Kind: "destroy", Target: ir.BindingRef{Kind: "binding", Name: "target"}},
	}})
	for _, side := range []string{"own", "oppo"} {
		state := testState()
		state.Turn.Active = side
		source := strings.Repeat("a", 32)
		state.Players[side] = withInstance(state.Players[side], "hand", ir.TestInstance{InstanceID: source, CardID: 77774003, DeclaredType: "spell"})
		foe := state.Players[oppositeSide(side)]
		for n := 1; n <= 4; n++ {
			foe = withInstance(foe, "field", ir.TestInstance{InstanceID: fmt.Sprintf("%032x", n), CardID: 10001110, DeclaredType: "follower"})
		}
		state.Players[oppositeSide(side)] = foe
		s, err := NewSession(pack, state, 1)
		if err != nil {
			t.Fatal(err)
		}
		id := func(n int) string { return fmt.Sprintf("%032x", n) }
		for n, attack := range []int{5, 5, 2, 10} {
			s.g.instances[id(n+1)].attack = attack
		}
		s.g.instances[id(4)].abilities["stealth"] = true
		before := s.g.snapshot()
		s.LegalActionsFor(side)
		if !reflect.DeepEqual(before, s.g.snapshot()) {
			t.Fatal("preflight changed extrema")
		}
		step := s.SubmitAs(strings.Repeat("b", 32), side, SimulatorCommand{Kind: "play", Source: source})
		if step.Status != StatusSuspended || len(step.Choice.Candidates) != 2 || step.Choice.Candidates[0].InstanceID != id(1) || step.Choice.Candidates[1].InstanceID != id(2) {
			t.Fatal("choice did not rank legal candidates", step)
		}
		data, err := s.EncodeContinuation()
		if err != nil {
			t.Fatal(err)
		}
		c, err := DecodeContinuation(data)
		if err != nil {
			t.Fatal(err)
		}
		restored, err := RestoreSession(pack, c)
		if err != nil {
			t.Fatal(err)
		}
		response := ChoiceResponse{RequestID: step.Choice.RequestID, ActionID: step.Choice.ActionID, StateRevision: step.Choice.StateRevision, SelectedInstanceIDs: []string{id(3)}}
		if r := restored.Resume(response); r.Status != StatusRejected {
			t.Fatal("accepted lower candidate", r)
		}
		response.SelectedInstanceIDs = []string{id(2)}
		for _, current := range []*Session{s, restored} {
			if r := current.Resume(response); r.Status != StatusCompleted {
				t.Fatal(r)
			}
			if current.g.instances[id(2)].zone != "graveyard" || current.g.rng.Consumed() != 0 {
				t.Fatal("choice resolution mismatch")
			}
		}
		if !reflect.DeepEqual(s.g.snapshot(), restored.g.snapshot()) {
			t.Fatal("restored selection diverged")
		}
	}
}

func TestDivineThunderRandomTies(t *testing.T) {
	pack := repeatCardPack(t)
	seen := map[string]bool{}
	for seed := uint64(1); seed <= 20; seed++ {
		state := testState()
		p := state.Players["own"]
		p.PP, p.MaxPP = 4, 4
		source := strings.Repeat("a", 32)
		state.Players["own"] = withInstance(p, "hand", ir.TestInstance{InstanceID: source, CardID: 10103310, DeclaredType: "spell"})
		foe := state.Players["oppo"]
		for n := 1; n <= 3; n++ {
			foe = withInstance(foe, "field", ir.TestInstance{InstanceID: fmt.Sprintf("%032x", n), CardID: 10001110, DeclaredType: "follower"})
		}
		state.Players["oppo"] = foe
		s, err := NewSession(pack, state, seed)
		if err != nil {
			t.Fatal(err)
		}
		s.g.oppo.field[0].attack = 5
		s.g.oppo.field[1].attack = 5
		if r := s.Submit(strings.Repeat("b", 32), SimulatorCommand{Kind: "play", Source: source}); r.Status != StatusCompleted {
			t.Fatal(r)
		}
		if len(s.g.oppo.graveyard) != 1 || s.g.oppo.graveyard[0].attack != 5 || s.g.rng.Consumed() != 1 {
			t.Fatal("incorrect random extremum")
		}
		seen[s.g.oppo.graveyard[0].id] = true
	}
	if len(seen) != 2 {
		t.Fatal("tie selection never reached both candidates")
	}
}
