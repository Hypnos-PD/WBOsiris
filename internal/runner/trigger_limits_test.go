package runner

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"wbo/internal/ir"
)

func assassinState(side, active string, copies int) ir.State {
	state := testState()
	state.Turn.Active = active
	state.Turn.Number = 6
	for n := 0; n < copies; n++ {
		state.Players[side] = withInstance(state.Players[side], "field", ir.TestInstance{InstanceID: fmt.Sprintf("%032x", n+1), CardID: 10171140, DeclaredType: "follower"})
	}
	for _, owner := range []string{"own", "oppo"} {
		for n := 0; n < 3; n++ {
			state.Players[owner] = withInstance(state.Players[owner], "deck", ir.TestInstance{InstanceID: fmt.Sprintf("%032x", 100+n+len(owner)*10), CardID: 10001110, DeclaredType: "follower"})
		}
	}
	return state
}

func TestLimitedListenerScopesFilterBeforeConsuming(t *testing.T) {
	for _, scope := range []string{"own", "oppo", "any"} {
		pack := repeatCardPack(t)
		for n := range pack.Cards {
			if pack.Cards[n].ID == 10171140 {
				for a := range pack.Cards[n].Abilities {
					if trigger, ok := pack.Cards[n].Abilities[a].Trigger.(ir.EventTrigger); ok {
						trigger.OncePerTurn = scope
						pack.Cards[n].Abilities[a].Trigger = trigger
					}
				}
			}
		}
		for _, owner := range []string{"own", "oppo"} {
			for _, active := range []string{"own", "oppo"} {
				t.Run(scope+"/"+owner+"/"+active, func(t *testing.T) {
					s, err := NewSession(pack, assassinState(owner, active, 1), 1)
					if err != nil {
						t.Fatal(err)
					}
					source := s.g.player(owner).field[0]
					s.g.summonFor(nil, oppositeSide(owner), 1, 90071120, false)
					s.g.summonFor(nil, owner, 1, 10001110, false)
					if len(source.usedTriggers) != 0 {
						t.Fatal("unmatched event consumed limit")
					}
					puppets := s.g.summonFor(nil, owner, 2, 90071120, false)
					want := scope == "any" || scope == "own" && owner == active || scope == "oppo" && owner != active
					if (len(s.g.triggers) == 1) != want || (len(source.usedTriggers) == 1) != want {
						t.Fatal("incorrect queue count", len(s.g.triggers))
					}
					if puppets[0].abilities["bane"] {
						t.Fatal("listener interrupted summoning effect")
					}
					if r := s.run(); r.Status != StatusCompleted {
						t.Fatal(r)
					}
					if puppets[0].abilities["bane"] != want || puppets[1].abilities["bane"] {
						t.Fatal("limit did not reserve first event")
					}
					view, err := s.View(owner)
					if err != nil || len(view.Own.Field[0].TriggerLimits) != 1 || view.Own.Field[0].TriggerLimits[0].Used != want {
						t.Fatal("incorrect limit view", err)
					}
				})
			}
		}
	}
}

func TestLimitedListenerInstancesAndAbilitiesAreIndependent(t *testing.T) {
	pack := repeatCardPack(t)
	for n := range pack.Cards {
		if pack.Cards[n].ID == 10171140 {
			for _, a := range pack.Cards[n].Abilities {
				if _, ok := a.Trigger.(ir.EventTrigger); ok {
					a.ID = strings.Repeat("f", 32)
					pack.Cards[n].Abilities = append(pack.Cards[n].Abilities, a)
					break
				}
			}
		}
	}
	s, err := NewSession(pack, assassinState("own", "own", 2), 1)
	if err != nil {
		t.Fatal(err)
	}
	s.g.summon(2, 90071120)
	if len(s.g.triggers) != 4 {
		t.Fatal("limits shared by distinct abilities or instances", len(s.g.triggers))
	}
	for _, i := range s.g.own.field[:2] {
		if len(i.usedTriggers) != 2 {
			t.Fatal("missing usage")
		}
	}
}

func TestLimitedListenerLifecycleAndHiddenView(t *testing.T) {
	pack := repeatCardPack(t)
	s, err := NewSession(pack, assassinState("own", "own", 1), 1)
	if err != nil {
		t.Fatal(err)
	}
	source := s.g.own.field[0]
	s.g.summon(1, 90071120)
	if r := s.run(); r.Status != StatusCompleted {
		t.Fatal(r)
	}
	before := triggerLimitViews(source)
	if r := s.Submit(strings.Repeat("a", 32), SimulatorCommand{Kind: "play", Source: source.id}); r.Status != StatusIllegal || !reflect.DeepEqual(before, triggerLimitViews(source)) {
		t.Fatal("illegal action changed usage", r)
	}
	clone := s.g.clone()
	for id := range clone.instances[source.id].usedTriggers {
		delete(clone.instances[source.id].usedTriggers, id)
	}
	if len(source.usedTriggers) != 1 {
		t.Fatal("sandbox shares usage map")
	}
	copy := s.g.copyInstance(source)
	if len(copy.usedTriggers) != 0 || len(source.usedTriggers) != 1 {
		t.Fatal("new copy inherited activation history")
	}
	s.g.addToZone(&s.g.own, copy, "hand")
	s.g.own.ep = 1
	if r := s.Submit(strings.Repeat("b", 32), SimulatorCommand{Kind: "evolve", Source: source.id}); r.Status != StatusCompleted || len(source.usedTriggers) != 1 {
		t.Fatal("evolution reset usage", r)
	}
	for n, side := range []string{"own", "oppo"} {
		if r := s.SubmitAs(fmt.Sprintf("%032x", n+30), side, SimulatorCommand{Kind: "end_turn"}); r.Status != StatusCompleted {
			t.Fatal(r)
		}
		if len(source.usedTriggers) != 0 {
			t.Fatal("turn did not reset usage")
		}
	}
	puppet := s.g.summon(1, 90071120)[0]
	if r := s.run(); r.Status != StatusCompleted || !puppet.abilities["bane"] {
		t.Fatal("new own turn did not activate", r)
	}
	s.g.returnCard(source, "hand")
	if len(source.usedTriggers) != 0 {
		t.Fatal("return to hand did not reset usage")
	}
	view, err := s.View("oppo")
	if err != nil || len(view.Oppo.Hand) != 0 {
		t.Fatal("private limit leaked", err)
	}
	s.g.own.pp = 10
	if r := s.Submit(strings.Repeat("c", 32), SimulatorCommand{Kind: "play", Source: source.id}); r.Status != StatusCompleted {
		t.Fatal(r)
	}
	puppet = s.g.summon(1, 90071120)[0]
	if r := s.run(); r.Status != StatusCompleted || !puppet.abilities["bane"] {
		t.Fatal("replayed source did not activate", r)
	}
	s.g.execCardEffect(ir.CardEffect{Kind: "transform", Target: ir.SelfRef{Kind: "self"}, CardID: 10001110}, source, nil)
	if len(source.usedTriggers) != 0 {
		t.Fatal("transform retained usage")
	}
}

func TestLimitedListenerQueueBudgetDoesNotConsume(t *testing.T) {
	s, err := NewSession(repeatCardPack(t), assassinState("own", "own", 1), 1)
	if err != nil {
		t.Fatal(err)
	}
	s.budget.policy.Triggers = 0
	s.g.budget = &s.budget
	s.g.summon(1, 90071120)
	if !s.budget.exceeded || len(s.g.triggers) != 0 || len(s.g.own.field[0].usedTriggers) != 0 {
		t.Fatal("failed queue consumed usage")
	}
}

func TestLimitedListenerContinuationBeforeAndDuringResolution(t *testing.T) {
	for _, inside := range []bool{false, true} {
		pack := repeatCardPack(t)
		pause := ir.ModeEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("e", 32)}, Kind: "mode", Options: []ir.ModeOption{{ID: 1, Body: []ir.Effect{}}}}
		body := []ir.Effect{ir.CardEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("d", 32)}, Kind: "summon", Owner: "own", Count: 2, CardID: 90071120, Output: "summoned"}}
		if inside {
			for n := range pack.Cards {
				if pack.Cards[n].ID == 10171140 {
					for a := range pack.Cards[n].Abilities {
						if _, ok := pack.Cards[n].Abilities[a].Trigger.(ir.EventTrigger); ok {
							pack.Cards[n].Abilities[a].Body = append([]ir.Effect{pause}, pack.Cards[n].Abilities[a].Body...)
						}
					}
				}
			}
		} else {
			body = append(body, pause)
		}
		pack.Cards = append(pack.Cards, ir.Card{ID: 88880005, CardType: "spell", PlayEffects: body})
		state := assassinState("own", "own", 1)
		spell := strings.Repeat("a", 32)
		state.Players["own"] = withInstance(state.Players["own"], "hand", ir.TestInstance{InstanceID: spell, CardID: 88880005, DeclaredType: "spell"})
		s, err := NewSession(pack, state, 1)
		if err != nil {
			t.Fatal(err)
		}
		step := s.Submit(strings.Repeat("b", 32), SimulatorCommand{Kind: "play", Source: spell})
		if step.Status != StatusSuspended || len(s.g.own.field[0].usedTriggers) != 1 {
			t.Fatal("missing reserved usage", step)
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
		for _, mutation := range []string{"unknown", "false", "wrong_turn", "missing"} {
			if inside && mutation == "missing" {
				continue
			}
			bad, err := DecodeContinuation(data)
			if err != nil {
				t.Fatal(err)
			}
			for n := range bad.Game.Instances {
				entity := &bad.Game.Instances[n]
				if entity.CardID != 10171140 {
					continue
				}
				switch mutation {
				case "unknown":
					entity.UsedTriggers["unknown"] = true
				case "false":
					for id := range entity.UsedTriggers {
						entity.UsedTriggers[id] = false
					}
				case "missing":
					entity.UsedTriggers = nil
				case "wrong_turn":
					bad.Game.Own.Field, bad.Game.Oppo.Field = bad.Game.Own.Field[1:], append(bad.Game.Oppo.Field, entity.ID)
				}
			}
			if _, err := RestoreSession(pack, bad); err == nil {
				t.Fatal("accepted malformed usage", mutation)
			}
		}
		response := ChoiceResponse{RequestID: step.Choice.RequestID, ActionID: step.Choice.ActionID, StateRevision: step.Choice.StateRevision, SelectedOptionID: 1}
		for _, current := range []*Session{s, restored} {
			if r := current.Resume(response); r.Status != StatusCompleted {
				t.Fatal(r)
			}
			if !current.g.own.field[1].abilities["bane"] || current.g.own.field[2].abilities["bane"] {
				t.Fatal("restored limit changed target")
			}
			third := current.g.summon(1, 90071120)[0]
			if r := current.run(); r.Status != StatusCompleted || third.abilities["bane"] {
				t.Fatal("restored quota triggered twice", r)
			}
		}
		if !reflect.DeepEqual(s.g.snapshot(), restored.g.snapshot()) {
			t.Fatal("continuation diverged")
		}
	}
}
