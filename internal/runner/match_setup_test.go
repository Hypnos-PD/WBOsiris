package runner

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"wbo/internal/ir"
)

func matchSetupDeck(pack *ir.CardPack) []int {
	var ids []int
	for n := range pack.Cards {
		card := &pack.Cards[n]
		if MatchCardUnavailableReason(card) == "" && (card.Meta.Class == "neutral" || card.Meta.Class == "forestcraft") {
			ids = append(ids, card.ID)
			if len(ids) == 14 {
				break
			}
		}
	}
	deck := make([]int, 40)
	for n := range deck {
		deck[n] = ids[n%len(ids)]
	}
	return deck
}

func TestMatchFirstPlayerDestructionHistoryRestoresInEitherSeat(t *testing.T) {
	for _, first := range []string{"own", "oppo"} {
		for _, active := range []string{first, oppositeSide(first)} {
			for _, destroyedDuring := range []string{first, oppositeSide(first)} {
				s, _, _, victim := historySession(t, destroyedDuring, 1)
				s.g.firstPlayer = first
				s.g.destroyByEffect([]*instance{s.g.instances[victim]})
				s.g.turn.Active = active
				saved := snapshotContinuationGame(s.g)
				_, err := restoreGame(s.g.cards, saved)
				future := active == first && destroyedDuring != first
				if (err != nil) != future {
					t.Fatalf("first=%s active=%s destroyed=%s: future=%v err=%v", first, active, destroyedDuring, future, err)
				}
				// Either player's record is valid once the next full round begins.
				saved.Turn.Number++
				if _, err := restoreGame(s.g.cards, saved); err != nil {
					t.Fatal("rejected previous-round destruction", err)
				}
			}
		}
	}
}

func TestMatchFirstPlayerRoundBoundarySurvivesContinuation(t *testing.T) {
	pack := repeatCardPack(t)
	pack.Cards = append(pack.Cards, ir.Card{ID: 77779001, CardType: "follower", Stats: &ir.Stats{Attack: 1, Life: 1}, Abilities: []ir.Ability{{
		ID: strings.Repeat("d", 32), Trigger: ir.EventTrigger{Kind: "event", Event: "turn_ended", Side: "own"},
		Body: []ir.Effect{ir.ModeEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("e", 32)}, Kind: "mode", Options: []ir.ModeOption{{ID: 1, Body: []ir.Effect{}}}}},
	}}})
	for _, first := range []string{"own", "oppo"} {
		second := oppositeSide(first)
		state := testState()
		state.FirstPlayer, state.Turn.Active = first, second
		state.Players[second] = withInstance(state.Players[second], "field", ir.TestInstance{InstanceID: strings.Repeat("1", 32), CardID: 77779001, DeclaredType: "follower"})
		state.Players[first] = withInstance(state.Players[first], "deck", ir.TestInstance{InstanceID: strings.Repeat("2", 32), CardID: 10001110, DeclaredType: "follower"})
		s, err := NewSession(pack, state, 1)
		if err != nil {
			t.Fatal(err)
		}
		step := s.SubmitAs(strings.Repeat("a", 32), second, SimulatorCommand{Kind: "end_turn"})
		if step.Status != StatusSuspended || s.g.turn.Number != 1 {
			t.Fatal("end-turn choice skipped its original round", step)
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
			if r := current.Resume(response); r.Status != StatusCompleted || current.g.turn.Active != first || current.g.turn.Number != 2 {
				t.Fatal("resumed the wrong round or seat", r)
			}
		}
		if !reflect.DeepEqual(s.g.snapshot(), restored.g.snapshot()) {
			t.Fatal("restored round boundary differs")
		}
		old, _ := DecodeContinuation(data)
		old.Version = "0.34.0"
		if _, err := RestoreSession(pack, old); err == nil {
			t.Fatal("accepted old round-count semantics")
		}
	}
}

func TestMatchSetupBothFirstPlayersAndDeterministicSeed(t *testing.T) {
	pack := repeatCardPack(t)
	deck := matchSetupDeck(pack)
	for _, first := range []string{"own", "oppo"} {
		s, err := NewMatchSessionWithFirstPlayer(pack, deck, deck, 12345, first)
		if err != nil {
			t.Fatal(err)
		}
		repeated, err := NewMatchSessionWithFirstPlayer(pack, deck, deck, 12345, first)
		if err != nil || !reflect.DeepEqual(s.g.snapshot(), repeated.g.snapshot()) {
			t.Fatal("same seed and first player did not reproduce opening", err)
		}
		other, err := NewMatchSessionWithFirstPlayer(pack, deck, deck, 54321, first)
		if err != nil || reflect.DeepEqual(instanceIDs(s.g.own.hand), instanceIDs(other.g.own.hand)) {
			t.Fatal("different seed did not change the known opening fixture", err)
		}
		for _, side := range []string{"own", "oppo"} {
			view, err := s.View(side)
			wantFirst := "oppo"
			if side == first {
				wantFirst = "own"
			}
			if err != nil || view.FirstPlayer != wantFirst || view.Turn.Active != wantFirst || view.Turn.Number != 1 || len(view.Own.Hand) != 4 || view.Own.DeckCount != 36 || view.Oppo.Hand != nil || view.Oppo.HandCount != 4 || view.Own.ExtraPPAvailable != (side != first) {
				t.Fatal("opening or normalized seat is incorrect", first, side, view, err)
			}
			wantPP := 0
			if side == first {
				wantPP = 1
			}
			if view.Own.PP != wantPP || view.Own.MaxPP != wantPP {
				t.Fatal("PP followed host identity instead of first player")
			}
			if err := s.Mulligan(side, nil); err != nil {
				t.Fatal(err)
			}
		}
		if r := s.StartMatch(); r.Status != StatusCompleted {
			t.Fatal(r)
		}
		if len(s.g.player(first).hand) != 5 || len(s.g.player(oppositeSide(first)).hand) != 4 {
			t.Fatal("initial turn draw went to the wrong seat")
		}
		for n := 0; n < 12; n++ {
			side := s.g.turn.Active
			round := s.g.turn.Number
			if r := s.SubmitAs(fmt.Sprintf("%032x", n+1), side, SimulatorCommand{Kind: "end_turn"}); r.Status != StatusCompleted {
				t.Fatal(r)
			}
			if side != first {
				round++
			}
			if s.g.turn.Number != round || s.g.player(s.g.turn.Active).pp != round {
				t.Fatal("round or PP advanced between the first and second player", first, n)
			}
			for _, candidate := range []string{"own", "oppo"} {
				threshold := 5
				if candidate != first {
					threshold = 4
				}
				if s.g.evolutionUnlocked(candidate, false) != (round >= threshold) || s.g.evolutionUnlocked(candidate, true) != (round >= threshold+2) {
					t.Fatal("evolution unlock followed host identity")
				}
			}
		}
	}
	if _, err := NewMatchSessionWithFirstPlayer(pack, deck, deck, 1, "guest"); err == nil {
		t.Fatal("invalid first player accepted")
	}
}
