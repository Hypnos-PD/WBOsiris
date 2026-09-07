package runner

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"wbo/internal/ir"
	"wbo/internal/ruleset"
)

func filteredDrawFixture(t *testing.T, side string, seed uint64) (*Session, []string) {
	t.Helper()
	pack := &ir.CardPack{Cards: []ir.Card{
		{ID: 77773001, CardType: "follower", Stats: &ir.Stats{Attack: 1, Life: 4}},
		{ID: 77773002, CardType: "follower", Stats: &ir.Stats{Attack: 2, Life: 4}},
		{ID: 77773003, CardType: "follower", Stats: &ir.Stats{Attack: 3, Life: 4}},
		{ID: 77773004, CardType: "spell"},
	}}
	state := testState()
	state.Turn.Active, state.FirstPlayer = side, "own"
	p := state.Players[side]
	var eligible []string
	for n, cardID := range []int{77773004, 77773001, 77773001, 77773004, 77773001, 77773002, 77773002, 77773003, 77773004} {
		id := fmt.Sprintf("%032x", n+1)
		kind := "follower"
		if cardID == 77773004 {
			kind = "spell"
		} else {
			eligible = append(eligible, id)
		}
		p = withInstance(p, "deck", ir.TestInstance{InstanceID: id, CardID: cardID, DeclaredType: kind})
	}
	state.Players[side] = p
	s, err := NewSession(pack, state, seed)
	if err != nil {
		t.Fatal(err)
	}
	return s, eligible
}

func TestFilteredDrawWeightsEveryCopyAndPreservesRemainingOrder(t *testing.T) {
	for _, side := range []string{"own", "oppo"} {
		counts := map[string]int{}
		for seed := uint64(1); seed <= 600; seed++ {
			s, eligible := filteredDrawFixture(t, side, seed)
			p := s.g.player(side)
			before := append([]*instance(nil), p.deck...)
			f := frame{}
			s.g.draw(ir.DrawEffect{Kind: "draw", Owner: side, Count: 1, Output: "drawn", Predicate: ir.FieldPredicate{Kind: "compare", Field: "life", Op: "eq", Value: 4}}, nil, f)
			if len(f["drawn"]) != 1 || len(p.hand) != 1 || s.g.rng.Consumed() != 1 {
				t.Fatal("wrong filtered draw result")
			}
			id := p.hand[0].id
			counts[id]++
			want := eligible[ruleset.NewRNG(seed).Index(len(eligible))]
			if id != want {
				t.Fatal("draw did not sample individual eligible instances")
			}
			var kept []*instance
			for _, card := range before {
				if card.id != id {
					kept = append(kept, card)
				}
			}
			if !reflect.DeepEqual(p.deck, kept) {
				t.Fatal("filtered draw shuffled remaining deck")
			}
			events := s.EventsFor(oppositeSide(side))
			if len(events) != 1 || events[0].Kind != "card_drawn" || events[0].Count != 1 || events[0].Subject != nil || events[0].CardID != 0 || events[0].InstanceID != "" {
				t.Fatal("missing or private draw event", events)
			}
		}
		if len(counts) != 6 {
			t.Fatal("some eligible copies could never be drawn", counts)
		}
		for id, count := range counts {
			if count < 65 || count > 140 {
				t.Fatal("individual copy frequency is biased", id, count)
			}
		}
	}
}

func TestFilteredDrawModesAndOverflow(t *testing.T) {
	for _, tc := range []struct {
		name      string
		count     int
		all       bool
		predicate ir.Predicate
		want, rng int
	}{
		{"top", 2, false, nil, 2, 0},
		{"filtered", 3, false, ir.FieldPredicate{Kind: "has_type", CardType: "follower"}, 3, 3},
		{"shortage", 9, false, ir.FieldPredicate{Kind: "has_type", CardType: "follower"}, 6, 6},
		{"all", 0, true, ir.FieldPredicate{Kind: "has_type", CardType: "follower"}, 6, 0},
		{"missing", 1, false, ir.FieldPredicate{Kind: "has_type", CardType: "amulet"}, 0, 0},
		{"zero", 0, false, ir.FieldPredicate{Kind: "has_type", CardType: "follower"}, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, eligible := filteredDrawFixture(t, "own", 7)
			f := frame{"drawn": {s.g.own.deck[0]}}
			s.g.draw(ir.DrawEffect{Kind: "draw", Owner: "own", Count: tc.count, All: tc.all, Output: "drawn", Predicate: tc.predicate}, nil, f)
			if len(f["drawn"]) != tc.want || len(s.g.own.hand) != tc.want || int(s.g.rng.Consumed()) != tc.rng || s.g.gameOver {
				t.Fatal("wrong draw count, RNG or deck-out")
			}
			seen := map[string]bool{}
			for n, card := range f["drawn"] {
				if seen[card.id] {
					t.Fatal("drew an instance twice")
				}
				seen[card.id] = true
				if tc.all && card.id != eligible[n] {
					t.Fatal("draw all lost stable order")
				}
			}
			if tc.name == "top" && (f["drawn"][0].id != fmt.Sprintf("%032x", 1) || f["drawn"][1].id != fmt.Sprintf("%032x", 2)) {
				t.Fatal("ordinary draw stopped using deck top")
			}
		})
	}
	s, _ := filteredDrawFixture(t, "own", 9)
	for n := 0; n < 8; n++ {
		i := s.g.newInstance(s.g.cards[77773004], fmt.Sprintf("%032x", n+20), "", "hand")
		s.g.own.hand = append(s.g.own.hand, i)
	}
	f := frame{}
	s.g.draw(ir.DrawEffect{Kind: "draw", Owner: "own", Count: 3, Output: "drawn", Predicate: ir.FieldPredicate{Kind: "has_type", CardType: "follower"}}, nil, f)
	if len(f["drawn"]) != 1 || len(s.g.own.hand) != 9 || len(s.g.own.graveyard) != 2 || s.g.own.shadows != 2 || len(s.g.own.destroyed) != 0 || s.g.events[0].Count != 3 {
		t.Fatal("overflow polluted drawn binding or destruction history")
	}
}

func TestFilteredDrawBudgetFailureDoesNotConsumeRandomness(t *testing.T) {
	for _, exhausted := range []string{"queries", "candidates", "events"} {
		s, _ := filteredDrawFixture(t, "own", 1)
		before := s.g.snapshot()
		policy := ruleset.Default().ExecutionBudget
		switch exhausted {
		case "queries":
			policy.QueryVisits = 1
		case "candidates":
			policy.Candidates = 1
		case "events":
			policy.Events = 0
		}
		budget := &budgetTracker{}
		budget.reset(policy)
		s.g.budget = budget
		s.g.draw(ir.DrawEffect{Kind: "draw", Owner: "own", Count: 1, Output: "drawn", Predicate: ir.FieldPredicate{Kind: "has_type", CardType: "follower"}}, nil, frame{})
		if !budget.exceeded || s.g.rng.Consumed() != 0 || !reflect.DeepEqual(before, s.g.snapshot()) {
			t.Fatal("budget failure drew a card or consumed RNG", exhausted)
		}
	}
}

func TestFilteredDrawContinuationPreservesNextRandomSelection(t *testing.T) {
	for _, side := range []string{"own", "oppo"} {
		pack := repeatCardPack(t)
		draw := ir.DrawEffect{Kind: "draw", Owner: "own", SourceZone: "deck", Count: 1, Output: "drawn", Predicate: ir.FieldPredicate{Kind: "has_type", CardType: "follower"}}
		pack.Cards = append(pack.Cards, ir.Card{ID: 77773005, CardType: "spell", PlayEffects: []ir.Effect{draw,
			ir.ModeEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("f", 32)}, Kind: "mode", Options: []ir.ModeOption{{ID: 1}, {ID: 2}}}, draw}})
		state := testState()
		state.Turn.Active = side
		id := strings.Repeat("a", 32)
		p := withInstance(state.Players[side], "hand", ir.TestInstance{InstanceID: id, CardID: 77773005, DeclaredType: "spell"})
		for n := 1; n <= 5; n++ {
			p = withInstance(p, "deck", ir.TestInstance{InstanceID: fmt.Sprintf("%032x", n), CardID: 10001110, DeclaredType: "follower"})
		}
		state.Players[side] = p
		s, err := NewSession(pack, state, 77)
		if err != nil {
			t.Fatal(err)
		}
		before := s.g.snapshot()
		s.LegalActionsFor(side)
		if !reflect.DeepEqual(before, s.g.snapshot()) {
			t.Fatal("preflight drew cards")
		}
		step := s.SubmitAs(strings.Repeat("b", 32), side, SimulatorCommand{Kind: "play", Source: id})
		if step.Status != StatusSuspended || s.g.rng.Consumed() != 1 {
			t.Fatal(step)
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
		c.Version = "0.15.0"
		if _, err := RestoreSession(pack, c); err == nil {
			t.Fatal("accepted old filtered-draw semantics")
		}
		response := ChoiceResponse{RequestID: step.Choice.RequestID, ActionID: step.Choice.ActionID, StateRevision: step.Choice.StateRevision, SelectedOptionID: 1}
		for _, current := range []*Session{s, restored} {
			if r := current.Resume(response); r.Status != StatusCompleted || current.g.rng.Consumed() != 2 || len(current.g.player(side).hand) != 2 {
				t.Fatal(r)
			}
			view, _ := current.View(oppositeSide(side))
			if len(view.Oppo.Hand) != 0 {
				t.Fatal("filtered hand leaked")
			}
		}
		if !reflect.DeepEqual(s.g.snapshot(), restored.g.snapshot()) {
			t.Fatal("restored random draw diverged")
		}
	}
}
