package runner

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"wbo/internal/ir"
)

func TestDimensionClimbMatchesOfficialSpellboostExamples(t *testing.T) {
	pack := repeatCardPack(t)
	for _, side := range []string{"own", "oppo"} {
		seenRedrawn, seenInDeck := false, false
		for seed := uint64(1); seed <= 24; seed++ {
			state := testState()
			state.Turn.Active = side
			actor := state.Players[side]
			actor.PP, actor.MaxPP = 5, 8
			sourceID, heldID := strings.Repeat("1", 32), strings.Repeat("2", 32)
			sourceCost, heldCost := 5, 6
			actor = withInstance(actor, "hand", ir.TestInstance{InstanceID: sourceID, CardID: 10134310, DeclaredType: "spell", Overrides: ir.InstanceOverrides{Cost: &sourceCost}})
			actor = withInstance(actor, "hand", ir.TestInstance{InstanceID: heldID, CardID: 10133320, DeclaredType: "spell", Overrides: ir.InstanceOverrides{Cost: &heldCost}})
			for n := 0; n < 5; n++ {
				actor = withInstance(actor, "deck", ir.TestInstance{InstanceID: fmt.Sprintf("%032x", n+16), CardID: 10133320, DeclaredType: "spell"})
			}
			state.Players[side] = actor
			s, err := NewSession(pack, state, seed)
			if err != nil {
				t.Fatal(err)
			}
			step := s.SubmitAs(strings.Repeat("3", 32), side, SimulatorCommand{Kind: "play", Source: sourceID})
			if step.Status != StatusCompleted {
				t.Fatal("compiled Dimension Climb did not resolve", step)
			}
			player := s.g.player(side)
			if player.pp != 8 || player.maxpp != 8 || len(player.hand) != 5 || len(player.deck) != 1 || player.shadows != 1 || s.g.instances[sourceID].zone != "graveyard" || s.g.instances[sourceID].cost != 5 {
				t.Fatal("Dimension Climb lost source, draw count, payment or restoration")
			}
			held := s.g.instances[heldID]
			if held.zone == "hand" {
				seenRedrawn = true
				if held.cost != 0 {
					t.Fatal("redrawn Demonic Call did not receive one queued plus five explicit boosts", held.cost)
				}
			} else {
				seenInDeck = true
				if held.cost != 5 {
					t.Fatal("Demonic Call remaining in deck lost its queued boost", held.cost)
				}
			}
			for _, card := range player.hand {
				if card.id != heldID && card.cost != 2 {
					t.Fatal("newly drawn card incorrectly received automatic boost", card.cost)
				}
			}
			last := s.Events()[len(s.Events())-1]
			if last.Kind != "pp_restored" || last.Side != side || last.Actual != 8 || s.g.rng.Consumed() != 1 {
				t.Fatal("wrong restoration event or shuffle consumption", last)
			}
		}
		if !seenRedrawn || !seenInDeck {
			t.Fatal("seeds did not exercise both official examples")
		}
	}
}

func TestRestorePPReadsCurrentLimitAndPreservesExtraPP(t *testing.T) {
	for _, actor := range []string{"own", "oppo"} {
		for _, owner := range []string{"own", "oppo"} {
			for _, current := range []int{0, 3, 7, 8} {
				state := testState()
				id := strings.Repeat("1", 32)
				state.Players[actor] = withInstance(state.Players[actor], "field", ir.TestInstance{InstanceID: id, CardID: 77774001, DeclaredType: "follower"})
				s, err := NewSession(&ir.CardPack{Cards: []ir.Card{{ID: 77774001, CardType: "follower", Stats: &ir.Stats{Attack: 1, Life: 1}}}}, state, 1)
				if err != nil {
					t.Fatal(err)
				}
				side := actor
				if owner == "oppo" {
					side = oppositeSide(actor)
				}
				player := s.g.player(side)
				player.pp, player.maxpp = current, 7
				s.g.execAdjust(ir.AdjustEffect{Kind: "restore_resource", Owner: owner, Resource: "pp"}, s.g.instances[id], nil)
				if player.pp != max(current, 7) || player.maxpp != 7 || s.g.player(oppositeSide(side)).pp != 0 || s.Events()[0].Actual != max(0, 7-current) || s.Events()[0].Side != side {
					t.Fatal("restoration used wrong owner, reduced PP, or changed the maximum")
				}
			}
		}
	}
}

func TestDimensionClimbStopsOnDeckOut(t *testing.T) {
	pack := repeatCardPack(t)
	state := testState()
	state.FirstPlayer = "own"
	zero := 0
	id := strings.Repeat("1", 32)
	actor := state.Players["own"]
	actor.PP, actor.MaxPP = 1, 8
	state.Players["own"] = withInstance(actor, "hand", ir.TestInstance{InstanceID: id, CardID: 10134310, DeclaredType: "spell", Overrides: ir.InstanceOverrides{Cost: &zero}})
	s, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	step := s.Submit(strings.Repeat("2", 32), SimulatorCommand{Kind: "play", Source: id})
	if step.Status != StatusCompleted || !s.g.gameOver || s.g.winner != "oppo" || s.g.own.pp != 1 {
		t.Fatal("deck-out did not stop subsequent PP restoration", step)
	}
}

func TestDimensionClimbRestoresAfterContinuation(t *testing.T) {
	pack := repeatCardPack(t)
	for n := range pack.Cards {
		card := &pack.Cards[n]
		if card.ID != 10134310 {
			continue
		}
		body := append([]ir.Effect(nil), card.PlayEffects...)
		last := body[len(body)-1].(ir.AdjustEffect)
		pause := ir.ModeEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("f", 32), Origin: last.Origin}, Kind: "mode", Options: []ir.ModeOption{{ID: 1}}}
		card.PlayEffects = append(body[:len(body)-1], pause, last)
	}
	state := testState()
	actor := state.Players["own"]
	actor.PP, actor.MaxPP = 5, 8
	sourceID, heldID := strings.Repeat("1", 32), strings.Repeat("2", 32)
	sourceCost, heldCost := 5, 6
	actor = withInstance(actor, "hand", ir.TestInstance{InstanceID: sourceID, CardID: 10134310, DeclaredType: "spell", Overrides: ir.InstanceOverrides{Cost: &sourceCost}})
	actor = withInstance(actor, "hand", ir.TestInstance{InstanceID: heldID, CardID: 10133320, DeclaredType: "spell", Overrides: ir.InstanceOverrides{Cost: &heldCost}})
	for n := 0; n < 4; n++ {
		actor = withInstance(actor, "deck", ir.TestInstance{InstanceID: fmt.Sprintf("%032x", n+16), CardID: 10133320, DeclaredType: "spell"})
	}
	state.Players["own"] = actor
	s, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	step := s.Submit(strings.Repeat("3", 32), SimulatorCommand{Kind: "play", Source: sourceID})
	if step.Status != StatusSuspended || s.g.own.pp != 0 || s.g.instances[heldID].cost != 6 {
		t.Fatal("spell did not suspend before restoration and queued boosts", step)
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
	choice := step.Choice
	answer := ChoiceResponse{RequestID: choice.RequestID, ActionID: choice.ActionID, StateRevision: choice.StateRevision, SelectedOptionID: 1}
	result, other := s.Resume(answer), restored.Resume(answer)
	if result.Status != StatusCompleted || s.g.own.pp != 8 || s.g.instances[heldID].cost != 0 {
		t.Fatal("resumed spell lost restoration or boosts", result)
	}
	if !reflect.DeepEqual(result, other) || !reflect.DeepEqual(s.g.snapshot(), restored.g.snapshot()) || !reflect.DeepEqual(s.Events(), restored.Events()) || s.BudgetState() != restored.BudgetState() {
		t.Fatal("restored spell diverged")
	}
}
