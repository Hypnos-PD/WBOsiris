package runner

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"wbo/internal/ir"
)

func TestHomeworkThresholdAndTransformedPlay(t *testing.T) {
	pack := repeatCardPack(t)
	for _, side := range []string{"own", "oppo"} {
		for _, initial := range []int{0, 2, 3, 4, 5} {
			t.Run(fmt.Sprintf("%s/X%d", side, initial), func(t *testing.T) {
				state := testState()
				state.Turn.Active, state.Turn.Number = side, 6
				id, williamID := strings.Repeat("1", 32), strings.Repeat("2", 32)
				actor := state.Players[side]
				actor.PP, actor.MaxPP, actor.EP = 1, 6, 1
				zero := 0
				actor = withInstance(actor, "hand", ir.TestInstance{InstanceID: id, CardID: 10133310, DeclaredType: "spell", Overrides: ir.InstanceOverrides{Counters: map[string]int{"x": initial}, Cost: &zero}})
				actor = withInstance(actor, "field", ir.TestInstance{InstanceID: williamID, CardID: 10132130, DeclaredType: "follower"})
				for n := 3; n < 5; n++ {
					actor = withInstance(actor, "deck", ir.TestInstance{InstanceID: fmt.Sprintf("%032x", n), CardID: 10133310, DeclaredType: "spell"})
				}
				state.Players[side] = actor
				s, err := NewSession(pack, state, 1)
				if err != nil {
					t.Fatal(err)
				}
				original := s.g.instances[id]
				step := s.SubmitAs(strings.Repeat("a", 32), side, SimulatorCommand{Kind: "evolve", Source: williamID})
				if step.Status != StatusCompleted {
					t.Fatal(step)
				}
				item := s.g.instances[id]
				if item != original || item.zone != "hand" || s.g.player(side).hand[0] != item {
					t.Fatal("transform lost identity or hand order")
				}
				transforms := 0
				for _, event := range s.Events() {
					if event.Kind == "card_transformed" {
						transforms++
						if event.PrivateTo != side || event.Subject.CardID != 10133310 || event.Target.CardID != 90033310 {
							t.Fatal(event)
						}
					}
				}
				if initial < 3 {
					if transforms != 0 || item.counters["x"] != initial+2 || item.cost != 0 {
						t.Fatal("premature transformation")
					}
					return
				}
				if transforms != 1 || item.card.ID != 90033310 || len(item.counters) != 0 || item.cost != 1 {
					t.Fatal("threshold did not reset identity exactly once")
				}
				step = s.SubmitAs(strings.Repeat("b", 32), side, SimulatorCommand{Kind: "play", Source: id})
				if step.Status != StatusCompleted || s.g.player(side).pp != 0 || item.zone != "graveyard" || len(s.g.player(side).hand) != 2 {
					t.Fatal("transformed card could not be played", step)
				}
				for _, drawn := range s.g.player(side).hand {
					if drawn.counters["x"] != 0 {
						t.Fatal("drawn card received retroactive boost")
					}
				}
			})
		}
	}
}

func TestHomeworkContinuationAfterTransformWithQueuedBoost(t *testing.T) {
	pack := repeatCardPack(t)
	for n := range pack.Cards {
		if pack.Cards[n].ID != 10133310 {
			continue
		}
		ability := &pack.Cards[n].Abilities[0]
		condition := ability.Body[1].(ir.IfEffect)
		condition.Then = append(condition.Then, ir.ModeEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("f", 32)}, Kind: "mode", Options: []ir.ModeOption{{ID: 1}, {ID: 2}}})
		ability.Body[1] = condition
	}
	for _, side := range []string{"own", "oppo"} {
		state := testState()
		state.Turn.Active, state.Turn.Number = side, 6
		id, williamID := strings.Repeat("1", 32), strings.Repeat("2", 32)
		actor := state.Players[side]
		actor.EP = 1
		actor = withInstance(actor, "hand", ir.TestInstance{InstanceID: id, CardID: 10133310, DeclaredType: "spell", Overrides: ir.InstanceOverrides{Counters: map[string]int{"x": 4}}})
		state.Players[side] = withInstance(actor, "field", ir.TestInstance{InstanceID: williamID, CardID: 10132130, DeclaredType: "follower"})
		s, err := NewSession(pack, state, 1)
		if err != nil {
			t.Fatal(err)
		}
		step := s.SubmitAs(strings.Repeat("a", 32), side, SimulatorCommand{Kind: "evolve", Source: williamID})
		if step.Status != StatusSuspended || s.g.instances[id].card.ID != 90033310 || len(s.g.triggers) != 1 {
			t.Fatal("missing transformed checkpoint", step)
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
			view, _ := current.View(oppositeSide(side))
			if view.PendingChoice != nil || len(view.Oppo.Hand) != 0 {
				t.Fatal("hidden transformed choice leaked")
			}
			response := ChoiceResponse{RequestID: step.Choice.RequestID, ActionID: step.Choice.ActionID, StateRevision: step.Choice.StateRevision, SelectedOptionID: 1}
			if result := current.Resume(response); result.Status != StatusCompleted {
				t.Fatal("queued old boost transformed again", result)
			}
			if len(current.g.instances[id].counters) != 0 {
				t.Fatal("old queued boost recreated X")
			}
			for _, event := range current.EventsFor(oppositeSide(side)) {
				if event.Kind == "card_transformed" && (event.Subject != nil || event.Target != nil) {
					t.Fatal("transform identity leaked")
				}
			}
		}
		if !reflect.DeepEqual(s.g.snapshot(), restored.g.snapshot()) {
			t.Fatal("restored transformation diverged")
		}
	}
}

func TestHomeworkTransformsAfterReturnToDeck(t *testing.T) {
	pack := repeatCardPack(t)
	for _, side := range []string{"own", "oppo"} {
		seen := map[string]bool{}
		for seed := uint64(1); seed <= 12; seed++ {
			state := testState()
			state.Turn.Active = side
			id, sourceID := strings.Repeat("1", 32), strings.Repeat("2", 32)
			zero := 0
			actor := state.Players[side]
			actor.PP, actor.MaxPP = 0, 8
			actor = withInstance(actor, "hand", ir.TestInstance{InstanceID: id, CardID: 10133310, DeclaredType: "spell", Overrides: ir.InstanceOverrides{Counters: map[string]int{"x": 4}}})
			actor = withInstance(actor, "hand", ir.TestInstance{InstanceID: sourceID, CardID: 10134310, DeclaredType: "spell", Overrides: ir.InstanceOverrides{Cost: &zero}})
			for n := 3; n < 11; n++ {
				actor = withInstance(actor, "deck", ir.TestInstance{InstanceID: fmt.Sprintf("%032x", n), CardID: 10001110, DeclaredType: "follower"})
			}
			state.Players[side] = actor
			s, err := NewSession(pack, state, seed)
			if err != nil {
				t.Fatal(err)
			}
			if result := s.SubmitAs(strings.Repeat("a", 32), side, SimulatorCommand{Kind: "play", Source: sourceID}); result.Status != StatusCompleted {
				t.Fatal(result)
			}
			item := s.g.instances[id]
			seen[item.zone] = true
			if item.card.ID != 90033310 || item.cost != 1 || len(item.counters) != 0 {
				t.Fatal("returned card lost queued transformation")
			}
			transforms := 0
			for _, event := range s.Events() {
				if event.Kind == "card_transformed" {
					transforms++
					if event.From != item.zone || event.PrivateTo != side {
						t.Fatal(event)
					}
				}
			}
			if transforms != 1 {
				t.Fatal("repeated boost transformed multiple times")
			}
		}
		if !seen["hand"] || !seen["deck"] {
			t.Fatal("missing redraw or deck coverage")
		}
	}
}
