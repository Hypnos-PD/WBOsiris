package runner

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"wbo/internal/ir"
)

func galmieuxState(side string) ir.State {
	state := testState()
	state.Turn.Active = side
	state.Players[side] = withInstance(state.Players[side], "field", ir.TestInstance{InstanceID: strings.Repeat("1", 32), CardID: 10344120, DeclaredType: "follower"})
	state.Players[oppositeSide(side)] = withInstance(state.Players[oppositeSide(side)], "field", ir.TestInstance{InstanceID: strings.Repeat("2", 32), CardID: 10144110, DeclaredType: "follower"})
	for n, seat := range []string{"own", "oppo"} {
		state.Players[seat] = withInstance(state.Players[seat], "deck", ir.TestInstance{InstanceID: fmt.Sprintf("%032x", 100+n), CardID: 10041110, DeclaredType: "follower"})
	}
	return state
}

func TestGalmieuxCrestLimitResetsAndEmptyTargetConsumesFollowerLimit(t *testing.T) {
	for _, side := range []string{"own", "oppo"} {
		t.Run(side, func(t *testing.T) {
			s, err := NewSession(repeatCardPack(t), galmieuxState(side), 49)
			if err != nil {
				t.Fatal(err)
			}
			p := s.g.player(side)
			source := p.field[0]
			enemy := s.g.player(oppositeSide(side)).field[0]
			s.g.gainCrest(source, "own", 10344120)
			// Off-turn damage must not reserve either limit.
			s.g.turn.Active = oppositeSide(side)
			s.g.damageInstance(source, 0)
			if r := s.run(); r.Status != StatusCompleted || len(source.usedTriggers) != 0 || len(p.crests[0].usedTriggers) != 0 {
				t.Fatal("off-turn survival consumed a limit", r)
			}
			s.g.turn.Active = side
			// Reserve both abilities before draining the queue.
			s.g.damageInstance(source, 0)
			s.g.damageInstance(source, 0)
			if len(s.g.triggers) != 2 || s.g.triggers[0].self != p.crests[0] || s.g.triggers[1].self != source {
				t.Fatal("survival trigger order or reservation is incorrect")
			}
			if r := s.run(); r.Status != StatusCompleted || len(p.hand) != 1 || enemy.life != 4 {
				t.Fatal("survival fired more than once", r)
			}
			for n, seat := range []string{side, oppositeSide(side)} {
				if r := s.SubmitAs(fmt.Sprintf("%032x", n+1), seat, SimulatorCommand{Kind: "end_turn"}); r.Status != StatusCompleted {
					t.Fatal(r)
				}
			}
			if s.g.turn.Active != side || len(source.usedTriggers) != 0 || len(p.crests[0].usedTriggers) != 0 {
				t.Fatal("new turn did not reset both limits")
			}
			// A listener with no enemy target still spends its once-per-turn use.
			s.g.destroyByEffect([]*instance{enemy})
			s.g.damageInstance(source, 0)
			if r := s.run(); r.Status != StatusCompleted || len(source.usedTriggers) != 1 || len(p.hand) != 3 {
				t.Fatal("empty target did not consume use or crest did not reset", r)
			}
			late := s.g.newInstance(s.g.cards[10144110], strings.Repeat("5", 32), "late", "field")
			s.g.player(oppositeSide(side)).field = append(s.g.player(oppositeSide(side)).field, late)
			s.g.damageInstance(source, 0)
			if r := s.run(); r.Status != StatusCompleted || late.life != 7 || len(p.hand) != 3 {
				t.Fatal("empty target left an extra use available", r)
			}
		})
	}
}

func TestGalmieuxCrestChecksDamagedSubjectAndFullHand(t *testing.T) {
	for _, side := range []string{"own", "oppo"} {
		for _, reason := range []string{"other_ally", "lethal_ally", "enemy", "leader", "full_hand"} {
			t.Run(side+"/"+reason, func(t *testing.T) {
				state := galmieuxState(side)
				allyID := strings.Repeat("3", 32)
				state.Players[side] = withInstance(state.Players[side], "field", ir.TestInstance{InstanceID: allyID, CardID: 10041110, DeclaredType: "follower"})
				if reason == "full_hand" {
					for n := 0; n < 9; n++ {
						state.Players[side] = withInstance(state.Players[side], "hand", ir.TestInstance{InstanceID: fmt.Sprintf("%032x", 200+n), CardID: 10041110, DeclaredType: "follower"})
					}
				}
				s, err := NewSession(repeatCardPack(t), state, 49)
				if err != nil {
					t.Fatal(err)
				}
				p := s.g.player(side)
				s.g.gainCrest(p.field[0], "own", 10344120)
				switch reason {
				case "other_ally", "full_hand":
					s.g.damageInstance(s.g.instances[allyID], 0)
				case "lethal_ally":
					s.g.damageInstance(s.g.instances[allyID], 99)
				case "enemy":
					s.g.damageInstance(s.g.player(oppositeSide(side)).field[0], 0)
				case "leader":
					s.g.emitDamage(1, &ir.EventTarget{Kind: "leader", Side: side}, nil)
				}
				s.g.resolveDeathBatch(nil)
				if r := s.run(); r.Status != StatusCompleted {
					t.Fatal(r)
				}
				if len(p.field[0].usedTriggers) != 0 {
					t.Fatal("another subject consumed Galmieux's personal limit")
				}
				wantHand, wantUsed := 0, 0
				if reason == "other_ally" {
					wantHand, wantUsed = 1, 1
				} else if reason == "full_hand" {
					wantHand, wantUsed = 9, 1
					if p.shadows != 1 || len(p.graveyard) != 1 || p.graveyard[0].card.ID != 90044320 {
						t.Fatal("overflow did not send the generated token to graveyard")
					}
				}
				if len(p.hand) != wantHand || len(p.crests[0].usedTriggers) != wantUsed {
					t.Fatal("crest checked its source instead of damaged subject")
				}
			})
		}
	}
}

func TestGalmieuxCrestContinuationPreservesQueueLimitsAndRandomness(t *testing.T) {
	for _, side := range []string{"own", "oppo"} {
		pack := repeatCardPack(t)
		for n := range pack.Cards {
			if pack.Cards[n].ID == 10344120 {
				a := &pack.Cards[n].Crest.Abilities[0]
				a.Body = append([]ir.Effect{ir.ModeEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("e", 32)}, Kind: "mode", Options: []ir.ModeOption{{ID: 1, Body: []ir.Effect{}}}}}, a.Body...)
			}
		}
		state := galmieuxState(side)
		state.Players[oppositeSide(side)] = withInstance(state.Players[oppositeSide(side)], "field", ir.TestInstance{InstanceID: strings.Repeat("3", 32), CardID: 10144110, DeclaredType: "follower"})
		fangsID := strings.Repeat("4", 32)
		state.Players[side] = withInstance(state.Players[side], "hand", ir.TestInstance{InstanceID: fangsID, CardID: 90044320, DeclaredType: "spell"})
		s, err := NewSession(pack, state, 49)
		if err != nil {
			t.Fatal(err)
		}
		s.g.gainCrest(s.g.player(side).field[0], "own", 10344120)
		before := s.g.snapshot()
		s.LegalActionsFor(side)
		if !reflect.DeepEqual(before, s.g.snapshot()) {
			t.Fatal("preview consumed survival limits or randomness")
		}
		step := s.SubmitAs(strings.Repeat("a", 32), side, SimulatorCommand{Kind: "play", Source: fangsID})
		if step.Status != StatusSuspended || len(s.g.triggers) != 1 {
			t.Fatal("crest failed to suspend ahead of follower retaliation", step)
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
			p := current.g.player(side)
			if len(p.hand) != 1 || p.hand[0].card.ID != 90044320 || len(p.crests[0].usedTriggers) != 1 || len(p.field[0].usedTriggers) != 1 {
				t.Fatal("restored crest or follower lost its reserved use")
			}
			current.g.damageInstance(p.field[0], 0)
			if r := current.run(); r.Status != StatusCompleted || len(p.hand) != 1 {
				t.Fatal("restored survival triggered twice", r)
			}
		}
		if !reflect.DeepEqual(s.g.snapshot(), restored.g.snapshot()) || !reflect.DeepEqual(s.Events(), restored.Events()) {
			t.Fatal("continuation changed random target, bindings, events or state")
		}
	}
}

func TestDamageSurvivalExternalBindingAndPostDamageFilter(t *testing.T) {
	for _, side := range []string{"own", "oppo"} {
		pack := repeatCardPack(t)
		for n := range pack.Cards {
			if pack.Cards[n].ID == 10344120 {
				a := &pack.Cards[n].Crest.Abilities[0]
				a.Trigger = ir.EventTrigger{Kind: "event", Event: "damaged", Side: "oppo", SubjectType: "follower", DuringTurn: "own", OncePerTurn: "own", Predicate: ir.FieldPredicate{Kind: "compare", Field: "life", Op: "le", Value: 5}}
				a.Body = []ir.Effect{ir.TargetEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("e", 32)}, Kind: "buff_stats", Target: ir.BindingRef{Kind: "binding", Name: "damaged"}, LifeDelta: 1}}
			}
		}
		s, err := NewSession(pack, galmieuxState(side), 49)
		if err != nil {
			t.Fatal(err)
		}
		p := s.g.player(side)
		s.g.gainCrest(p.field[0], "own", 10344120)
		enemy := s.g.player(oppositeSide(side)).field[0]
		s.g.damageInstance(enemy, 1)
		if r := s.run(); r.Status != StatusCompleted || len(p.crests[0].usedTriggers) != 0 {
			t.Fatal("failed filter consumed a use", r)
		}
		s.g.damageInstance(enemy, 1)
		if r := s.run(); r.Status != StatusCompleted || enemy.life != 6 || p.field[0].life != 5 || len(p.crests[0].usedTriggers) != 1 {
			t.Fatal("listener failed to buff its damaged subject", side, r, enemy.life, p.field[0].life, p.crests[0].usedTriggers)
		}
	}
}
