package runner

import (
	"reflect"
	"strings"
	"testing"

	"wbo/internal/ir"
)

func TestCounterChoiceRestoreAndIsolation(t *testing.T) {
	pack := repeatCardPack(t)
	id, targetID := strings.Repeat("1", 32), strings.Repeat("2", 32)
	for _, side := range []string{"own", "oppo"} {
		t.Run(side, func(t *testing.T) {
			state := testState()
			state.Turn.Active = side
			actor := state.Players[side]
			actor.PP, actor.MaxPP = 1, 1
			state.Players[side] = actor
			state.Players[side] = withInstance(state.Players[side], "hand", ir.TestInstance{InstanceID: id, CardID: 10131320, DeclaredType: "spell", Overrides: ir.InstanceOverrides{Counters: map[string]int{"x": 7}}})
			state.Players[oppositeSide(side)] = withInstance(state.Players[oppositeSide(side)], "field", ir.TestInstance{InstanceID: targetID, CardID: 90021110, DeclaredType: "follower", Overrides: ir.InstanceOverrides{Stats: &ir.Stats{Attack: 3, Life: 10}}})
			s, err := NewSession(pack, state, 1)
			if err != nil {
				t.Fatal(err)
			}
			view, err := s.View(side)
			if err != nil || view.Own.Hand[0].Counters["x"] != 7 {
				t.Fatal(view, err)
			}
			view.Own.Hand[0].Counters["x"] = 99
			other, _ := s.View(oppositeSide(side))
			if len(other.Oppo.Hand) != 0 {
				t.Fatal("opponent can see hidden counter")
			}
			clone := s.g.clone()
			clone.instances[id].counters["x"] = 88
			if s.g.instances[id].counters["x"] != 7 {
				t.Fatal("view or preflight clone aliased counter")
			}
			step := s.SubmitAs(strings.Repeat("a", 32), side, SimulatorCommand{Kind: "play", Source: id})
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
			for n := range saved.Game.Instances {
				if saved.Game.Instances[n].ID == id {
					saved.Game.Instances[n].Counters["x"] = 100
				}
			}
			if restored.g.instances[id].counters["x"] != 7 {
				t.Fatal("restore aliased input counters")
			}
			for _, bad := range []map[string]int{nil, {"x": -1}, {"x": ir.MaxCounterValue + 1}, {"y": 7}, {"x": 7, "extra": 1}} {
				tampered, _ := DecodeContinuation(data)
				for n := range tampered.Game.Instances {
					if tampered.Game.Instances[n].ID == id {
						tampered.Game.Instances[n].Counters = bad
					}
				}
				if _, err := RestoreSession(pack, tampered); err == nil {
					t.Fatal("restored invalid counters", bad)
				}
			}
			response := ChoiceResponse{RequestID: step.Choice.RequestID, ActionID: step.Choice.ActionID, StateRevision: step.Choice.StateRevision, SelectedInstanceIDs: []string{targetID}}
			if restored.Resume(response).Status != StatusCompleted || s.Resume(response).Status != StatusCompleted {
				t.Fatal("cannot resume")
			}
			if restored.g.instances[targetID].life != 3 || !reflect.DeepEqual(s.g.snapshot(), restored.g.snapshot()) {
				t.Fatal("restored counter changed damage")
			}
		})
	}
}

func TestCounterZoneAndIdentityLifecycle(t *testing.T) {
	pack := repeatCardPack(t)
	for _, side := range []string{"own", "oppo"} {
		for _, sourceZone := range []string{"hand", "field"} {
			for _, destination := range []string{"hand", "deck"} {
				if sourceZone == "hand" && destination == "hand" {
					continue
				}
				state := testState()
				id := strings.Repeat("1", 32)
				state.Players[side] = withInstance(state.Players[side], sourceZone, ir.TestInstance{InstanceID: id, CardID: 10132130, DeclaredType: "follower", Overrides: ir.InstanceOverrides{Counters: map[string]int{"x": 5}}})
				s, err := NewSession(pack, state, 1)
				if err != nil {
					t.Fatal(err)
				}
				i := s.g.instances[id]
				if sourceZone == "field" {
					view, _ := s.View(oppositeSide(side))
					if view.Oppo.Field[0].Counters["x"] != 5 {
						t.Fatal("field counter is not public")
					}
				}
				s.g.returnCard(i, destination)
				want := 0
				if sourceZone == "hand" {
					want = 5
				}
				if i.counters["x"] != want {
					t.Fatal("wrong return counter", side, sourceZone, destination, i.counters)
				}
				s.g.execCardEffect(ir.CardEffect{Kind: "transform", Target: ir.SelfRef{Kind: "self"}, CardID: 10131320}, i, nil)
				if i.counters["x"] != 2 {
					t.Fatal("transform did not initialize new identity")
				}
				fresh := s.g.newInstance(i.card, strings.Repeat("3", 32), "fresh", "hand")
				i.counters["x"] = 9
				if fresh.counters["x"] != 2 || i.card.Counters["x"] != 2 {
					t.Fatal("instances share counters")
				}
			}
		}
	}
}

func TestCounterSpellboostBothPlayers(t *testing.T) {
	pack := repeatCardPack(t)
	for _, side := range []string{"own", "oppo"} {
		state := testState()
		state.Turn.Active = side
		actor := state.Players[side]
		actor.PP, actor.MaxPP = 1, 1
		id, held, drawn, enemy := strings.Repeat("1", 32), strings.Repeat("2", 32), strings.Repeat("3", 32), strings.Repeat("4", 32)
		actor = withInstance(actor, "hand", ir.TestInstance{InstanceID: id, CardID: 10031310, DeclaredType: "spell"})
		actor = withInstance(actor, "hand", ir.TestInstance{InstanceID: held, CardID: 10131320, DeclaredType: "spell"})
		state.Players[side] = withInstance(actor, "deck", ir.TestInstance{InstanceID: drawn, CardID: 10131320, DeclaredType: "spell"})
		state.Players[oppositeSide(side)] = withInstance(state.Players[oppositeSide(side)], "hand", ir.TestInstance{InstanceID: enemy, CardID: 10131320, DeclaredType: "spell"})
		s, err := NewSession(pack, state, 1)
		if err != nil {
			t.Fatal(err)
		}
		if result := s.SubmitAs(strings.Repeat("a", 32), side, SimulatorCommand{Kind: "play", Source: id}); result.Status != StatusCompleted {
			t.Fatal(result)
		}
		if s.g.instances[held].counters["x"] != 3 || s.g.instances[drawn].counters["x"] != 2 || s.g.instances[enemy].counters["x"] != 2 {
			t.Fatal("wrong spellboost recipients", side)
		}
	}
}

func TestCounterConditionAndRepeatReadCurrentInstance(t *testing.T) {
	read := ir.Scalar{Kind: "self_counter", Field: "life"}
	card := ir.Card{ID: 77772001, CardType: "spell", Counters: map[string]int{"life": 2}, PlayEffects: []ir.Effect{
		ir.IfEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("b", 32)}, Kind: "if", Condition: ir.CompareCondition{Kind: "compare", Left: read, Op: "eq", Right: 2}, Then: []ir.Effect{
			ir.RepeatEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("c", 32)}, Kind: "repeat", TimesExpr: &read, Body: []ir.Effect{
				ir.AdjustEffect{Kind: "adjust_counter", Field: "life", Delta: 1},
				ir.TargetEffect{Kind: "damage", Target: ir.LeaderRef{Kind: "leader", Side: "oppo"}, AmountExpr: &read},
			}},
		}},
	}}
	for _, side := range []string{"own", "oppo"} {
		state := testState()
		state.Turn.Active = side
		id := strings.Repeat("1", 32)
		state.Players[side] = withInstance(state.Players[side], "hand", ir.TestInstance{InstanceID: id, CardID: card.ID, DeclaredType: "spell"})
		s, err := NewSession(&ir.CardPack{Cards: []ir.Card{card}}, state, 1)
		if err != nil {
			t.Fatal(err)
		}
		result := s.SubmitAs(strings.Repeat("a", 32), side, SimulatorCommand{Kind: "play", Source: id})
		if result.Status != StatusCompleted || s.g.player(oppositeSide(side)).leaderLife != 13 || s.g.instances[id].counters["life"] != 4 {
			t.Fatal("counter comparison, repeat count snapshot or dynamic damage failed", result)
		}
	}
}

func TestCounterOverflowTerminatesExecution(t *testing.T) {
	card := ir.Card{ID: 77772001, CardType: "spell", Counters: map[string]int{"x": ir.MaxCounterValue}, PlayEffects: []ir.Effect{
		ir.AdjustEffect{Kind: "adjust_counter", Field: "x", Delta: 1},
		ir.TargetEffect{Kind: "damage", Target: ir.LeaderRef{Kind: "leader", Side: "oppo"}, Amount: 1},
	}}
	state := testState()
	id := strings.Repeat("1", 32)
	state.Players["own"] = withInstance(state.Players["own"], "hand", ir.TestInstance{InstanceID: id, CardID: card.ID, DeclaredType: "spell"})
	s, err := NewSession(&ir.CardPack{Cards: []ir.Card{card}}, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	step := s.Submit(strings.Repeat("a", 32), SimulatorCommand{Kind: "play", Source: id})
	if step.Status != StatusFault || step.ErrorCode != "counter_overflow" || s.g.instances[id].counters["x"] != ir.MaxCounterValue || s.g.oppo.leaderLife != 20 {
		t.Fatal(step, s.g.oppo.leaderLife)
	}
	if s.Submit(strings.Repeat("b", 32), SimulatorCommand{Kind: "end_turn"}).Status != StatusFault || s.Continuation() != nil || len(s.LegalActions()) != 0 {
		t.Fatal("faulted session continued")
	}
}
