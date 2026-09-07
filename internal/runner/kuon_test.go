package runner

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"wbo/internal/ir"
)

func TestKuonOfficialNobleShikigamiExamples(t *testing.T) {
	pack := repeatCardPack(t)
	for _, side := range []string{"own", "oppo"} {
		for _, tc := range []struct{ card, attack, life, want int }{
			{0, 0, 0, 10}, {90031140, 20, 1, 13}, {90034120, 10, 2, 11},
		} {
			t.Run(fmt.Sprintf("%s/%d", side, tc.want), func(t *testing.T) {
				state := testState()
				state.Turn.Active, state.Turn.Number = side, 10
				p := state.Players[side]
				p.PP, p.MaxPP = 10, 10
				source := strings.Repeat("a", 32)
				p = withInstance(p, "hand", ir.TestInstance{InstanceID: source, CardID: 10134110, DeclaredType: "follower"})
				if tc.card != 0 {
					p = withInstance(p, "field", ir.TestInstance{InstanceID: strings.Repeat("b", 32), CardID: tc.card, DeclaredType: "follower", Overrides: ir.InstanceOverrides{Stats: &ir.Stats{Attack: tc.attack, Life: tc.life}}})
				}
				state.Players[side] = p
				s, err := NewSession(pack, state, 1)
				if err != nil {
					t.Fatal(err)
				}
				step := s.SubmitAs(strings.Repeat("c", 32), side, SimulatorCommand{Kind: "play", Source: source})
				if step.Status != StatusCompleted {
					t.Fatal(step)
				}
				field := s.g.player(side).field
				if len(field) != 2 || field[0].card.ID != 10134110 || field[1].card.ID != 90034120 || field[1].attack != tc.want || field[1].life != tc.want {
					t.Fatal("Noble did not match the official base-stat total", field)
				}
				if !field[1].abilities["ward"] || !field[1].abilities["aura"] || s.g.rng.Consumed() != 0 {
					t.Fatal("wrong keywords or random consumption")
				}
			})
		}
	}
}

func TestNobleCountsOnlyItsControllerDeathsDuringTheCurrentTurn(t *testing.T) {
	pack := repeatCardPack(t)
	state := testState()
	state.Players["own"] = withInstance(state.Players["own"], "field", ir.TestInstance{InstanceID: strings.Repeat("1", 32), CardID: 90034110, DeclaredType: "follower"})
	state.Players["oppo"] = withInstance(state.Players["oppo"], "field", ir.TestInstance{InstanceID: strings.Repeat("2", 32), CardID: 90031140, DeclaredType: "follower"})
	s, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	s.g.destroyByEffect([]*instance{s.g.instances[strings.Repeat("1", 32)]})
	if r := s.Submit(strings.Repeat("3", 32), SimulatorCommand{Kind: "end_turn"}); r.Status != StatusCompleted {
		t.Fatal(r)
	}
	s.g.destroyByEffect([]*instance{s.g.instances[strings.Repeat("2", 32)]})
	for _, side := range []string{"own", "oppo"} {
		expr := &ir.SumExpr{Kind: "sum", Field: "base_attack", Source: ir.HistoryRef{Kind: "history", Side: side, Window: "this_turn", Member: "follower"}}
		want := 0
		if side == "oppo" {
			want = 3
		}
		if got := s.g.numericValue(expr, nil, nil); got != want {
			t.Fatal(side, got, want)
		}
	}
	// A new allied death during the opponent's turn belongs to that same turn.
	ally := s.g.summonFor(nil, "own", 1, 90031130, false)[0]
	s.g.destroyByEffect([]*instance{ally})
	if got := s.g.numericValue(&ir.SumExpr{Kind: "sum", Field: "base_life", Source: ir.HistoryRef{Kind: "history", Side: "own", Window: "this_turn", Member: "follower"}}, nil, nil); got != 1 {
		t.Fatal(got)
	}
}

func TestNobleEntryTriggerAndHistorySumResumeTogether(t *testing.T) {
	pack := repeatCardPack(t)
	for n := range pack.Cards {
		if pack.Cards[n].ID == 90034120 {
			a := &pack.Cards[n].Abilities[0]
			a.Body = append([]ir.Effect{ir.ModeEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("e", 32)}, Kind: "mode", Options: []ir.ModeOption{{ID: 1, Body: []ir.Effect{}}}}}, a.Body...)
		}
	}
	state := testState()
	state.Turn.Number = 10
	p := state.Players["own"]
	p.PP, p.MaxPP = 10, 10
	source := strings.Repeat("a", 32)
	state.Players["own"] = withInstance(p, "hand", ir.TestInstance{InstanceID: source, CardID: 10134110, DeclaredType: "follower"})
	s, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	step := s.Submit(strings.Repeat("b", 32), SimulatorCommand{Kind: "play", Source: source})
	if step.Status != StatusSuspended {
		t.Fatal(step)
	}
	data, err := s.EncodeContinuation()
	if err != nil {
		t.Fatal(err)
	}
	saved, err := DecodeContinuation(data)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := RestoreSession(pack, saved)
	if err != nil {
		t.Fatal(err)
	}
	for _, current := range []*Session{s, restored} {
		if r := current.Resume(ChoiceResponse{RequestID: step.Choice.RequestID, ActionID: step.Choice.ActionID, StateRevision: step.Choice.StateRevision, SelectedOptionID: 1}); r.Status != StatusCompleted {
			t.Fatal(r)
		}
		noble := current.g.own.field[1]
		if noble.attack != 10 || noble.life != 10 {
			t.Fatal("history sum lost across pause", noble)
		}
		current.g.summonFor(nil, "own", 1, 90031130, false)
		if len(current.g.triggers) != 0 {
			t.Fatal("another follower retriggered Noble")
		}
		kuon := current.g.own.field[0]
		current.g.execCardEffect(ir.CardEffect{Kind: "transform", Target: ir.SelfRef{Kind: "self"}, CardID: 90034120}, kuon, nil)
		if kuon.attack != 1 || kuon.life != 1 || len(current.g.triggers) != 0 {
			t.Fatal("transformation incorrectly triggered Noble entry")
		}
	}
	if !reflect.DeepEqual(s.g.snapshot(), restored.g.snapshot()) {
		t.Fatal("restored entry trigger diverged")
	}
}

func TestHistorySumBudgetDoesNotApplyPartialBuff(t *testing.T) {
	state := testState()
	state.Players["own"] = withInstance(state.Players["own"], "field", ir.TestInstance{InstanceID: strings.Repeat("1", 32), CardID: 90031140, DeclaredType: "follower"})
	s, err := NewSession(repeatCardPack(t), state, 1)
	if err != nil {
		t.Fatal(err)
	}
	s.g.destroyByEffect([]*instance{s.g.instances[strings.Repeat("1", 32)]})
	noble := s.g.summonFor(nil, "own", 1, 90034120, false)[0]
	policy := s.budgetPolicy
	policy.QueryVisits = 1
	s.g.budget = &budgetTracker{policy: policy}
	expr := &ir.SumExpr{Kind: "sum", Field: "base_attack", Source: ir.HistoryRef{Kind: "history", Side: "own", Window: "this_turn"}}
	s.g.execTargetEffect(ir.TargetEffect{Kind: "buff_stats", Target: ir.SelfRef{Kind: "self"}, AttackExpr: expr, LifeExpr: expr}, noble, nil)
	if !s.g.budget.exceeded || noble.attack != 1 || noble.life != 1 {
		t.Fatal("partial aggregate changed stats")
	}
}

func TestNobleHistoryWindowDuringPausedTurnStart(t *testing.T) {
	pack := repeatCardPack(t)
	pack.Cards = append(pack.Cards, ir.Card{ID: 88880007, CardType: "amulet", Abilities: []ir.Ability{{
		ID: strings.Repeat("a", 32), Trigger: ir.SimpleTrigger{Kind: "lastwords"}, Body: []ir.Effect{
			ir.ModeEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("b", 32)}, Kind: "mode", Options: []ir.ModeOption{{ID: 1, Body: []ir.Effect{}}}},
			ir.TargetEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("c", 32)}, Kind: "destroy", Target: ir.ZoneRef{Kind: "zone", Side: "own", Zone: "field", Member: "follower"}},
			ir.CardEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("d", 32)}, Kind: "summon", Owner: "own", CardID: 90034120, Count: 1, Output: "summoned"},
		},
	}}})
	state := testState()
	one := 1
	p := withInstance(state.Players["oppo"], "field", ir.TestInstance{InstanceID: strings.Repeat("1", 32), CardID: 88880007, DeclaredType: "amulet", Overrides: ir.InstanceOverrides{Countdown: &one}})
	p = withInstance(p, "field", ir.TestInstance{InstanceID: strings.Repeat("2", 32), CardID: 90031140, DeclaredType: "follower"})
	state.Players["oppo"] = withInstance(p, "field", ir.TestInstance{InstanceID: strings.Repeat("3", 32), CardID: 90031130, DeclaredType: "follower"})
	s, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	s.g.destroyByEffect([]*instance{s.g.instances[strings.Repeat("3", 32)]})
	step := s.Submit(strings.Repeat("e", 32), SimulatorCommand{Kind: "end_turn"})
	if step.Status != StatusSuspended || s.g.turnTransition != "starting_triggers" {
		t.Fatal(step, s.g.turnTransition)
	}
	data, err := s.EncodeContinuation()
	if err != nil {
		t.Fatal(err)
	}
	saved, err := DecodeContinuation(data)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := RestoreSession(pack, saved)
	if err != nil {
		t.Fatal(err)
	}
	for _, current := range []*Session{s, restored} {
		if r := current.Resume(ChoiceResponse{RequestID: step.Choice.RequestID, ActionID: step.Choice.ActionID, StateRevision: step.Choice.StateRevision, SelectedOptionID: 1}); r.Status != StatusCompleted {
			t.Fatal(r)
		}
		field := current.g.oppo.field
		if len(field) != 1 || field[0].card.ID != 90034120 || field[0].attack != 4 || field[0].life != 4 {
			t.Fatal("turn-start history included previous-turn Paper", field)
		}
	}
	if !reflect.DeepEqual(s.g.snapshot(), restored.g.snapshot()) {
		t.Fatal("turn-start aggregation changed after restore")
	}
}
