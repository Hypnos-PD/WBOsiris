package runner

import (
	"reflect"
	"strings"
	"testing"
)

func TestConcedeEndsFromEitherSeatAndRecordsReason(t *testing.T) {
	for _, actor := range []string{"own", "oppo"} {
		state := testState()
		state.Turn.Active = oppositeSide(actor)
		s, err := NewSession(repeatCardPack(t), state, 1)
		if err != nil {
			t.Fatal(err)
		}
		result := s.SubmitAs(strings.Repeat("a", 32), actor, SimulatorCommand{Kind: "concede"})
		if result.Status != StatusCompleted || !s.g.gameOver || s.g.winner != oppositeSide(actor) || len(s.g.events) != 1 || s.g.events[0].Kind != "game_ended" || s.g.events[0].Reason != "concede" {
			t.Fatalf("concede failed actor=%s: %#v events=%#v", actor, result, s.g.events)
		}
		if result := s.SubmitAs(strings.Repeat("b", 32), actor, SimulatorCommand{Kind: "concede"}); result.ErrorCode != "game_over" {
			t.Fatal("accepted a second concession")
		}
	}
}

func TestConcedeDoesNotMutateStateAfterInvalidActionID(t *testing.T) {
	state := testState()
	s, err := NewSession(repeatCardPack(t), state, 1)
	if err != nil {
		t.Fatal(err)
	}
	before := s.g.snapshot()
	if result := s.SubmitAs("bad", "own", SimulatorCommand{Kind: "concede"}); result.Status != StatusRejected {
		t.Fatal(result)
	}
	if s.g.gameOver || len(s.g.events) != 0 || !reflect.DeepEqual(before, s.g.snapshot()) {
		t.Fatal("invalid concession mutated game")
	}
}
