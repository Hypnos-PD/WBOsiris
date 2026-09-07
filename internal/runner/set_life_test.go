package runner

import (
	"reflect"
	"strings"
	"testing"

	"wbo/internal/ir"
)

func TestSetLifeIsNotDamageAndResetsOnReturn(t *testing.T) {
	pack := repeatCardPack(t)
	state := testState()
	a, b, c := strings.Repeat("1", 32), strings.Repeat("2", 32), strings.Repeat("3", 32)
	p := withInstance(state.Players["own"], "field", ir.TestInstance{InstanceID: a, CardID: 10001110, DeclaredType: "follower"})
	p = withInstance(p, "field", ir.TestInstance{InstanceID: b, CardID: 10001110, DeclaredType: "follower"})
	state.Players["own"] = withInstance(p, "field", ir.TestInstance{InstanceID: c, CardID: 10001210, DeclaredType: "amulet"})
	s, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	i := s.g.instances[a]
	i.superEvolved = true
	i.abilities["barrier"] = true
	i.damageReduction = 9
	e := ir.TargetEffect{Kind: "set_life", Target: ir.BindingRef{Kind: "binding", Name: "targets"}, Amount: 1}
	f := frame{"targets": {i, s.g.instances[c], nil}}
	s.g.execTargetEffect(e, i, f)
	if i.life != 1 || !i.abilities["barrier"] || len(s.g.events) != 0 || s.g.instances[c].life != 0 {
		t.Fatal("set life was treated as damage or affected amulet")
	}
	e.Amount = 7
	s.g.execTargetEffect(e, i, f)
	if i.life != 7 || len(s.g.events) != 0 {
		t.Fatal("set life was treated as healing")
	}
	s.g.returnCard(i, "hand")
	if i.life != 2 || i.superEvolved {
		t.Fatal("return did not reset set life")
	}
	e.Amount = 0
	f["targets"] = []*instance{s.g.instances[b]}
	s.g.execTargetEffect(e, i, f)
	if s.g.instances[b].zone != "graveyard" || len(s.g.events) != 1 || s.g.events[0].Kind != "destroyed" || s.g.own.shadows != 1 {
		t.Fatal("zero life did not cause destruction", s.g.events)
	}
}

func TestLilyFanfareAndEvolutionContinuationBothPlayers(t *testing.T) {
	pack := repeatCardPack(t)
	for _, side := range []string{"own", "oppo"} {
		state := testState()
		state.Turn.Active, state.Turn.Number = side, 5
		id, target, drawn := strings.Repeat("1", 32), strings.Repeat("2", 32), strings.Repeat("3", 32)
		p := state.Players[side]
		p.PP, p.MaxPP, p.Combo, p.EP = 2, 2, 2, 1
		p = withInstance(p, "hand", ir.TestInstance{InstanceID: id, CardID: 10113110, DeclaredType: "follower"})
		state.Players[side] = withInstance(p, "deck", ir.TestInstance{InstanceID: drawn, CardID: 10001110, DeclaredType: "follower"})
		state.Players[oppositeSide(side)] = withInstance(state.Players[oppositeSide(side)], "field", ir.TestInstance{InstanceID: target, CardID: 10001110, DeclaredType: "follower", Overrides: ir.InstanceOverrides{Stats: &ir.Stats{Attack: 5, Life: 7}, Keywords: []string{"barrier"}}})
		s, err := NewSession(pack, state, 1)
		if err != nil {
			t.Fatal(err)
		}
		before := s.g.snapshot()
		s.LegalActionsFor(side)
		if !reflect.DeepEqual(before, s.g.snapshot()) {
			t.Fatal("preflight set target life")
		}
		step := s.SubmitAs(strings.Repeat("a", 32), side, SimulatorCommand{Kind: "play", Source: id})
		if step.Status != StatusSuspended || s.g.instances[target].life != 7 {
			t.Fatal(step)
		}
		restore := func(current *Session) *Session {
			t.Helper()
			data, err := current.EncodeContinuation()
			if err != nil {
				t.Fatal(err)
			}
			c, err := DecodeContinuation(data)
			if err != nil {
				t.Fatal(err)
			}
			out, err := RestoreSession(pack, c)
			if err != nil {
				t.Fatal(err)
			}
			return out
		}
		resume := func(current *Session, step StepResult) {
			t.Helper()
			r := current.Resume(ChoiceResponse{RequestID: step.Choice.RequestID, ActionID: step.Choice.ActionID, StateRevision: step.Choice.StateRevision, SelectedInstanceIDs: []string{target}})
			if r.Status != StatusCompleted {
				t.Fatal(r)
			}
		}
		restored := restore(s)
		for _, current := range []*Session{s, restored} {
			resume(current, step)
			if current.g.instances[target].life != 1 || !current.g.instances[target].abilities["barrier"] {
				t.Fatal("set life consumed barrier")
			}
		}
		if !reflect.DeepEqual(s.g.snapshot(), restored.g.snapshot()) {
			t.Fatal("restored fanfare diverged")
		}
		step = s.SubmitAs(strings.Repeat("b", 32), side, SimulatorCommand{Kind: "evolve", Source: id})
		if step.Status != StatusSuspended || s.g.instances[drawn].zone != "hand" {
			t.Fatal("evolution did not draw before choice", step)
		}
		restored = restore(s)
		for _, current := range []*Session{s, restored} {
			resume(current, step)
			if current.g.instances[target].life != 1 || current.g.instances[target].abilities["barrier"] || current.g.player(side).ep != 0 || current.g.instances[id].life != 5 {
				t.Fatal("evolution damage or resources incorrect")
			}
		}
		if !reflect.DeepEqual(s.g.snapshot(), restored.g.snapshot()) {
			t.Fatal("restored evolution diverged")
		}
	}
}
