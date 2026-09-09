package runner

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"wbo/internal/ir"
)

func TestReposeOfficialNegativeAttackAndGraceContinuation(t *testing.T) {
	pack := repeatCardPack(t)
	for _, side := range []string{"own", "oppo"} {
		state := crestState(side)
		state.FirstPlayer = side
		follower, shrine := strings.Repeat("1", 32), strings.Repeat("2", 32)
		p := withInstance(state.Players[side], "hand", ir.TestInstance{InstanceID: follower, CardID: 10361110, DeclaredType: "follower"})
		state.Players[side] = withInstance(p, "field", ir.TestInstance{InstanceID: shrine, CardID: 10162210, DeclaredType: "amulet"})
		s, err := NewSession(pack, state, 1)
		if err != nil {
			t.Fatal(err)
		}
		if r := s.SubmitAs(strings.Repeat("a", 32), side, SimulatorCommand{Kind: "play", Source: follower}); r.Status != StatusCompleted {
			t.Fatal(r)
		}
		crestEndTurn(t, s)
		if s.g.instances[follower].attack != -2 || !s.g.instances[follower].abilities["ward"] || s.g.player(side).crests[0].countdown != 4 {
			t.Fatal("end-turn crest did not apply its signed debuff")
		}
		crestEndTurn(t, s)
		if s.g.player(side).crests[0].countdown != 3 {
			t.Fatal("crest did not count down on its owner's next turn")
		}
		step := s.SubmitAs(strings.Repeat("b", 32), side, SimulatorCommand{Kind: "engage", Source: shrine})
		if step.Status != StatusSuspended {
			t.Fatal(step)
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
		for _, current := range []*Session{s, restored} {
			choice := step.Choice
			if r := current.Resume(ChoiceResponse{RequestID: choice.RequestID, ActionID: choice.ActionID, StateRevision: choice.StateRevision, SelectedInstanceIDs: []string{follower}}); r.Status != StatusCompleted {
				t.Fatal(r)
			}
			i := current.g.instances[follower]
			if i.attack != -1 || i.currentAttack() != 0 || i.life != 5 {
				t.Fatal("Grace added to displayed zero instead of stored -2")
			}
			view, _ := current.View(side)
			if view.Own.Field[1].Attack != 0 {
				t.Fatal("official QA zero-attack result was not public")
			}
			if r := current.SubmitAs(strings.Repeat("c", 32), side, SimulatorCommand{Kind: "evolve", Source: follower}); r.Status != StatusCompleted {
				t.Fatal(r)
			}
			if i.attack != 1 || i.life != 7 {
				t.Fatal("evolution forgot the signed attack deficit")
			}
		}
		if !reflect.DeepEqual(s.g.snapshot(), restored.g.snapshot()) {
			t.Fatal("Repose continuation diverged")
		}
	}
}

func TestReposeAttackHistoryPersistsAfterAttackerLeavesAndResetsNextTurn(t *testing.T) {
	pack := repeatCardPack(t)
	for _, side := range []string{"own", "oppo"} {
		for _, dies := range []bool{false, true} {
			state := crestState(side)
			state.FirstPlayer = side
			a, ally, enemy, crest := strings.Repeat("1", 32), strings.Repeat("2", 32), strings.Repeat("3", 32), strings.Repeat("4", 32)
			life := 4
			if dies {
				life = 1
			}
			p := withInstance(state.Players[side], "field", ir.TestInstance{InstanceID: a, CardID: 10361110, DeclaredType: "follower", Overrides: ir.InstanceOverrides{Stats: &ir.Stats{Attack: 0, Life: life}}})
			p = withInstance(p, "field", ir.TestInstance{InstanceID: ally, CardID: 10361110, DeclaredType: "follower"})
			state.Players[side] = withInstance(p, "crests", ir.TestInstance{InstanceID: crest, CardID: 10361110, DeclaredType: "crest"})
			other := oppositeSide(side)
			state.Players[other] = withInstance(state.Players[other], "field", ir.TestInstance{InstanceID: enemy, CardID: 10001110, DeclaredType: "follower"})
			s, err := NewSession(pack, state, 1)
			if err != nil {
				t.Fatal(err)
			}
			i := s.g.instances[a]
			if r := s.SubmitAs(strings.Repeat("a", 32), side, SimulatorCommand{Kind: "attack", Source: a, Defender: enemy}); r.Status != StatusCompleted {
				t.Fatal(r)
			}
			if (i.zone == "graveyard") != dies || !s.g.player(side).attackedThisTurn {
				t.Fatal("zero damage or departure erased attack history")
			}
			for _, owner := range []string{side, other} {
				if got := s.g.condition(ir.AttackHistoryCondition{Kind: "attack_history", Side: "own", Attacked: true}, s.g.player(owner).field[0]); got != (owner == side) {
					t.Fatal("condition used the wrong controller")
				}
			}
			restored, err := restoreGame(s.g.cards, snapshotContinuationGame(s.g))
			if err != nil || !restored.player(side).attackedThisTurn {
				t.Fatal("restored history lost an attack", err)
			}
			crestEndTurn(t, s)
			if s.g.instances[ally].attack != 0 || s.g.instances[ally].abilities["ward"] || s.g.rng.Consumed() != 0 {
				t.Fatal("crest triggered after a zero-damage attack")
			}
			// The previous player's stored flag does not mean an attack occurred in this turn.
			if s.g.condition(ir.AttackHistoryCondition{Kind: "attack_history", Side: "oppo", Attacked: true}, s.g.player(other).field[0]) {
				t.Fatal("condition read the previous player's turn")
			}
			crestEndTurn(t, s)
			if s.g.player(side).attackedThisTurn {
				t.Fatal("attack history did not reset")
			}
			crestEndTurn(t, s)
			if s.g.rng.Consumed() != 1 {
				t.Fatal("crest did not trigger on the next peaceful turn")
			}
		}
	}
}

func TestAttackHistoryConditionTracksRejectedAndLeaderAttacks(t *testing.T) {
	pack := repeatCardPack(t)
	state := crestState("oppo")
	id := fmt.Sprintf("%032x", 1)
	state.Players["oppo"] = withInstance(state.Players["oppo"], "field", ir.TestInstance{InstanceID: id, CardID: 10361110, DeclaredType: "follower"})
	s, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	i := s.g.instances[id]
	i.summoningSick = true
	if r := s.SubmitAs(strings.Repeat("a", 32), "oppo", SimulatorCommand{Kind: "attack", Source: id}); r.Status != StatusIllegal || s.g.oppo.attackedThisTurn {
		t.Fatal("rejected attack changed history", r)
	}
	i.summoningSick = false
	if r := s.SubmitAs(strings.Repeat("b", 32), "oppo", SimulatorCommand{Kind: "attack", Source: id}); r.Status != StatusCompleted || !s.g.oppo.attackedThisTurn || s.g.own.leaderLife != 20 {
		t.Fatal("zero leader attack was not recorded", r)
	}
}
