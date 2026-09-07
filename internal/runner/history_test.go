package runner

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"wbo/internal/ir"
	"wbo/internal/ruleset"
)

func historySession(t *testing.T, side string, seed uint64) (*Session, *ir.CardPack, string, string) {
	t.Helper()
	pack, state := reanimateFixture()
	pack.Cards[0].PlayEffects = append([]ir.Effect{ir.ModeEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("d", 32)}, Kind: "mode", Options: []ir.ModeOption{{ID: 1, Body: []ir.Effect{}}}}}, pack.Cards[0].PlayEffects...)
	state.Turn.Active = side
	source, victim := strings.Repeat("1", 32), strings.Repeat("2", 32)
	p := withInstance(state.Players[side], "hand", ir.TestInstance{InstanceID: source, CardID: 77771001, DeclaredType: "spell"})
	state.Players[side] = withInstance(p, "field", ir.TestInstance{InstanceID: victim, CardID: 77771002, DeclaredType: "follower"})
	s, err := NewSession(pack, state, seed)
	if err != nil {
		t.Fatal(err)
	}
	return s, pack, source, victim
}

func transformHistoryVictim(s *Session, victim *instance, card int) {
	s.g.returnCard(victim, "hand")
	s.g.execCardEffect(ir.CardEffect{Kind: "transform", Target: ir.SelfRef{Kind: "self"}, CardID: card}, victim, nil)
}

func TestDestructionHistorySurvivesLiveTransformationAndRestore(t *testing.T) {
	for _, side := range []string{"own", "oppo"} {
		t.Run(side, func(t *testing.T) {
			s, pack, source, victimID := historySession(t, side, 7)
			victim := s.g.instances[victimID]
			victim.cost, victim.attack, victim.life = 1, 9, 2
			victim.evolved, victim.departed = true, true
			s.g.destroyByEffect([]*instance{victim})
			history := append([]DestructionRecord{}, s.g.player(side).destroyed...)
			transformHistoryVictim(s, victim, 77771005)
			if !reflect.DeepEqual(history, s.g.player(side).destroyed) || victim.card.ID != 77771005 {
				t.Fatal("live transformation rewrote destruction history")
			}
			for _, viewer := range []string{"own", "oppo"} {
				view, _ := s.View(viewer)
				p := view.Own
				if viewer != side {
					p = view.Oppo
				}
				if len(p.Destroyed) != 1 || p.Destroyed[0].CardID != 77771002 || p.Destroyed[0].Cost != 1 || p.Destroyed[0].Attack != 9 || !p.Destroyed[0].Evolved {
					t.Fatal("public history reflects the hidden transformed card", p.Destroyed)
				}
			}
			ref := ir.FilterRef{Kind: "filter", Source: ir.ZoneRef{Kind: "zone", Side: "own", Zone: "destroyed", Member: "follower"}, Predicate: ir.FieldPredicate{Kind: "has_card", CardID: 77771002}}
			if len(s.g.fromRef(ref, victim, nil)) != 1 {
				t.Fatal("history query lost the destroyed identity")
			}
			step := s.SubmitAs(strings.Repeat("a", 32), side, SimulatorCommand{Kind: "play", Source: source})
			if step.Status != StatusSuspended {
				t.Fatal(step)
			}
			data, err := s.EncodeContinuation()
			if err != nil {
				t.Fatal(err)
			}
			again, _ := s.EncodeContinuation()
			if !bytes.Equal(data, again) {
				t.Fatal("nondeterministic history encoding")
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
				if r := current.Resume(ChoiceResponse{RequestID: step.Choice.RequestID, ActionID: step.Choice.ActionID, StateRevision: step.Choice.StateRevision, SelectedOptionID: 1}); r.Status != StatusCompleted {
					t.Fatal(r)
				}
				p := current.g.player(side)
				if len(p.field) != 1 || p.field[0].card.ID != 77771002 || p.field[0].cost != 4 || p.field[0].attack != 3 || p.field[0].evolved || !p.field[0].departed || current.g.rng.Consumed() != 0 {
					t.Fatal("reanimation did not use the destroyed definition")
				}
			}
			if !reflect.DeepEqual(s.g.snapshot(), restored.g.snapshot()) {
				t.Fatal("restored history execution diverged")
			}
		})
	}
}

func TestRepeatedDestructionKeepsEachIdentityAndLotteryTicket(t *testing.T) {
	for seed := uint64(0); seed < 12; seed++ {
		s, _, _, id := historySession(t, "own", seed)
		victim := s.g.instances[id]
		s.g.destroyByEffect([]*instance{victim})
		transformHistoryVictim(s, victim, 77771003)
		s.g.move(victim, "field")
		s.g.destroyByEffect([]*instance{victim})
		transformHistoryVictim(s, victim, 77771005)
		if len(s.g.own.destroyed) != 2 || s.g.own.destroyed[0].CardID != 77771002 || s.g.own.destroyed[1].CardID != 77771003 {
			t.Fatal("repeated instance IDs merged or rewrote history")
		}
		clone := s.g.clone()
		clone.own.destroyed[0].CardID = 77771005
		if s.g.own.destroyed[0].CardID != 77771002 {
			t.Fatal("legality sandbox shares mutable history")
		}
		saved := snapshotContinuationGame(s.g)
		restored, err := restoreGame(s.g.cards, saved)
		if err != nil {
			t.Fatal(err)
		}
		for _, g := range []*game{s.g, restored} {
			g.execCardEffect(ir.CardEffect{Kind: "reanimate", Owner: "own", MaxCost: 4, Output: "summoned"}, nil, frame{})
			want := []int{77771002, 77771003}[ruleset.NewRNG(seed).Index(2)]
			if len(g.own.field) != 1 || g.own.field[0].card.ID != want || g.rng.Consumed() != 1 {
				t.Fatal("history lottery no longer uses one ticket per destruction", seed)
			}
		}
	}
}

func TestHistoryQueriesCannotModifyTheLiveInstance(t *testing.T) {
	s, _, _, id := historySession(t, "own", 1)
	victim := s.g.instances[id]
	s.g.destroyByEffect([]*instance{victim})
	transformHistoryVictim(s, victim, 77771005)
	ref := ir.ZoneRef{Kind: "zone", Side: "own", Zone: "destroyed", Member: "card"}
	before := s.g.snapshot()
	s.g.execTargetEffect(ir.TargetEffect{Kind: "return", Target: ref, Destination: "hand"}, nil, frame{})
	s.g.execTargetEffect(ir.TargetEffect{Kind: "buff_stats", Target: ref, AttackDelta: 10, LifeDelta: 10}, nil, frame{})
	s.g.execCardEffect(ir.CardEffect{Kind: "transform", Target: ref, CardID: 77771004}, nil, frame{})
	for _, kind := range []string{"choose", "require", "random_choose"} {
		if len(s.g.selectionCandidates(ir.SelectionEffect{Kind: kind, Source: ref}, nil, nil)) != 0 {
			t.Fatal("history entry became an actionable selection")
		}
	}
	if !reflect.DeepEqual(before, s.g.snapshot()) {
		t.Fatal("read-only history operation changed game state")
	}
	query := s.g.fromRef(ref, nil, nil)
	query[0].cost = 50
	if s.g.fromRef(ref, nil, nil)[0].cost == 50 {
		t.Fatal("query result mutated the stored record")
	}
}

func TestRestoreRejectsMalformedDestructionRecords(t *testing.T) {
	s, _, _, id := historySession(t, "own", 1)
	s.g.destroyByEffect([]*instance{s.g.instances[id]})
	for name, mutate := range map[string]func(*ContinuationGame){
		"unknown identity":     func(g *ContinuationGame) { g.Own.Destroyed[0].CardID = 88888888 },
		"wrong identity":       func(g *ContinuationGame) { g.Own.Destroyed[0].CardID = 77771003 },
		"spell identity":       func(g *ContinuationGame) { g.Own.Destroyed[0].CardID = 77771001 },
		"unknown instance":     func(g *ContinuationGame) { g.Own.Destroyed[0].InstanceID = "missing" },
		"wrong owner":          func(g *ContinuationGame) { g.Oppo.Destroyed, g.Own.Destroyed = g.Own.Destroyed, nil },
		"missing record":       func(g *ContinuationGame) { g.Own.Destroyed = nil },
		"duplicate event":      func(g *ContinuationGame) { g.Own.Destroyed = append(g.Own.Destroyed, g.Own.Destroyed[0]) },
		"missing event":        func(g *ContinuationGame) { g.Own.Destroyed[0].EventSequence = 1000 },
		"wrong event":          func(g *ContinuationGame) { g.Own.Destroyed[0].EventSequence = 1 },
		"fake initial record":  func(g *ContinuationGame) { g.Own.Destroyed[0].EventSequence = 0 },
		"negative cost":        func(g *ContinuationGame) { g.Own.Destroyed[0].Cost = -1 },
		"missing turn":         func(g *ContinuationGame) { g.Own.Destroyed[0].TurnSide = "" },
		"unknown turn side":    func(g *ContinuationGame) { g.Own.Destroyed[0].TurnSide = "both" },
		"future turn":          func(g *ContinuationGame) { g.Own.Destroyed[0].TurnNumber = g.Turn.Number + 1 },
		"future opponent turn": func(g *ContinuationGame) { g.Own.Destroyed[0].TurnSide = "oppo" },
		"negative turn":        func(g *ContinuationGame) { g.Own.Destroyed[0].TurnNumber = -1 },
	} {
		t.Run(name, func(t *testing.T) {
			saved := snapshotContinuationGame(s.g)
			mutate(&saved)
			if _, err := restoreGame(s.g.cards, saved); err == nil {
				t.Fatal("malformed destruction record accepted")
			}
		})
	}
}
