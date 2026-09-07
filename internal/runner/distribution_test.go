package runner

import (
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"wbo/internal/ir"
	"wbo/internal/project"
)

func TestCompiledDistributionCardsPlayAndEvolveForEitherPlayer(t *testing.T) {
	root := filepath.Join("..", "..")
	loaded := project.LoadWithRoot([]string{
		filepath.Join(root, "cards", "10001", "10113120.wbo"),
		filepath.Join(root, "cards", "10001", "10154130.wbo"),
		filepath.Join(root, "cards", "90000", "90001110.wbo"),
	}, true, root)
	if loaded.HasErrors() {
		t.Fatal(loaded.Diagnostics)
	}
	pack, _, err := project.BuildRuntimePacks(loaded)
	if err != nil {
		t.Fatal(err)
	}
	for _, side := range []string{"own", "oppo"} {
		for _, cardID := range []int{10113120, 10154130} {
			t.Run(fmt.Sprintf("%s/%d", side, cardID), func(t *testing.T) {
				state := testState()
				state.Turn.Active = side
				actor := state.Players[side]
				actor.PP, actor.MaxPP, actor.EP = 5, 5, 1
				sourceID, targetID := strings.Repeat("a", 32), strings.Repeat("b", 32)
				actor = withInstance(actor, "hand", ir.TestInstance{InstanceID: sourceID, CardID: cardID, DeclaredType: "follower"})
				for n := 0; n < 2; n++ {
					actor = withInstance(actor, "deck", ir.TestInstance{InstanceID: fmt.Sprintf("%032x", n+1), CardID: 90001110, DeclaredType: "follower"})
				}
				state.Players[side] = actor
				targetSide := oppositeSide(side)
				state.Players[targetSide] = withInstance(state.Players[targetSide], "field", ir.TestInstance{InstanceID: targetID, CardID: 90001110, DeclaredType: "follower", Overrides: ir.InstanceOverrides{Stats: &ir.Stats{Attack: 1, Life: 8}}})
				session, err := NewSession(pack, state, 1)
				if err != nil {
					t.Fatal(err)
				}
				if result := session.SubmitAs(strings.Repeat("c", 32), side, SimulatorCommand{Kind: "play", Source: sourceID}); result.Status != StatusCompleted {
					t.Fatal(result)
				}
				wantHand, wantAfterPlay, wantAfterEvolve, wantLeader := 2, 8, 6, 20
				if cardID == 10154130 {
					wantHand, wantAfterPlay, wantAfterEvolve, wantLeader = 0, 1, 1, 17
				}
				if len(session.g.player(side).hand) != wantHand || session.g.instances[targetID].life != wantAfterPlay {
					t.Fatal("compiled fanfare diverged from card text")
				}
				if result := session.SubmitAs(strings.Repeat("d", 32), side, SimulatorCommand{Kind: "evolve", Source: sourceID}); result.Status != StatusCompleted {
					t.Fatal(result)
				}
				if session.g.instances[targetID].life != wantAfterEvolve || session.g.player(side).leaderLife != wantLeader || session.g.player(targetSide).leaderLife != wantLeader {
					t.Fatal("compiled evolve diverged from card text")
				}
			})
		}
	}
}

func TestDistributedDamageAllocationAndOverflow(t *testing.T) {
	for _, tc := range []struct {
		name           string
		amount         int
		life           []int
		barrier        bool
		lastReduction  int
		overflow       bool
		wantLife       []int
		wantLeaderLife int
	}{
		{name: "empty", amount: 7, wantLeaderLife: 20},
		{name: "empty_overflow", amount: 7, overflow: true, wantLeaderLife: 13},
		{name: "last_receives_all_remainder", amount: 8, life: []int{2, 2}, lastReduction: 4, wantLife: []int{0, 0}, wantLeaderLife: 20},
		{name: "barrier_consumes_allocation", amount: 7, life: []int{2, 3}, barrier: true, wantLife: []int{2, 0}, wantLeaderLife: 20},
		{name: "barrier_with_overflow", amount: 7, life: []int{2, 3}, barrier: true, overflow: true, wantLife: []int{2, 0}, wantLeaderLife: 18},
		{name: "reduction_does_not_refund", amount: 7, life: []int{2, 3}, lastReduction: 2, overflow: true, wantLife: []int{0, 2}, wantLeaderLife: 18},
		{name: "zero_keeps_barrier", amount: 0, life: []int{2}, barrier: true, overflow: true, wantLife: []int{2}, wantLeaderLife: 20},
	} {
		for _, side := range []string{"own", "oppo"} {
			t.Run(tc.name+"/"+side, func(t *testing.T) {
				pack := &ir.CardPack{Cards: []ir.Card{
					{ID: 77774001, CardType: "follower", Stats: &ir.Stats{Attack: 1, Life: 1}},
					{ID: 77774002, CardType: "amulet"},
				}}
				state := testState()
				sourceID := strings.Repeat("a", 32)
				state.Players[side] = withInstance(state.Players[side], "field", ir.TestInstance{InstanceID: sourceID, CardID: 77774001, DeclaredType: "follower"})
				targetSide := oppositeSide(side)
				opponent := withInstance(state.Players[targetSide], "field", ir.TestInstance{InstanceID: strings.Repeat("b", 32), CardID: 77774002, DeclaredType: "amulet"})
				for n, life := range tc.life {
					overrides := ir.InstanceOverrides{Stats: &ir.Stats{Attack: 1, Life: life}, Keywords: []string{"stealth", "aura"}}
					if n == 0 && tc.barrier {
						overrides.Keywords = append(overrides.Keywords, "barrier")
					}
					if n == len(tc.life)-1 {
						overrides.DamageReduction = &tc.lastReduction
					}
					opponent = withInstance(opponent, "field", ir.TestInstance{InstanceID: fmt.Sprintf("%032x", 10-n), CardID: 77774001, DeclaredType: "follower", Overrides: overrides})
				}
				state.Players[targetSide] = opponent
				session, err := NewSession(pack, state, 1)
				if err != nil {
					t.Fatal(err)
				}
				effect := ir.TargetEffect{Kind: "damage", DamageType: "effect", Distribution: "field_entry_order", Amount: tc.amount,
					Target: ir.ZoneRef{Kind: "zone", Side: "oppo", Zone: "field", Member: "follower"}}
				if tc.overflow {
					effect.Overflow = ir.LeaderRef{Kind: "leader", Side: "oppo", ValueType: "leader"}
				}
				session.g.execTargetEffect(effect, session.g.instances[sourceID], frame{})
				for n, want := range tc.wantLife {
					target := session.g.instances[fmt.Sprintf("%032x", 10-n)]
					if target.life != want || (target.zone == "graveyard") != (want == 0) {
						t.Fatalf("target %d life=%d zone=%s want=%d", n, target.life, target.zone, want)
					}
					if n == 0 && tc.barrier && target.abilities["barrier"] != (tc.amount == 0) {
						t.Fatal("incorrect barrier consumption")
					}
				}
				if session.g.player(targetSide).leaderLife != tc.wantLeaderLife || session.g.player(side).leaderLife != 20 || session.g.rng.Consumed() != 0 {
					t.Fatal("incorrect leader overflow or random consumption")
				}
				if tc.amount == 0 && len(session.g.events) != 0 {
					t.Fatal("zero damage distribution emitted events")
				}
			})
		}
	}
}

func TestRepeatedDistributionAfterRestorePreservesDeathBatches(t *testing.T) {
	mode := ir.ModeEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("a", 32)}, Kind: "mode", Options: []ir.ModeOption{
		{ID: 1, Body: []ir.Effect{}}, {ID: 2, Body: []ir.Effect{}},
	}}
	body := []ir.Effect{mode}
	for n := 0; n < 3; n++ {
		body = append(body, ir.TargetEffect{NodeBase: ir.NodeBase{ID: fmt.Sprintf("%032x", n+100)}, Kind: "damage", DamageType: "effect", Distribution: "field_entry_order", Amount: 3,
			Target: ir.ZoneRef{Kind: "zone", Side: "oppo", Zone: "field", Member: "follower"}})
	}
	pack := &ir.CardPack{Cards: []ir.Card{
		{ID: 77774001, CardType: "spell", PlayEffects: body},
		{ID: 77774002, CardType: "follower", Stats: &ir.Stats{Attack: 1, Life: 1}, Intrinsic: []string{"barrier"}, Abilities: []ir.Ability{
			{ID: strings.Repeat("b", 32), Trigger: ir.SimpleTrigger{Kind: "lastwords"}, Body: []ir.Effect{
				ir.CardEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("c", 32)}, Kind: "summon", Owner: "own", Count: 1, CardID: 77774003, Output: "summoned"},
			}},
		}},
		{ID: 77774003, CardType: "follower", Stats: &ir.Stats{Attack: 1, Life: 5}},
	}}
	state := testState()
	sourceID := strings.Repeat("1", 32)
	state.Players["own"] = withInstance(state.Players["own"], "hand", ir.TestInstance{InstanceID: sourceID, CardID: 77774001, DeclaredType: "spell"})
	for n, id := range []string{strings.Repeat("9", 32), strings.Repeat("8", 32), strings.Repeat("7", 32)} {
		cardID := 77774002
		if n == 2 {
			cardID = 77774003
		}
		state.Players["oppo"] = withInstance(state.Players["oppo"], "field", ir.TestInstance{InstanceID: id, CardID: cardID, DeclaredType: "follower"})
	}
	session, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	step := session.SubmitAs(strings.Repeat("d", 32), "own", SimulatorCommand{Kind: "play", Source: sourceID})
	if step.Status != StatusSuspended {
		t.Fatal(step)
	}
	encoded, err := session.EncodeContinuation()
	if err != nil {
		t.Fatal(err)
	}
	saved, err := DecodeContinuation(encoded)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := RestoreSession(pack, saved)
	if err != nil {
		t.Fatal(err)
	}
	choice := ChoiceResponse{RequestID: step.Choice.RequestID, ActionID: step.Choice.ActionID, StateRevision: step.Choice.StateRevision, SelectedOptionID: 1}
	for _, run := range []*Session{session, restored} {
		if result := run.Resume(choice); result.Status != StatusCompleted {
			t.Fatal(result)
		}
		if len(run.g.oppo.field) != 2 || len(run.g.oppo.destroyed) != 3 {
			t.Fatal("repeated allocation did not recompute survivors")
		}
		for _, token := range run.g.oppo.field {
			if token.life != 5 {
				t.Fatal("lastwords summon received remaining distributed damage")
			}
		}
		var batches []uint64
		var damage []int
		for _, event := range run.g.events {
			if event.Kind == "destroyed" {
				batches = append(batches, event.BatchID)
			}
			if event.Kind == "damaged" {
				damage = append(damage, event.Actual)
			}
		}
		if len(batches) != 3 || batches[0] == 0 || batches[0] != batches[1] || batches[1] == batches[2] {
			t.Fatalf("incorrect simultaneous deaths: %v", batches)
		}
		if !reflect.DeepEqual(damage, []int{0, 0, 1, 1, 1, 1, 3}) {
			t.Fatalf("incorrect repeated shield allocation: %v", damage)
		}
	}
	if !reflect.DeepEqual(session.g.snapshot(), restored.g.snapshot()) {
		t.Fatal("restoration changed distribution order or outcome")
	}
}

func TestDistributionBudgetFailureDoesNotUsePartialTargetSet(t *testing.T) {
	pack := &ir.CardPack{Cards: []ir.Card{{ID: 77774001, CardType: "follower", Stats: &ir.Stats{Attack: 1, Life: 2}}}}
	state := testState()
	for n := 0; n < 2; n++ {
		state.Players["oppo"] = withInstance(state.Players["oppo"], "field", ir.TestInstance{InstanceID: fmt.Sprintf("%032x", n+1), CardID: 77774001, DeclaredType: "follower"})
	}
	session, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	policy := session.budgetPolicy
	policy.QueryVisits = 1
	session.g.budget = &budgetTracker{policy: policy}
	before := session.g.snapshot()
	session.g.execTargetEffect(ir.TargetEffect{Kind: "damage", DamageType: "effect", Distribution: "field_entry_order", Amount: 7,
		Target:   ir.ZoneRef{Kind: "zone", Side: "oppo", Zone: "field", Member: "follower"},
		Overflow: ir.LeaderRef{Kind: "leader", Side: "oppo", ValueType: "leader"}}, nil, frame{})
	if !session.g.budget.exceeded || !reflect.DeepEqual(before, session.g.snapshot()) {
		t.Fatal("partial target query applied damage or overflow")
	}
}
