package runner

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"wbo/internal/ir"
)

func extraPPSession(t *testing.T, turn int, costs ...int) *Session {
	t.Helper()
	state := testState()
	state.FirstPlayer = "own"
	state.Turn = ir.Turn{Active: "oppo", Number: turn}
	actor := state.Players["oppo"]
	actor.PP, actor.MaxPP = turn, turn
	actor.ExtraPPEarly, actor.ExtraPPLate = true, true
	pack := &ir.CardPack{Cards: []ir.Card{{ID: 77775001, CardType: "spell"}}}
	for n, cost := range costs {
		actor = withInstance(actor, "hand", ir.TestInstance{InstanceID: fmt.Sprintf("%032x", n+1), CardID: 77775001, DeclaredType: "spell", Overrides: ir.InstanceOverrides{Cost: &cost}})
	}
	state.Players["oppo"] = actor
	for _, side := range []string{"own", "oppo"} {
		player := state.Players[side]
		for n := 0; n < 20; n++ {
			player = withInstance(player, "deck", ir.TestInstance{InstanceID: fmt.Sprintf("%s%031x", map[string]string{"own": "a", "oppo": "b"}[side], n+1), CardID: 77775001, DeclaredType: "spell"})
		}
		state.Players[side] = player
	}
	s, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func extraPPCommand(t *testing.T, s *Session, kind string, source int) StepResult {
	t.Helper()
	command := SimulatorCommand{Kind: kind}
	if source > 0 {
		command.Source = fmt.Sprintf("%032x", source)
	}
	result := s.SubmitAs(fmt.Sprintf("%032x", s.g.revision+1000), s.g.turn.Active, command)
	if result.Status != StatusCompleted {
		t.Fatalf("%s failed: %#v", kind, result)
	}
	return result
}

func extraPPNextRound(t *testing.T, s *Session) {
	t.Helper()
	extraPPCommand(t, s, "end_turn", 0)
	extraPPCommand(t, s, "end_turn", 0)
}

func assertExtraPP(t *testing.T, s *Session, available bool, uses int) {
	t.Helper()
	for _, viewer := range []string{"own", "oppo"} {
		view, err := s.View(viewer)
		if err != nil {
			t.Fatal(err)
		}
		second, first := view.Oppo, view.Own
		if viewer == "oppo" {
			second, first = view.Own, view.Oppo
		}
		if second.ExtraPPAvailable != available || second.ExtraPPUses != uses || first.ExtraPPAvailable || first.ExtraPPUses != 0 {
			t.Fatalf("wrong extra PP view for %s: second=%#v first=%#v", viewer, second, first)
		}
	}
	legal := false
	for _, action := range s.LegalActionsFor("oppo") {
		legal = legal || action.Kind == "use_extra_pp"
	}
	if legal != available {
		t.Fatal("availability and legal actions disagree")
	}
	if !available {
		before := s.g.snapshot()
		result := s.SubmitAs(strings.Repeat("e", 32), "oppo", SimulatorCommand{Kind: "use_extra_pp"})
		if result.Status != StatusIllegal || !reflect.DeepEqual(before, s.g.snapshot()) {
			t.Fatal("unavailable extra PP mutated state", result)
		}
	}
}

func TestExtraPPIsConsumedAtMostTwiceAcrossTurns(t *testing.T) {
	s := extraPPSession(t, 3, 4, 7)
	assertExtraPP(t, s, true, 1)
	extraPPCommand(t, s, "use_extra_pp", 0)
	extraPPCommand(t, s, "play", 1)
	assertExtraPP(t, s, false, 0)
	for turn := 4; turn <= 5; turn++ {
		extraPPNextRound(t, s)
		assertExtraPP(t, s, false, 0)
	}
	extraPPNextRound(t, s)
	assertExtraPP(t, s, true, 1)
	extraPPCommand(t, s, "use_extra_pp", 0)
	extraPPCommand(t, s, "play", 2)
	assertExtraPP(t, s, false, 0)
	for turn := 7; turn <= 10; turn++ {
		extraPPNextRound(t, s)
		assertExtraPP(t, s, false, 0)
	}
}

func TestExtraPPFirstUseAfterFifthTurnCannotRefresh(t *testing.T) {
	for _, turn := range []int{6, 8} {
		t.Run(fmt.Sprint(turn), func(t *testing.T) {
			s := extraPPSession(t, turn, turn+1)
			assertExtraPP(t, s, true, 1)
			extraPPCommand(t, s, "use_extra_pp", 0)
			extraPPCommand(t, s, "play", 1)
			if s.g.oppo.pp != 0 || s.g.oppo.maxpp != turn {
				t.Fatal("extra PP changed the maximum or was not spent")
			}
			assertExtraPP(t, s, false, 0)
			extraPPNextRound(t, s)
			assertExtraPP(t, s, false, 0)
		})
	}
}

func TestExtraPPUnusedAtFifthTurnDoesNotStackAtSixth(t *testing.T) {
	s := extraPPSession(t, 5, 1, 0, 7)
	extraPPCommand(t, s, "use_extra_pp", 0)
	extraPPCommand(t, s, "play", 1)
	extraPPCommand(t, s, "play", 2)
	if s.g.oppo.pp != 5 || !s.g.oppo.extraPPActive {
		t.Fatal("base PP payment consumed the extra point")
	}
	extraPPCommand(t, s, "end_turn", 0)
	if s.g.oppo.pp != 4 || s.g.oppo.extraPPActive {
		t.Fatal("unused extra point was not removed")
	}
	extraPPCommand(t, s, "end_turn", 0)
	assertExtraPP(t, s, true, 1)
	extraPPCommand(t, s, "use_extra_pp", 0)
	extraPPCommand(t, s, "play", 3)
	assertExtraPP(t, s, false, 0)
	extraPPNextRound(t, s)
	assertExtraPP(t, s, false, 0)
}

func TestPPGainPreservesExistingExtraPoint(t *testing.T) {
	for _, owner := range []string{"own", "oppo"} {
		for _, current := range []int{0, 4, 5, 6} {
			for _, amount := range []int{0, 1, 5} {
				s := extraPPSession(t, 5)
				player := s.g.player(owner)
				player.pp, player.maxpp = current, 5
				s.g.execAdjust(ir.AdjustEffect{Kind: "adjust_resource", Owner: owner, Resource: "pp", Delta: amount}, nil, nil)
				if want := max(current, min(5, current+amount)); player.pp != want || player.maxpp != 5 {
					t.Fatalf("%s %d + %d: got %d/5, want %d/5", owner, current, amount, player.pp, want)
				}
			}
		}
	}
}

func TestExtraPPContinuationPreservesPaymentAndRestoration(t *testing.T) {
	for _, cost := range []int{1, 7} {
		t.Run(fmt.Sprint(cost), func(t *testing.T) {
			pack := &ir.CardPack{Cards: []ir.Card{{ID: 77775001, CardType: "spell", Cost: cost, PlayEffects: []ir.Effect{
				ir.ModeEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("a", 32)}, Kind: "mode", Options: []ir.ModeOption{{ID: 1}}},
				ir.AdjustEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("b", 32)}, Kind: "restore_resource", Owner: "own", Resource: "pp"},
			}}}}
			state := testState()
			state.FirstPlayer = "own"
			state.Turn = ir.Turn{Active: "oppo", Number: 6}
			for _, side := range []string{"own", "oppo"} {
				player := state.Players[side]
				player.PP, player.MaxPP = 6, 6
				player.ExtraPPEarly, player.ExtraPPLate = side == "oppo", side == "oppo"
				player = withInstance(player, "deck", ir.TestInstance{InstanceID: map[string]string{"own": strings.Repeat("2", 32), "oppo": strings.Repeat("3", 32)}[side], CardID: 77775001, DeclaredType: "spell"})
				state.Players[side] = player
			}
			state.Players["oppo"] = withInstance(state.Players["oppo"], "hand", ir.TestInstance{InstanceID: strings.Repeat("1", 32), CardID: 77775001, DeclaredType: "spell"})
			s, err := NewSession(pack, state, 1)
			if err != nil {
				t.Fatal(err)
			}
			extraPPCommand(t, s, "use_extra_pp", 0)
			step := s.SubmitAs(strings.Repeat("4", 32), "oppo", SimulatorCommand{Kind: "play", Source: strings.Repeat("1", 32)})
			if step.Status != StatusSuspended || s.g.oppo.extraPPActive != (cost == 1) {
				t.Fatal("payment did not preserve pending or consumed state", step)
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
			if result, other := s.Resume(answer), restored.Resume(answer); result.Status != StatusCompleted || !reflect.DeepEqual(result, other) || !reflect.DeepEqual(s.g.snapshot(), restored.g.snapshot()) {
				t.Fatal("restored PP payment diverged", result, other)
			}
			for _, session := range []*Session{s, restored} {
				if session.g.oppo.pp != 6 {
					t.Fatal("resumed restoration lost PP")
				}
				extraPPCommand(t, session, "end_turn", 0)
				want := 6
				if cost == 1 {
					want = 5
				}
				if session.g.oppo.pp != want {
					t.Fatal("end turn refunded consumed PP or kept unused PP")
				}
				extraPPCommand(t, session, "end_turn", 0)
				uses := 0
				if cost == 1 {
					uses = 1
				}
				assertExtraPP(t, session, cost == 1, uses)
			}
		})
	}
}
