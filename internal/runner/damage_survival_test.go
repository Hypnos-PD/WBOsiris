package runner

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"wbo/internal/ir"
)

func survivalState(side string) ir.State {
	state := testState()
	state.Turn.Active = side
	state.Players[side] = withInstance(state.Players[side], "field", ir.TestInstance{InstanceID: strings.Repeat("1", 32), CardID: 10341110, DeclaredType: "follower"})
	for n, card := range []int{10041110, 10041110, 10001110} {
		state.Players[side] = withInstance(state.Players[side], "deck", ir.TestInstance{InstanceID: fmt.Sprintf("%032x", 100+n), CardID: card, DeclaredType: "follower"})
	}
	return state
}

func TestDevoteeSurvivesZeroAndPreventedDamageOnEitherSeat(t *testing.T) {
	for _, side := range []string{"own", "oppo"} {
		for _, tc := range []struct {
			name           string
			amount         int
			barrier, super bool
			reduction      int
		}{
			{name: "zero", amount: 0}, {name: "zero_barrier", amount: 0, barrier: true},
			{name: "barrier", amount: 3, barrier: true}, {name: "super", amount: 3, super: true},
			{name: "reduction", amount: 1, reduction: 1}, {name: "nonlethal", amount: 1},
		} {
			t.Run(side+"/"+tc.name, func(t *testing.T) {
				s, err := NewSession(repeatCardPack(t), survivalState(side), 1)
				if err != nil {
					t.Fatal(err)
				}
				source := s.g.player(side).field[0]
				if tc.barrier {
					source.addKeyword("barrier", "")
				}
				source.superEvolved = tc.super
				source.evolved = tc.super
				source.damageReduction = tc.reduction
				s.g.damageInstance(source, tc.amount)
				s.g.resolveDeathBatch(nil)
				if r := s.run(); r.Status != StatusCompleted {
					t.Fatal(r)
				}
				p := s.g.player(side)
				if len(p.hand) != 1 || p.hand[0].card.ID != 10041110 || source.zone != "field" {
					t.Fatal("surviving damage did not draw a Dragoncraft follower")
				}
				if tc.barrier && source.abilities["barrier"] != (tc.amount == 0) {
					t.Fatal("zero damage consumed barrier or positive damage retained it")
				}
				if len(s.Events()) == 0 || s.Events()[0].Kind != "damaged" || s.Events()[0].Actual != 2-source.life {
					t.Fatal("missing actual damage fact")
				}
			})
		}
	}
}

func TestDamageSurvivalRejectsLethalOffTurnAndDepartedSources(t *testing.T) {
	for _, side := range []string{"own", "oppo"} {
		for _, reason := range []string{"lethal", "off_turn", "departed", "other_target", "bane"} {
			t.Run(side+"/"+reason, func(t *testing.T) {
				state := survivalState(side)
				state.Players[oppositeSide(side)] = withInstance(state.Players[oppositeSide(side)], "field", ir.TestInstance{InstanceID: strings.Repeat("2", 32), CardID: 10001120, DeclaredType: "follower"})
				s, err := NewSession(repeatCardPack(t), state, 1)
				if err != nil {
					t.Fatal(err)
				}
				source := s.g.player(side).field[0]
				switch reason {
				case "lethal":
					s.g.damageInstance(source, 2)
				case "off_turn":
					s.g.turn.Active = oppositeSide(side)
					s.g.damageInstance(source, 0)
				case "departed":
					s.g.damageInstance(source, 1)
					s.g.destroyByEffect([]*instance{source})
				case "other_target":
					s.g.damageInstance(s.g.player(oppositeSide(side)).field[0], 0)
				case "bane":
					defender := s.g.player(oppositeSide(side)).field[0]
					defender.addKeyword("bane", "")
					defender.life = 5
					if r := s.SubmitAs(strings.Repeat("a", 32), side, SimulatorCommand{Kind: "attack", Source: source.id, Defender: defender.id}); r.Status != StatusCompleted {
						t.Fatal(r)
					}
				}
				s.g.resolveDeathBatch(nil)
				if r := s.run(); r.Status != StatusCompleted {
					t.Fatal(r)
				}
				if len(s.g.player(side).hand) != 0 {
					t.Fatal("ineligible survival triggered")
				}
			})
		}
	}
}

func TestDevoteeOfficialZeroAttackCombatAndReadOnlyPreview(t *testing.T) {
	for _, side := range []string{"own", "oppo"} {
		state := survivalState(side)
		defender := strings.Repeat("2", 32)
		state.Players[oppositeSide(side)] = withInstance(state.Players[oppositeSide(side)], "field", ir.TestInstance{InstanceID: defender, CardID: 10001120, DeclaredType: "follower"})
		s, err := NewSession(repeatCardPack(t), state, 1)
		if err != nil {
			t.Fatal(err)
		}
		before := s.g.snapshot()
		s.LegalActionsFor(side)
		if !reflect.DeepEqual(before, s.g.snapshot()) {
			t.Fatal("preview mutated survival or RNG")
		}
		r := s.SubmitAs(strings.Repeat("a", 32), side, SimulatorCommand{Kind: "attack", Source: strings.Repeat("1", 32), Defender: defender})
		if r.Status != StatusCompleted || len(s.g.player(side).hand) != 1 || s.g.player(side).field[0].life != 2 || s.g.instances[defender].zone != "graveyard" {
			t.Fatal("official zero-attack example diverged", r)
		}
	}
}

func TestDamageSurvivalOnceLimitAndContinuation(t *testing.T) {
	pack := repeatCardPack(t)
	pause := ir.ModeEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("e", 32)}, Kind: "mode", Options: []ir.ModeOption{{ID: 1, Body: []ir.Effect{}}}}
	for n := range pack.Cards {
		if pack.Cards[n].ID == 10341110 {
			ability := &pack.Cards[n].Abilities[0]
			trigger := ability.Trigger.(ir.EventTrigger)
			trigger.OncePerTurn = "any"
			ability.Trigger = trigger
			ability.Body = append([]ir.Effect{pause}, ability.Body...)
		}
	}
	state := survivalState("own")
	defender := strings.Repeat("2", 32)
	state.Players["oppo"] = withInstance(state.Players["oppo"], "field", ir.TestInstance{InstanceID: defender, CardID: 10001120, DeclaredType: "follower"})
	s, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	step := s.Submit(strings.Repeat("a", 32), SimulatorCommand{Kind: "attack", Source: strings.Repeat("1", 32), Defender: defender})
	if step.Status != StatusSuspended || len(s.g.own.field[0].usedTriggers) != 1 {
		t.Fatal("survival did not suspend with reserved limit", step)
	}
	data, err := s.EncodeContinuation()
	if err != nil {
		t.Fatal(err)
	}
	checkpoint, err := DecodeContinuation(data)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := RestoreSession(pack, checkpoint)
	if err != nil {
		t.Fatal(err)
	}
	response := ChoiceResponse{RequestID: step.Choice.RequestID, ActionID: step.Choice.ActionID, StateRevision: step.Choice.StateRevision, SelectedOptionID: 1}
	for _, current := range []*Session{s, restored} {
		if r := current.Resume(response); r.Status != StatusCompleted {
			t.Fatal(r)
		}
		current.g.damageInstance(current.g.own.field[0], 0)
		if r := current.run(); r.Status != StatusCompleted || len(current.g.own.hand) != 1 {
			t.Fatal("survival fired twice", r)
		}
	}
	if !reflect.DeepEqual(s.g.snapshot(), restored.g.snapshot()) {
		t.Fatal("restored survival state diverged")
	}
	bad, _ := DecodeContinuation(data)
	bad.Game.Turn.Active = "oppo"
	if _, err := RestoreSession(pack, bad); err == nil {
		t.Fatal("accepted once-per-turn usage outside during scope")
	}
}
