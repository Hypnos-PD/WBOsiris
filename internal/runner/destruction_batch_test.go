package runner

import (
	"reflect"
	"strings"
	"testing"

	"wbo/internal/ir"
)

func TestReapersDeathslashSelectionsAndBatchForBothSeats(t *testing.T) {
	pack := repeatCardPack(t)
	for _, side := range []string{"own", "oppo"} {
		for _, protected := range []bool{false, true} {
			state := testState()
			state.Turn.Active = side
			source, ally, enemy := strings.Repeat("1", 32), strings.Repeat("2", 32), strings.Repeat("3", 32)
			owner := state.Players[side]
			owner.PP, owner.MaxPP = 1, 1
			owner = withInstance(owner, "hand", ir.TestInstance{InstanceID: source, CardID: 10151310, DeclaredType: "spell"})
			owner = withInstance(owner, "field", ir.TestInstance{InstanceID: ally, CardID: 10001110, DeclaredType: "follower"})
			state.Players[side] = owner
			state.Players[oppositeSide(side)] = withInstance(state.Players[oppositeSide(side)], "field", ir.TestInstance{InstanceID: enemy, CardID: 10001110, DeclaredType: "follower"})
			s, err := NewSession(pack, state, 1)
			if err != nil {
				t.Fatal(err)
			}
			s.g.instances[ally].superEvolved = protected
			s.g.instances[ally].evolved = protected
			before := s.g.snapshot()
			s.LegalActionsFor(side)
			if !reflect.DeepEqual(before, s.g.snapshot()) {
				t.Fatal("preflight changed state")
			}
			step := s.Begin(strings.Repeat("4", 32), ir.SourceAction{Kind: "play", Actor: side, Source: source})
			if step.Status != StatusSuspended || len(step.Choice.Candidates) != 1 || step.Choice.Candidates[0].InstanceID != ally {
				t.Fatal(step)
			}
			first := step.Choice
			step = s.Resume(ChoiceResponse{RequestID: first.RequestID, ActionID: first.ActionID, StateRevision: first.StateRevision, SelectedInstanceIDs: []string{ally}})
			if step.Status != StatusSuspended || len(step.Choice.Candidates) != 1 || step.Choice.Candidates[0].InstanceID != enemy {
				t.Fatal(step)
			}
			if s.g.instances[ally].zone != "field" || s.g.instances[enemy].zone != "field" || s.g.player(side).pp != 0 {
				t.Fatal("destroyed before both choices")
			}
			if r := s.Resume(ChoiceResponse{RequestID: first.RequestID, ActionID: first.ActionID, StateRevision: first.StateRevision, SelectedInstanceIDs: []string{enemy}}); r.Status != StatusRejected {
				t.Fatal("accepted first request for second selection")
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
			for _, current := range []*Session{s, restored} {
				choice := current.PendingChoice()
				r := current.Resume(ChoiceResponse{RequestID: choice.RequestID, ActionID: choice.ActionID, StateRevision: choice.StateRevision, SelectedInstanceIDs: []string{enemy}})
				if r.Status != StatusCompleted {
					t.Fatal(r)
				}
				if current.g.instances[enemy].zone != "graveyard" || (current.g.instances[ally].zone == "field") != protected {
					t.Fatal("wrong destruction protection")
				}
				var deaths []ir.RuntimeEvent
				for _, event := range current.g.events {
					if event.Kind == "destroyed" {
						deaths = append(deaths, event)
					}
				}
				if protected {
					if len(deaths) != 1 {
						t.Fatal(deaths)
					}
				} else if len(deaths) != 2 || deaths[0].BatchID == 0 || deaths[0].BatchID != deaths[1].BatchID || deaths[0].Subject.InstanceID != ally || deaths[1].Subject.InstanceID != enemy {
					t.Fatal("not one active-side-first death batch", deaths)
				}
			}
			if !reflect.DeepEqual(s.g.snapshot(), restored.g.snapshot()) {
				t.Fatal("restored batch diverged")
			}
		}
	}
}

func TestReapersDeathslashPreflightRequiresBothSides(t *testing.T) {
	pack := repeatCardPack(t)
	for _, missing := range []string{"own", "oppo", "aura", "guard"} {
		state := testState()
		source := strings.Repeat("1", 32)
		owner := state.Players["own"]
		owner.PP, owner.MaxPP = 1, 1
		state.Players["own"] = withInstance(owner, "hand", ir.TestInstance{InstanceID: source, CardID: 10151310, DeclaredType: "spell"})
		for n, side := range []string{"own", "oppo"} {
			if missing == side {
				continue
			}
			card := ir.TestInstance{InstanceID: strings.Repeat(string(rune('2'+n)), 32), CardID: 10001110, DeclaredType: "follower"}
			if side == "oppo" && (missing == "aura" || missing == "guard") {
				card.Overrides.Keywords = []string{"aura"}
			}
			if side == "oppo" && missing == "guard" {
				card.CardID = 90074120
			}
			state.Players[side] = withInstance(state.Players[side], "field", card)
		}
		s, err := NewSession(pack, state, 1)
		if err != nil {
			t.Fatal(err)
		}
		before := s.g.snapshot()
		step := s.Begin(strings.Repeat("4", 32), ir.SourceAction{Kind: "play", Actor: "own", Source: source})
		if step.Status != StatusIllegal || step.IllegalCode != "target_required" || !reflect.DeepEqual(before, s.g.snapshot()) {
			t.Fatal(missing, step)
		}
	}
}

func TestDestructionBatchDeduplicatesOverlappingBindings(t *testing.T) {
	pack := repeatCardPack(t)
	state := testState()
	id := strings.Repeat("1", 32)
	state.Players["own"] = withInstance(state.Players["own"], "field", ir.TestInstance{InstanceID: id, CardID: 10001110, DeclaredType: "follower"})
	s, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	f := frame{"first": bindEntities(s.g.instances[id]), "second": bindEntities(s.g.instances[id])}
	s.g.execTargetEffect(ir.TargetEffect{Kind: "destroy", Output: "destroyed", Target: ir.DestructionBatchRef{Kind: "destruction_batch", Targets: []ir.Ref{ir.BindingRef{Kind: "binding", Name: "first"}, ir.BindingRef{Kind: "binding", Name: "second"}}}}, nil, f)
	if len(f["destroyed"]) != 1 || s.g.own.shadows != 1 {
		t.Fatal("duplicated destruction", f)
	}
}

func TestMultipleRequirementsStillRejectStateDependentPreflight(t *testing.T) {
	pack := repeatCardPack(t)
	s, err := NewSession(pack, testState(), 1)
	if err != nil {
		t.Fatal(err)
	}
	require := ir.SelectionEffect{Kind: "require", Source: ir.CharacterSetRef{Kind: "characters", Side: "oppo"}}
	for _, body := range [][]ir.Effect{
		{ir.DrawEffect{Kind: "draw", Count: 1}, require},
		{ir.IfEffect{Condition: ir.CompareCondition{Kind: "compare", Left: ir.Scalar{Kind: "scalar", Side: "own", Field: "combo"}, Op: "ge", Right: 0}, Then: []ir.Effect{ir.DrawEffect{Kind: "draw", Count: 1}}}, require},
		{ir.SelectionEffect{Kind: "require", Source: ir.BindingRef{Kind: "binding", Name: "selected"}}},
		{ir.AdjustEffect{Kind: "adjust_resource", Owner: "own", Resource: "combo", Delta: 1}, ir.IfEffect{Condition: ir.CompareCondition{Kind: "compare", Left: ir.Scalar{Kind: "scalar", Side: "own", Field: "combo"}, Op: "ge", Right: 1}, Then: []ir.Effect{require}}},
	} {
		if code := s.g.preflightRequirements(body, nil, frame{}, true); code != "unsupported_preflight" {
			t.Fatal("accepted state-dependent requirement", code)
		}
	}
}
