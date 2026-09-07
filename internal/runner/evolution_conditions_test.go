package runner

import (
	"reflect"
	"strings"
	"testing"

	"wbo/internal/ir"
)

func TestEvolutionUnlockConditionsFollowSeatAndTurn(t *testing.T) {
	pack := repeatCardPack(t)
	id := strings.Repeat("1", 32)
	for _, first := range []string{"own", "oppo", ""} {
		for _, side := range []string{"own", "oppo"} {
			state := testState()
			state.FirstPlayer = first
			state.Players[side] = withInstance(state.Players[side], "field", ir.TestInstance{InstanceID: id, CardID: 10001110, DeclaredType: "follower"})
			s, err := NewSession(pack, state, 1)
			if err != nil {
				t.Fatal(err)
			}
			s.g.player(side).ep, s.g.player(side).sep = 0, 0
			s.g.player(side).evolvedThisTurn = true
			for turn := 3; turn <= 8; turn++ {
				s.g.turn.Number = turn
				for _, active := range []string{"own", "oppo"} {
					s.g.turn.Active = active
					for _, relative := range []string{"own", "oppo"} {
						player := side
						if relative == "oppo" {
							player = oppositeSide(side)
						}
						for _, form := range []string{"evolved", "super_evolved"} {
							threshold := 4
							if form == "super_evolved" {
								threshold = 6
							}
							if player == first || first == "" && player == "own" {
								threshold++
							}
							condition := ir.EvolutionUnlockedCondition{Kind: "evolution_unlocked", Side: relative, Form: form}
							if got := s.g.condition(condition, s.g.instances[id]); got != (turn >= threshold) {
								t.Fatalf("first=%s source=%s relative=%s active=%s turn=%d form=%s: %v", first, side, relative, active, turn, form, got)
							}
						}
					}
				}
			}
		}
	}
}

func TestSamuraiFanfareAtBothSeatBoundaries(t *testing.T) {
	pack := repeatCardPack(t)
	id := strings.Repeat("1", 32)
	for _, first := range []string{"own", "oppo"} {
		for _, side := range []string{"own", "oppo"} {
			threshold := 6
			if side == first {
				threshold = 7
			}
			for _, turn := range []int{threshold - 1, threshold} {
				state := testState()
				state.FirstPlayer, state.Turn.Active, state.Turn.Number = first, side, turn
				p := state.Players[side]
				p.PP, p.MaxPP, p.SEP = 2, turn, 0
				state.Players[side] = withInstance(p, "hand", ir.TestInstance{InstanceID: id, CardID: 10121150, DeclaredType: "follower"})
				s, err := NewSession(pack, state, 1)
				if err != nil {
					t.Fatal(err)
				}
				s.g.player(side).evolvedThisTurn = true
				if r := s.SubmitAs(strings.Repeat("a", 32), side, SimulatorCommand{Kind: "play", Source: id}); r.Status != StatusCompleted {
					t.Fatal(r)
				}
				i := s.g.instances[id]
				if i.abilities["bane"] != (turn == threshold) || !i.abilities["rush"] || i.evolved || i.superEvolved {
					t.Fatalf("wrong Samurai state first=%s side=%s turn=%d", first, side, turn)
				}
			}
		}
	}
}

func TestSelfFormConditionsIncludeSuperEvolution(t *testing.T) {
	g := &game{}
	for _, tc := range []struct {
		cardType       string
		evolved, super bool
		want           [3]bool
	}{
		{"follower", false, false, [3]bool{true, false, false}},
		{"follower", true, false, [3]bool{false, true, false}},
		{"follower", true, true, [3]bool{false, true, true}},
		{"amulet", false, false, [3]bool{false, false, false}},
	} {
		i := &instance{card: &ir.Card{CardType: tc.cardType}, evolved: tc.evolved, superEvolved: tc.super}
		for n, form := range []string{"unevolved", "evolved", "super_evolved"} {
			c := ir.SelfFormCondition{Kind: "self_form", Form: form}
			if g.condition(c, i) != tc.want[n] || g.condition(c, nil) {
				t.Fatal("wrong self form", tc, form)
			}
		}
	}
}

func TestCeresManualEvolutionEndTurnAndBarrierRenewal(t *testing.T) {
	pack := repeatCardPack(t)
	id := strings.Repeat("1", 32)
	for _, side := range []string{"own", "oppo"} {
		for _, kind := range []string{"evolve", "superevolve"} {
			state := testState()
			state.Turn.Active, state.Turn.Number = side, 7
			p := state.Players[side]
			p.Leader.Life, p.EP, p.SEP = 10, 1, 1
			state.Players[side] = withInstance(p, "field", ir.TestInstance{InstanceID: id, CardID: 10153110, DeclaredType: "follower"})
			s, err := NewSession(pack, state, 1)
			if err != nil {
				t.Fatal(err)
			}
			if r := s.SubmitAs(strings.Repeat("a", 32), side, SimulatorCommand{Kind: kind, Source: id}); r.Status != StatusCompleted {
				t.Fatal(r)
			}
			if r := s.SubmitAs(strings.Repeat("b", 32), side, SimulatorCommand{Kind: "end_turn"}); r.Status != StatusCompleted {
				t.Fatal(r)
			}
			wantHeal := 2
			if kind == "superevolve" {
				wantHeal = 4
			}
			i := s.g.instances[id]
			if s.g.player(side).leaderLife != 10+wantHeal || i.abilities["barrier"] != (wantHeal == 4) || !i.abilities["bane"] {
				t.Fatal("Ceres end phase resolved the wrong form", side, kind)
			}
			if wantHeal == 4 {
				before := i.life
				s.g.execTargetEffect(ir.TargetEffect{Kind: "damage", Target: ir.SelfRef{Kind: "self"}, Amount: 1}, i, frame{})
				if i.abilities["barrier"] || i.life != before {
					t.Fatal("Barrier did not prevent damage and get consumed")
				}
				if r := s.Advance(ir.AdvanceAction{Timing: "turn_end", Side: side}); r.Status != StatusCompleted {
					t.Fatal(r)
				}
				if !i.abilities["barrier"] || s.g.player(side).leaderLife != 18 {
					t.Fatal("Ceres did not renew Barrier on the following end phase")
				}
			}
		}
	}
}

func TestEvolutionConditionsSurviveSuspendedAction(t *testing.T) {
	pack := repeatCardPack(t)
	pack.Cards = append(pack.Cards, ir.Card{ID: 77775003, CardType: "follower", Stats: &ir.Stats{Attack: 1, Life: 2}, Abilities: []ir.Ability{{
		ID: strings.Repeat("7", 32), Trigger: ir.EventTrigger{Kind: "event", Event: "turn_ended", Side: "own"}, Body: []ir.Effect{
			ir.ModeEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("8", 32)}, Kind: "mode", Options: []ir.ModeOption{{ID: 1}, {ID: 2}}},
			ir.IfEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("9", 32)}, Kind: "if", Condition: ir.SelfFormCondition{Kind: "self_form", Form: "super_evolved"}, Then: []ir.Effect{
				ir.IfEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("a", 32)}, Kind: "if", Condition: ir.EvolutionUnlockedCondition{Kind: "evolution_unlocked", Side: "own", Form: "super_evolved"}, Then: []ir.Effect{
					ir.TargetEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("b", 32)}, Kind: "heal", Target: ir.LeaderRef{Kind: "leader", Side: "own", ValueType: "leader"}, Amount: 4},
				}},
			}},
		},
	}}})
	state := testState()
	state.Turn.Number = 7
	id := strings.Repeat("1", 32)
	p := state.Players["own"]
	p.Leader.Life = 10
	state.Players["own"] = withInstance(p, "field", ir.TestInstance{InstanceID: id, CardID: 77775003, DeclaredType: "follower"})
	s, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	s.g.applyEvolution(s.g.instances[id], true)
	step := s.Submit(strings.Repeat("c", 32), SimulatorCommand{Kind: "end_turn"})
	if step.Status != StatusSuspended || s.g.own.leaderLife != 10 {
		t.Fatal(step)
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
	for _, current := range []*Session{s, restored} {
		if r := current.Resume(ChoiceResponse{RequestID: step.Choice.RequestID, ActionID: step.Choice.ActionID, StateRevision: step.Choice.StateRevision, SelectedOptionID: 1}); r.Status != StatusCompleted || current.g.own.leaderLife != 14 {
			t.Fatal("restored condition chose the wrong branch", r)
		}
	}
	if !reflect.DeepEqual(s.g.snapshot(), restored.g.snapshot()) {
		t.Fatal("restoring conditional end phase diverged")
	}
}
