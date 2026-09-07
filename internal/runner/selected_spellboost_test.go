package runner

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"wbo/internal/ir"
)

func TestRainbowSelectionPrivacyAndContinuation(t *testing.T) {
	pack := repeatCardPack(t)
	for _, side := range []string{"own", "oppo"} {
		state := testState()
		state.Turn.Active = side
		p := state.Players[side]
		p.PP, p.MaxPP = 2, 2
		ids := []int{10131310, 10131320, 10032120, 10031110, 90033310}
		for n, card := range ids {
			kind := "spell"
			if n == 2 || n == 3 {
				kind = "follower"
			}
			p = withInstance(p, "hand", ir.TestInstance{InstanceID: fmt.Sprintf("%032x", n+1), CardID: card, DeclaredType: kind})
		}
		p = withInstance(p, "deck", ir.TestInstance{InstanceID: fmt.Sprintf("%032x", 6), CardID: 10131320, DeclaredType: "spell"})
		state.Players[side] = p
		s, err := NewSession(pack, state, 1)
		if err != nil {
			t.Fatal(err)
		}
		step := s.SubmitAs(strings.Repeat("a", 32), side, SimulatorCommand{Kind: "play", Source: fmt.Sprintf("%032x", 1)})
		if step.Status != StatusSuspended || len(step.Choice.Candidates) != 2 {
			t.Fatal("wrong boostable candidates", step)
		}
		for n, candidate := range step.Choice.Candidates {
			if candidate.InstanceID != fmt.Sprintf("%032x", n+2) {
				t.Fatal("offered a card without an On Spellboost ability", candidate)
			}
		}
		view, err := s.View(oppositeSide(side))
		if err != nil || view.PendingChoice != nil || len(view.Oppo.Hand) != 0 {
			t.Fatal("choice leaked opponent hand", err)
		}
		before := s.g.snapshot()
		response := ChoiceResponse{RequestID: step.Choice.RequestID, ActionID: step.Choice.ActionID, StateRevision: step.Choice.StateRevision, SelectedInstanceIDs: []string{fmt.Sprintf("%032x", 4)}}
		if bad := s.Resume(response); bad.Status != StatusRejected || !reflect.DeepEqual(before, s.g.snapshot()) {
			t.Fatal("invalid target changed state", bad)
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
		response.SelectedInstanceIDs = []string{fmt.Sprintf("%032x", 2)}
		for _, current := range []*Session{s, restored} {
			if r := current.Resume(response); r.Status != StatusCompleted {
				t.Fatal(r)
			}
			if current.g.instances[fmt.Sprintf("%032x", 2)].counters["x"] != 4 || current.g.instances[fmt.Sprintf("%032x", 3)].cost != 9 || current.g.instances[fmt.Sprintf("%032x", 6)].counters["x"] != 2 {
				t.Fatal("incorrect selected, automatic or drawn boost")
			}
		}
		if !reflect.DeepEqual(s.g.snapshot(), restored.g.snapshot()) {
			t.Fatal("restored selected boost diverged")
		}
	}
}

func TestRainbowHomeworkThresholdDoesNotRecreateCounter(t *testing.T) {
	pack := repeatCardPack(t)
	for _, initial := range []int{2, 3, 4} {
		state := testState()
		p := state.Players["own"]
		p.PP, p.MaxPP = 2, 2
		source, target := strings.Repeat("1", 32), strings.Repeat("2", 32)
		p = withInstance(p, "hand", ir.TestInstance{InstanceID: source, CardID: 10131310, DeclaredType: "spell"})
		p = withInstance(p, "hand", ir.TestInstance{InstanceID: target, CardID: 10133310, DeclaredType: "spell", Overrides: ir.InstanceOverrides{Counters: map[string]int{"x": initial}}})
		state.Players["own"] = p
		s, err := NewSession(pack, state, 1)
		if err != nil {
			t.Fatal(err)
		}
		step := s.Submit(strings.Repeat("a", 32), SimulatorCommand{Kind: "play", Source: source})
		if step.Status != StatusSuspended {
			t.Fatal(step)
		}
		if r := s.Resume(ChoiceResponse{RequestID: step.Choice.RequestID, ActionID: step.Choice.ActionID, StateRevision: step.Choice.StateRevision, SelectedInstanceIDs: []string{target}}); r.Status != StatusCompleted {
			t.Fatal(r)
		}
		i := s.g.instances[target]
		if initial == 2 {
			if i.card.ID != 10133310 || i.counters["x"] != 4 {
				t.Fatal("premature transformation")
			}
		} else if i.card.ID != 90033310 || len(i.counters) != 0 {
			t.Fatal("queued boost recreated transformed counter")
		}
	}
}

func TestAnnesSummoningExpiresForBothPlayers(t *testing.T) {
	pack := repeatCardPack(t)
	for _, side := range []string{"own", "oppo"} {
		state := testState()
		state.Turn.Active = side
		source, homework := strings.Repeat("1", 32), strings.Repeat("2", 32)
		p := state.Players[side]
		p.PP, p.MaxPP = 5, 5
		p = withInstance(p, "hand", ir.TestInstance{InstanceID: source, CardID: 10134120, DeclaredType: "follower"})
		p = withInstance(p, "hand", ir.TestInstance{InstanceID: homework, CardID: 10133310, DeclaredType: "spell", Overrides: ir.InstanceOverrides{Counters: map[string]int{"x": 3}}})
		state.Players[side] = p
		s, err := NewSession(pack, state, 1)
		if err != nil {
			t.Fatal(err)
		}
		if r := s.SubmitAs(strings.Repeat("a", 32), side, SimulatorCommand{Kind: "play", Source: source}); r.Status != StatusCompleted {
			t.Fatal(r)
		}
		if i := s.g.instances[homework]; i.card.ID != 90033310 || len(i.counters) != 0 {
			t.Fatal("three boosts did not safely transform Homework")
		}
		token := s.g.player(side).field[1]
		if token.card.ID != 90034130 || !token.abilities["ward"] || !token.abilities["rush"] || token.attack != 5 || token.life != 5 {
			t.Fatal("incorrect Summoning")
		}
		if r := s.SubmitAs(strings.Repeat("b", 32), side, SimulatorCommand{Kind: "end_turn"}); r.Status != StatusCompleted || token.zone != "field" {
			t.Fatal("Summoning left at its controller turn end", r)
		}
		if r := s.SubmitAs(strings.Repeat("c", 32), oppositeSide(side), SimulatorCommand{Kind: "end_turn"}); r.Status != StatusCompleted || token.zone != "graveyard" {
			t.Fatal("Summoning survived opponent turn end", r)
		}
	}
}
