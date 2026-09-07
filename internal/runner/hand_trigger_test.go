package runner

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"wbo/internal/ir"
)

func TestBayleOnlyListensFromHandToAlliedFollowerDepartures(t *testing.T) {
	pack := repeatCardPack(t)
	for _, side := range []string{"own", "oppo"} {
		for _, departure := range []string{"destroy", "hand", "deck", "banished", "transform", "amulet", "enemy"} {
			t.Run(side+"/"+departure, func(t *testing.T) {
				state := testState()
				state.Turn.Active = side
				for n, zone := range []string{"hand", "hand", "field", "deck"} {
					state.Players[side] = withInstance(state.Players[side], zone, ir.TestInstance{InstanceID: fmt.Sprintf("%032x", n+1), CardID: 10113130, DeclaredType: "follower"})
				}
				target, other := strings.Repeat("a", 32), oppositeSide(side)
				targetSide, targetCard, targetType := side, 10001110, "follower"
				if departure == "enemy" {
					targetSide = other
				}
				if departure == "amulet" {
					targetCard, targetType = 10173210, "amulet"
				}
				state.Players[targetSide] = withInstance(state.Players[targetSide], "field", ir.TestInstance{InstanceID: target, CardID: targetCard, DeclaredType: targetType})
				state.Players[other] = withInstance(state.Players[other], "hand", ir.TestInstance{InstanceID: strings.Repeat("b", 32), CardID: 10113130, DeclaredType: "follower"})
				s, err := NewSession(pack, state, 1)
				if err != nil {
					t.Fatal(err)
				}
				i := s.g.instances[target]
				switch departure {
				case "destroy", "enemy", "amulet":
					s.g.destroyByEffect([]*instance{i})
				case "transform":
					s.g.execCardEffect(ir.CardEffect{Kind: "transform", Target: ir.SelfRef{Kind: "self"}, CardID: 10001110}, i, nil)
				case "hand", "deck":
					s.g.returnCard(i, departure)
				default:
					s.g.move(i, departure)
				}
				if r := s.run(); r.Status != StatusCompleted {
					t.Fatal(r)
				}
				want := 7
				if departure == "transform" || departure == "amulet" || departure == "enemy" {
					want = 8
				}
				for n := 1; n <= 4; n++ {
					cost := 8
					if n <= 2 {
						cost = want
					}
					if s.g.instances[fmt.Sprintf("%032x", n)].cost != cost {
						t.Fatal("wrong source zone or subject", n, departure)
					}
				}
				opposingCost := 8
				if departure == "enemy" {
					opposingCost = 7
				}
				if s.g.player(other).hand[0].cost != opposingCost {
					t.Fatal("wrong event owner")
				}
			})
		}
	}
}

func TestHandListenerIndexTracksDrawPlayDiscardAndTransform(t *testing.T) {
	pack := repeatCardPack(t)
	state := testState()
	id, ally := strings.Repeat("1", 32), strings.Repeat("2", 32)
	state.Players["own"] = withInstance(state.Players["own"], "deck", ir.TestInstance{InstanceID: id, CardID: 10113130, DeclaredType: "follower"})
	state.Players["own"] = withInstance(state.Players["own"], "field", ir.TestInstance{InstanceID: ally, CardID: 10001110, DeclaredType: "follower"})
	s, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	i := s.g.instances[id]
	s.g.draw(ir.DrawEffect{Kind: "draw", Owner: "own", SourceZone: "deck", Count: 1, Output: "drawn"}, nil, frame{})
	if len(s.g.triggerIndex.abilities("follower_left", i)) != 1 {
		t.Fatal("draw did not register hand source")
	}
	s.g.returnCard(s.g.instances[ally], "hand")
	if r := s.run(); r.Status != StatusCompleted || i.cost != 7 {
		t.Fatal("drawn Bayle missed departure", r)
	}
	s.g.own.pp = 10
	if r := s.Begin(strings.Repeat("3", 32), ir.SourceAction{Kind: "play", Actor: "own", Source: id}); r.Status != StatusCompleted {
		t.Fatal(r)
	}
	if len(s.g.triggerIndex.abilities("follower_left", i)) != 0 {
		t.Fatal("played Bayle still listens")
	}
	s.g.returnCard(i, "hand")
	if r := s.run(); r.Status != StatusCompleted || i.cost != 8 {
		t.Fatal("return did not reset source", r)
	}
	for n := 0; n < 10; n++ {
		s.g.move(s.g.instances[ally], "field")
		s.g.returnCard(s.g.instances[ally], "hand")
		if r := s.run(); r.Status != StatusCompleted {
			t.Fatal(r)
		}
	}
	if i.cost != 0 {
		t.Fatal("reduction floor", i.cost)
	}
	s.g.execCardEffect(ir.CardEffect{Kind: "transform", Target: ir.SelfRef{Kind: "self"}, CardID: 10001110}, i, nil)
	if len(s.g.triggerIndex.abilities("follower_left", i)) != 0 {
		t.Fatal("transform retained old listener")
	}
	s.g.execCardEffect(ir.CardEffect{Kind: "transform", Target: ir.SelfRef{Kind: "self"}, CardID: 10113130}, i, nil)
	if len(s.g.triggerIndex.abilities("follower_left", i)) != 1 {
		t.Fatal("transform failed to register listener")
	}
	s.g.discardCards([]*instance{i})
	if len(s.g.triggerIndex.abilities("follower_left", i)) != 0 {
		t.Fatal("discard retained listener")
	}
}

func TestQueuedHandListenerRestoresAndCancelsOnLeavingHand(t *testing.T) {
	for _, cancel := range []bool{false, true} {
		pack := repeatCardPack(t)
		body := []ir.Effect{ir.TargetEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("a", 32)}, Kind: "return", Destination: "hand", Target: ir.ZoneRef{Kind: "zone", Side: "own", Zone: "field", Member: "follower"}}}
		if cancel {
			body = append(body, ir.TargetEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("b", 32)}, Kind: "discard", Target: ir.ZoneRef{Kind: "zone", Side: "own", Zone: "hand", Member: "follower"}})
		}
		body = append(body, ir.ModeEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("c", 32)}, Kind: "mode", Options: []ir.ModeOption{{ID: 1, Body: []ir.Effect{}}}})
		pack.Cards = append(pack.Cards, ir.Card{ID: 88880004, CardType: "spell", PlayEffects: body})
		state := testState()
		spell, bayle := strings.Repeat("1", 32), strings.Repeat("2", 32)
		state.Players["own"] = withInstance(state.Players["own"], "hand", ir.TestInstance{InstanceID: spell, CardID: 88880004, DeclaredType: "spell"})
		state.Players["own"] = withInstance(state.Players["own"], "hand", ir.TestInstance{InstanceID: bayle, CardID: 10113130, DeclaredType: "follower"})
		for n := 0; n < 2; n++ {
			state.Players["own"] = withInstance(state.Players["own"], "field", ir.TestInstance{InstanceID: fmt.Sprintf("%032x", n+3), CardID: 10001110, DeclaredType: "follower"})
		}
		s, err := NewSession(pack, state, 1)
		if err != nil {
			t.Fatal(err)
		}
		step := s.Begin(strings.Repeat("d", 32), ir.SourceAction{Kind: "play", Actor: "own", Source: spell})
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
		response := ChoiceResponse{RequestID: step.Choice.RequestID, ActionID: step.Choice.ActionID, StateRevision: step.Choice.StateRevision, SelectedOptionID: 1}
		want := 6
		if cancel {
			want = 8
		}
		for _, current := range []*Session{s, restored} {
			if r := current.Resume(response); r.Status != StatusCompleted || current.g.instances[bayle].cost != want {
				t.Fatal("pending hand trigger diverged", r, current.g.instances[bayle].cost)
			}
		}
		if !reflect.DeepEqual(s.g.snapshot(), restored.g.snapshot()) {
			t.Fatal("restore diverged")
		}
	}
}

func TestAncientCannonFiresOncePerFusionAfterSourceAbilityAndRestores(t *testing.T) {
	pack := repeatCardPack(t)
	for n := range pack.Cards {
		if pack.Cards[n].ID == 90071210 {
			fusion := &pack.Cards[n].FusionAbilities[0]
			fusion.Body = append([]ir.Effect{ir.ModeEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("e", 32)}, Kind: "mode", Options: []ir.ModeOption{{ID: 1, Body: []ir.Effect{}}}}}, fusion.Body...)
		}
	}
	for _, side := range []string{"own", "oppo"} {
		state := testState()
		state.Turn.Active = side
		source := strings.Repeat("1", 32)
		state.Players[side] = withInstance(state.Players[side], "hand", ir.TestInstance{InstanceID: source, CardID: 90071210, DeclaredType: "amulet"})
		materials := []string{strings.Repeat("2", 32), strings.Repeat("3", 32)}
		for n, id := range materials {
			state.Players[side] = withInstance(state.Players[side], "hand", ir.TestInstance{InstanceID: id, CardID: 90071220, DeclaredType: "amulet"})
			state.Players[side] = withInstance(state.Players[side], "field", ir.TestInstance{InstanceID: fmt.Sprintf("%032x", n+10), CardID: 10173210, DeclaredType: "amulet"})
		}
		other := oppositeSide(side)
		state.Players[other] = withInstance(state.Players[other], "field", ir.TestInstance{InstanceID: strings.Repeat("4", 32), CardID: 10173210, DeclaredType: "amulet"})
		for n := 0; n < 2; n++ {
			state.Players[other] = withInstance(state.Players[other], "field", ir.TestInstance{InstanceID: fmt.Sprintf("%032x", n+20), CardID: 10113130, DeclaredType: "follower", Overrides: ir.InstanceOverrides{Stats: &ir.Stats{Attack: 4, Life: 6}}})
		}
		s, err := NewSession(pack, state, 7)
		if err != nil {
			t.Fatal(err)
		}
		step := s.SubmitAs(strings.Repeat("a", 32), side, SimulatorCommand{Kind: "fusion", Source: source})
		if step.Status != StatusSuspended {
			t.Fatal(step)
		}
		step = s.Resume(ChoiceResponse{RequestID: step.Choice.RequestID, ActionID: step.Choice.ActionID, StateRevision: step.Choice.StateRevision, SelectedInstanceIDs: materials})
		if step.Status != StatusSuspended || step.Choice.Kind != "mode" || len(s.g.triggers) != 2 {
			t.Fatal("fusion did not queue exactly two cannons", step)
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
		response := ChoiceResponse{RequestID: step.Choice.RequestID, ActionID: step.Choice.ActionID, StateRevision: step.Choice.StateRevision, SelectedOptionID: 1}
		for _, current := range []*Session{s, restored} {
			if r := current.Resume(response); r.Status != StatusCompleted {
				t.Fatal(r)
			}
			totalLife := 0
			for _, i := range current.g.player(other).field {
				if i.card.CardType == "follower" {
					totalLife += i.life
				}
			}
			if totalLife != 8 || current.g.instances[source].card.ID != 90072110 || current.g.rng.Consumed() != 2 {
				t.Fatal("fusion trigger count or RNG", totalLife, current.g.rng.Consumed())
			}
			transformed := false
			for _, event := range current.EventsFor(other) {
				if event.Kind == "card_transformed" {
					transformed = true
				}
				if event.Kind == "damaged" && !transformed {
					t.Fatal("listener ran before source ability")
				}
				if event.Kind == "card_fused" || event.Kind == "card_transformed" {
					if event.Subject != nil {
						t.Fatal("hidden fusion identity leaked")
					}
				}
			}
		}
		if !reflect.DeepEqual(s.g.snapshot(), restored.g.snapshot()) {
			t.Fatal("restored fusion listeners diverged")
		}
	}
}
