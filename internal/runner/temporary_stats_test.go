package runner

import (
	"maps"
	"reflect"
	"strings"
	"testing"

	"wbo/internal/ir"
)

func TestTemporaryStatsStackAndPreserveLaterChanges(t *testing.T) {
	pack := repeatCardPack(t)
	state := testState()
	id := strings.Repeat("1", 32)
	state.Players["own"] = withInstance(state.Players["own"], "field", ir.TestInstance{InstanceID: id, CardID: 10001110, DeclaredType: "follower"})
	s, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	i := s.g.instances[id]
	i.buffStats(2, 3, "own")
	i.buffStats(1, 2, "own")
	i.buffStats(-5, 1, "oppo")
	i.buffStats(1, 1, "")
	s.g.execTargetEffect(ir.TargetEffect{Kind: "damage", Target: ir.SelfRef{Kind: "self"}, Amount: 1}, i, frame{})
	if r := s.Advance(ir.AdvanceAction{Timing: "turn_end", Side: "own"}); r.Status != StatusCompleted {
		t.Fatal(r)
	}
	if i.attack != -2 || i.life != 3 {
		t.Fatalf("expiration lost permanent changes, damage or signed attack: %d/%d", i.attack, i.life)
	}
	if r := s.Advance(ir.AdvanceAction{Timing: "turn_end", Side: "oppo"}); r.Status != StatusCompleted {
		t.Fatal(r)
	}
	if i.attack != 3 || i.life != 2 || len(i.temporaryStats) != 0 {
		t.Fatalf("other deadline incorrect: %d/%d", i.attack, i.life)
	}
	i.buffStats(3, 4, "own")
	i.buffStats(2, 1, "oppo")
	s.g.execTargetEffect(ir.TargetEffect{Kind: "set_life", Target: ir.SelfRef{Kind: "self"}, Amount: 1}, i, frame{})
	s.g.expireTurnEffects("own")
	s.g.expireTurnEffects("oppo")
	if i.attack != 3 || i.life != 1 || i.zone != "field" {
		t.Fatal("set life retained an old life delta or erased the attack delta")
	}
	i.buffStats(2, 0, "own")
	s.g.returnCard(i, "hand")
	if i.attack != 2 || len(i.temporaryStats) != 0 {
		t.Fatal("field return did not reset temporary stats")
	}
	i.buffStats(2, 1, "own")
	s.g.returnCard(i, "deck")
	if i.attack != 4 || i.life != 3 {
		t.Fatal("hand to deck lost attached stats")
	}
	s.g.expireTurnEffects("own")
	if i.attack != 2 || i.life != 2 {
		t.Fatal("stats did not expire in the deck")
	}
	i.buffStats(2, 1, "oppo")
	resetCardState(i, i.card)
	if len(i.temporaryStats) != 0 {
		t.Fatal("card state replacement retained temporary stats")
	}
}

func TestCommanderAndMainyuExpireBothPlayers(t *testing.T) {
	pack := repeatCardPack(t)
	for _, side := range []string{"own", "oppo"} {
		for _, card := range []int{10122110, 10161140} {
			state := testState()
			state.Turn.Active, state.Turn.Number = side, 7
			id, amulet := strings.Repeat("1", 32), strings.Repeat("2", 32)
			p := state.Players[side]
			p.PP, p.MaxPP, p.EP = 3, 3, 1
			p = withInstance(p, "field", ir.TestInstance{InstanceID: id, CardID: card, DeclaredType: "follower"})
			p = withInstance(p, "field", ir.TestInstance{InstanceID: amulet, CardID: 10161210, DeclaredType: "amulet"})
			state.Players[side] = p
			s, err := NewSession(pack, state, 1)
			if err != nil {
				t.Fatal(err)
			}
			command, wantAttack, wantLife := SimulatorCommand{Kind: "evolve", Source: id}, 4, 3
			if card == 10161140 {
				command, wantAttack, wantLife = SimulatorCommand{Kind: "engage", Source: amulet}, 3, 2
			}
			if r := s.SubmitAs(strings.Repeat("a", 32), side, command); r.Status != StatusCompleted {
				t.Fatal(r)
			}
			if i := s.g.instances[id]; i.attack != wantAttack || i.life != wantLife {
				t.Fatalf("incorrect card effect for %d %s: %d/%d", card, side, i.attack, i.life)
			}
			if r := s.SubmitAs(strings.Repeat("b", 32), side, SimulatorCommand{Kind: "end_turn"}); r.Status != StatusCompleted {
				t.Fatal(r)
			}
			if i := s.g.instances[id]; i.attack != wantAttack-1 || i.life != wantLife || len(i.temporaryStats) != 0 {
				t.Fatalf("card bonus failed to expire for %s: %d/%d", side, i.attack, i.life)
			}
		}
	}
}

func TestStatExpirationDeathChoicePrecedesNextTurn(t *testing.T) {
	pack := repeatCardPack(t)
	pack.Cards = append(pack.Cards, ir.Card{ID: 77775002, CardType: "follower", Stats: &ir.Stats{Attack: 1, Life: 2}, Abilities: []ir.Ability{{
		ID: strings.Repeat("7", 32), Trigger: ir.SimpleTrigger{Kind: "lastwords"}, Body: []ir.Effect{
			ir.ModeEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("8", 32)}, Kind: "mode", Options: []ir.ModeOption{{ID: 1}, {ID: 2}}},
		},
	}}})
	for _, advance := range []bool{false, true} {
		state := testState()
		id, other := strings.Repeat("1", 32), strings.Repeat("2", 32)
		state.Players["own"] = withInstance(state.Players["own"], "field", ir.TestInstance{InstanceID: id, CardID: 77775002, DeclaredType: "follower"})
		state.Players["oppo"] = withInstance(state.Players["oppo"], "field", ir.TestInstance{InstanceID: other, CardID: 10001110, DeclaredType: "follower"})
		s, err := NewSession(pack, state, 1)
		if err != nil {
			t.Fatal(err)
		}
		for _, target := range []string{id, other} {
			i := s.g.instances[target]
			i.buffStats(0, 3, "own")
			i.life = 1
		}
		var step StepResult
		if advance {
			step = s.Advance(ir.AdvanceAction{Timing: "turn_end", Side: "own"})
		} else {
			step = s.Submit(strings.Repeat("a", 32), SimulatorCommand{Kind: "end_turn"})
		}
		if step.Status != StatusSuspended || s.g.turn.Active != "own" || s.g.instances[id].zone != "graveyard" || s.g.instances[other].zone != "graveyard" {
			t.Fatal("expiration deaths were not simultaneous before the choice", step)
		}
		var batches []uint64
		for _, event := range s.g.events {
			if event.Kind == "destroyed" {
				batches = append(batches, event.BatchID)
			}
		}
		if len(batches) != 2 || batches[0] != batches[1] {
			t.Fatal("expiration split the death batch", batches)
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
			if r := current.Resume(ChoiceResponse{RequestID: step.Choice.RequestID, ActionID: step.Choice.ActionID, StateRevision: step.Choice.StateRevision, SelectedOptionID: 1}); r.Status != StatusCompleted {
				t.Fatal(r)
			}
			wantSide := "oppo"
			if advance {
				wantSide = "own"
			}
			if current.g.turn.Active != wantSide || current.g.endingSide != "" {
				t.Fatal("end phase failed to complete")
			}
		}
		if !reflect.DeepEqual(s.g.snapshot(), restored.g.snapshot()) {
			t.Fatal("restoring expiration lastwords diverged")
		}
	}
}

func TestTemporaryStatsCloneRestoreAndQueryBudget(t *testing.T) {
	pack := repeatCardPack(t)
	state := testState()
	id := strings.Repeat("1", 32)
	state.Players["own"] = withInstance(state.Players["own"], "field", ir.TestInstance{InstanceID: id, CardID: 10001110, DeclaredType: "follower"})
	s, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	i := s.g.instances[id]
	i.buffStats(2, 1, "own")
	cloned := s.g.clone()
	cloned.instances[id].buffStats(3, 0, "own")
	if i.temporaryStats["own"].Attack != 2 {
		t.Fatal("clone aliases temporary stats")
	}
	saved := snapshotContinuationGame(s.g)
	for _, bad := range []map[string]ir.Stats{{"enemy": {Attack: 1}}, {"own": {}}} {
		saved.Instances[0].TemporaryStats = maps.Clone(bad)
		if _, err := restoreGame(s.g.cards, saved); err == nil {
			t.Fatal("accepted invalid temporary stats")
		}
	}
	s.budgetPolicy.QueryVisits = 1
	s.budget.reset(s.budgetPolicy)
	step := s.Advance(ir.AdvanceAction{Timing: "turn_end", Side: "own"})
	if step.Status != StatusFault || step.ErrorCode != executionBudgetExceeded || i.attack != 4 || i.life != 3 {
		t.Fatal("query budget allowed partial expiration", step)
	}
}
