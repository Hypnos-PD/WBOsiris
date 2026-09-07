package runner

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"wbo/internal/ir"
)

func TestCompiledAlphaAcceptsEitherOrBothMaterialKinds(t *testing.T) {
	pack := repeatCardPack(t)
	for _, selection := range [][]int{{90073120}, {90073130}, {90073120, 90073130}} {
		t.Run(fmt.Sprint(selection), func(t *testing.T) {
			state := testState()
			sourceID := strings.Repeat("1", 32)
			actor := withInstance(state.Players["own"], "hand", ir.TestInstance{InstanceID: sourceID, CardID: 90073110, DeclaredType: "follower"})
			var ids []string
			for n, id := range selection {
				instanceID := fmt.Sprintf("%032x", n+2)
				ids = append(ids, instanceID)
				actor = withInstance(actor, "hand", ir.TestInstance{InstanceID: instanceID, CardID: id, DeclaredType: "follower"})
			}
			state.Players["own"] = withInstance(actor, "hand", ir.TestInstance{InstanceID: strings.Repeat("4", 32), CardID: 90072110, DeclaredType: "follower"})
			s, err := NewSession(pack, state, 1)
			if err != nil {
				t.Fatal(err)
			}
			step := s.Submit(strings.Repeat("5", 32), SimulatorCommand{Kind: "fusion", Source: sourceID})
			if step.Status != StatusSuspended || len(step.Choice.Candidates) != len(ids) {
				t.Fatalf("missing alternative or included invalid material: %#v", step)
			}
			step = s.Resume(ChoiceResponse{RequestID: step.Choice.RequestID, ActionID: step.Choice.ActionID, StateRevision: step.Choice.StateRevision, SelectedInstanceIDs: ids})
			if step.Status != StatusCompleted || len(s.g.instances[sourceID].materials) != len(ids) || s.g.own.shadows != 0 {
				t.Fatal("material union did not resolve atomically", step)
			}
			source := s.g.instances[sourceID]
			if len(ids) == 2 {
				if source.card.ID != 90074110 || source.cost != 10 || source.attack != 10 || source.life != 10 || source.fusedThisTurn || !source.abilities["storm"] {
					t.Fatal("both material kinds did not transform to omega")
				}
			} else if source.card.ID != 90073110 || !source.fusedThisTurn {
				t.Fatal("single-kind fusion transformed early or did not consume use")
			}
		})
	}
}

func TestFusionLimitSurvivesContinuationAndResetsNextTurn(t *testing.T) {
	pack := repeatCardPack(t)
	for _, side := range []string{"own", "oppo"} {
		t.Run(side, func(t *testing.T) {
			state := testState()
			state.Turn.Active = side
			ids := make([]string, 5)
			actor := state.Players[side]
			for n, cardID := range []int{90073110, 90073120, 90073130, 90073110, 90073130} {
				ids[n] = fmt.Sprintf("%032x", n+1)
				actor = withInstance(actor, "hand", ir.TestInstance{InstanceID: ids[n], CardID: cardID, DeclaredType: "follower"})
			}
			state.Players[side] = actor
			for n, owner := range []string{"own", "oppo"} {
				state.Players[owner] = withInstance(state.Players[owner], "deck", ir.TestInstance{InstanceID: fmt.Sprintf("%032x", n+16), CardID: 10001110, DeclaredType: "follower"})
			}
			s, err := NewSession(pack, state, 1)
			if err != nil {
				t.Fatal(err)
			}
			step := s.SubmitAs(strings.Repeat("6", 32), side, SimulatorCommand{Kind: "fusion", Source: ids[0]})
			if step.Status != StatusSuspended {
				t.Fatal(step)
			}
			choice := step.Choice
			bad := s.Resume(ChoiceResponse{RequestID: choice.RequestID, ActionID: choice.ActionID, StateRevision: choice.StateRevision, SelectedInstanceIDs: []string{ids[1], ids[1]}})
			if bad.Status != StatusRejected || s.g.instances[ids[0]].fusedThisTurn || len(s.g.instances[ids[0]].materials) != 0 {
				t.Fatal("rejected response consumed fusion")
			}
			step = s.Resume(ChoiceResponse{RequestID: choice.RequestID, ActionID: choice.ActionID, StateRevision: choice.StateRevision, SelectedInstanceIDs: []string{ids[1]}})
			if step.Status != StatusCompleted {
				t.Fatal(step)
			}
			s.g.returnCard(s.g.instances[ids[0]], "deck")
			s.g.move(s.g.instances[ids[0]], "hand")
			if !s.g.instances[ids[0]].fusedThisTurn {
				t.Fatal("returning to deck and drawing again refreshed fusion")
			}
			for _, action := range s.LegalActionsFor(side) {
				if action.Kind == "fusion" && action.Source == ids[0] {
					t.Fatal("used fusion remains in legal actions")
				}
			}
			step = s.SubmitAs(strings.Repeat("7", 32), side, SimulatorCommand{Kind: "fusion", Source: ids[3]})
			if step.Status != StatusSuspended {
				t.Fatal("another copy should still be able to fuse", step)
			}
			data, err := s.EncodeContinuation()
			if err != nil {
				t.Fatal(err)
			}
			saved, err := DecodeContinuation(data)
			if err != nil {
				t.Fatal(err)
			}
			tampered, err := DecodeContinuation(data)
			if err != nil {
				t.Fatal(err)
			}
			for n := range tampered.Game.Instances {
				if tampered.Game.Instances[n].ID == ids[3] {
					tampered.Game.Instances[n].FusedThisTurn = true
				}
			}
			if _, err := RestoreSession(pack, tampered); err == nil {
				t.Fatal("restored a pending fusion for an already used source")
			}
			s, err = RestoreSession(pack, saved)
			if err != nil {
				t.Fatal(err)
			}
			if !s.g.instances[ids[0]].fusedThisTurn || s.g.instances[ids[3]].fusedThisTurn {
				t.Fatal("continuation lost per-instance fusion allowance")
			}
			step = s.Resume(ChoiceResponse{RequestID: step.Choice.RequestID, ActionID: step.Choice.ActionID, StateRevision: step.Choice.StateRevision, SelectedInstanceIDs: []string{ids[4]}})
			if step.Status != StatusCompleted {
				t.Fatal(step)
			}
			before := s.g.snapshot()
			step = s.SubmitAs(strings.Repeat("8", 32), side, SimulatorCommand{Kind: "fusion", Source: ids[0]})
			if step.Status != StatusIllegal || step.IllegalCode != "already_fused" || !reflect.DeepEqual(before, s.g.snapshot()) {
				t.Fatal("second fusion was not rejected without mutation", step)
			}
			for n, owner := range []string{side, oppositeSide(side)} {
				step = s.SubmitAs(fmt.Sprintf("%032x", n+32), owner, SimulatorCommand{Kind: "end_turn"})
				if step.Status != StatusCompleted {
					t.Fatal(step)
				}
			}
			step = s.SubmitAs(strings.Repeat("9", 32), side, SimulatorCommand{Kind: "fusion", Source: ids[0]})
			if step.Status != StatusSuspended {
				t.Fatal("fusion did not reset on next turn", step)
			}
			step = s.Resume(ChoiceResponse{RequestID: step.Choice.RequestID, ActionID: step.Choice.ActionID, StateRevision: step.Choice.StateRevision, SelectedInstanceIDs: []string{ids[2]}})
			if step.Status != StatusCompleted || s.g.instances[ids[0]].card.ID != 90074110 {
				t.Fatal("cross-turn distinct material accumulation failed", step)
			}
		})
	}
}

func TestTransformAllowsNewFusionInSameTurn(t *testing.T) {
	pack := repeatCardPack(t)
	state := testState()
	ids := []string{strings.Repeat("1", 32), strings.Repeat("2", 32), strings.Repeat("3", 32)}
	for _, id := range ids {
		state.Players["own"] = withInstance(state.Players["own"], "hand", ir.TestInstance{InstanceID: id, CardID: 90071210, DeclaredType: "amulet"})
	}
	s, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	for n, material := range ids[1:] {
		step := s.Submit(fmt.Sprintf("%032x", n+16), SimulatorCommand{Kind: "fusion", Source: ids[0]})
		if step.Status != StatusSuspended {
			t.Fatal("new transformation was blocked from fusion", step)
		}
		step = s.Resume(ChoiceResponse{RequestID: step.Choice.RequestID, ActionID: step.Choice.ActionID, StateRevision: step.Choice.StateRevision, SelectedInstanceIDs: []string{material}})
		if step.Status != StatusCompleted {
			t.Fatal(step)
		}
	}
	if i := s.g.instances[ids[0]]; i.card.ID != 90073120 || len(i.materials) != 2 || i.fusedThisTurn {
		t.Fatal("transformation did not preserve materials and reset turn state")
	}
}

func TestFusionPreflightChecksLaterDeclarations(t *testing.T) {
	card := ir.Card{ID: 77773001, CardType: "amulet"}
	for n, id := range []int{77773002, 77773003} {
		card.FusionAbilities = append(card.FusionAbilities, ir.FusionAbility{ID: fmt.Sprintf("%032x", n+16), MaterialFilter: ir.MaterialFilter{Kind: "material_filter", Source: ir.ZoneRef{Kind: "zone", Side: "own", Zone: "hand", Member: "card"}, Minimum: 1, Predicate: ir.FieldPredicate{Kind: "has_card", CardID: id}}})
	}
	state := testState()
	sourceID, materialID := strings.Repeat("1", 32), strings.Repeat("2", 32)
	state.Players["own"] = withInstance(state.Players["own"], "hand", ir.TestInstance{InstanceID: sourceID, CardID: card.ID, DeclaredType: "amulet"})
	state.Players["own"] = withInstance(state.Players["own"], "hand", ir.TestInstance{InstanceID: materialID, CardID: 77773003, DeclaredType: "amulet"})
	s, err := NewSession(&ir.CardPack{Cards: []ir.Card{card, {ID: 77773003, CardType: "amulet"}}}, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	step := s.Submit(strings.Repeat("3", 32), SimulatorCommand{Kind: "fusion", Source: sourceID})
	if step.Status != StatusSuspended || step.Choice.NodeID != card.FusionAbilities[1].ID || step.Choice.Candidates[0].InstanceID != materialID {
		t.Fatal("preflight did not consider second declaration", step)
	}
}

func TestFusionEventBudgetFailureDoesNotConsumeMaterials(t *testing.T) {
	pack := repeatCardPack(t)
	state := testState()
	id, materialID := strings.Repeat("1", 32), strings.Repeat("2", 32)
	for _, instanceID := range []string{id, materialID} {
		state.Players["own"] = withInstance(state.Players["own"], "hand", ir.TestInstance{InstanceID: instanceID, CardID: 90071210, DeclaredType: "amulet"})
	}
	s, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	s.budgetPolicy.Events = 0
	step := s.Submit(strings.Repeat("3", 32), SimulatorCommand{Kind: "fusion", Source: id})
	if step.Status != StatusSuspended {
		t.Fatal(step)
	}
	step = s.Resume(ChoiceResponse{RequestID: step.Choice.RequestID, ActionID: step.Choice.ActionID, StateRevision: step.Choice.StateRevision, SelectedInstanceIDs: []string{materialID}})
	if step.Status != StatusFault || step.ErrorCode != executionBudgetExceeded || len(s.g.own.hand) != 2 || len(s.g.instances[id].materials) != 0 || s.g.instances[id].fusedThisTurn || len(s.Events()) != 0 {
		t.Fatal("fusion budget exhaustion consumed materials or reported a choice error", step)
	}
}
