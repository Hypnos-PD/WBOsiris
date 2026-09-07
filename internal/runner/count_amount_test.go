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

func countCardPack(t *testing.T) *ir.CardPack {
	t.Helper()
	root := filepath.Join("..", "..")
	loaded := project.LoadWithRoot([]string{
		filepath.Join(root, "cards", "10001", "10111130.wbo"),
		filepath.Join(root, "cards", "90000", "90001110.wbo"),
	}, true, root)
	if loaded.HasErrors() {
		t.Fatal(loaded.Diagnostics)
	}
	pack, _, err := project.BuildRuntimePacks(loaded)
	if err != nil {
		t.Fatal(err)
	}
	return pack
}

func TestCompiledFairyBeastCountsHandAfterDrawing(t *testing.T) {
	pack := countCardPack(t)
	for _, side := range []string{"own", "oppo"} {
		for _, tc := range []struct{ held, life, want int }{
			{0, 10, 11}, {3, 10, 14}, {8, 10, 19}, {3, 19, 20},
		} {
			t.Run(fmt.Sprintf("%s/held%d/life%d", side, tc.held, tc.life), func(t *testing.T) {
				state := testState()
				state.Turn.Active = side
				actor := state.Players[side]
				actor.PP, actor.MaxPP, actor.Leader.Life = 8, 8, tc.life
				sourceID := strings.Repeat("1", 32)
				actor = withInstance(actor, "hand", ir.TestInstance{InstanceID: sourceID, CardID: 10111130, DeclaredType: "follower"})
				for n := 0; n < tc.held; n++ {
					actor = withInstance(actor, "hand", ir.TestInstance{InstanceID: fmt.Sprintf("%032x", n+100), CardID: 90001110, DeclaredType: "follower"})
				}
				state.Players[side] = withInstance(actor, "deck", ir.TestInstance{InstanceID: strings.Repeat("2", 32), CardID: 90001110, DeclaredType: "follower"})
				session, err := NewSession(pack, state, 1)
				if err != nil {
					t.Fatal(err)
				}
				result := session.SubmitAs(strings.Repeat("a", 32), side, SimulatorCommand{Kind: "play", Source: sourceID})
				if result.Status != StatusCompleted {
					t.Fatal(result)
				}
				view, _ := session.View(side)
				if view.Own.LeaderLife != tc.want || len(view.Own.Hand) != tc.held+1 || len(view.Own.Field) != 1 || view.Own.PP != 0 || view.Oppo.LeaderLife != 20 {
					t.Fatalf("wrong post-draw count: %#v", view)
				}
			})
		}
	}
}

func TestCountDamageSnapshotsFilteredSourceBeforeAnyTargetChanges(t *testing.T) {
	pack := &ir.CardPack{Cards: []ir.Card{
		{ID: 77773001, CardType: "follower", Stats: &ir.Stats{Attack: 1, Life: 4}, Traits: []string{"golem", "officer"}},
		{ID: 77773002, CardType: "follower", Stats: &ir.Stats{Attack: 1, Life: 4}, Traits: []string{"golem"}},
	}}
	for _, side := range []string{"own", "oppo"} {
		t.Run(side, func(t *testing.T) {
			state := testState()
			ids := []string{strings.Repeat("1", 32), strings.Repeat("2", 32), strings.Repeat("3", 32)}
			for n, id := range ids {
				cardID := 77773001
				if n == 2 {
					cardID = 77773002
				}
				state.Players[side] = withInstance(state.Players[side], "field", ir.TestInstance{InstanceID: id, CardID: cardID, DeclaredType: "follower"})
			}
			session, err := NewSession(pack, state, 1)
			if err != nil {
				t.Fatal(err)
			}
			zone := ir.ZoneRef{Kind: "zone", Side: "own", Zone: "field", Member: "follower"}
			session.g.execTargetEffect(ir.TargetEffect{
				Kind: "damage", DamageType: "effect", Target: zone,
				Predicate: ir.FieldPredicate{Kind: "has_trait", Trait: "officer"},
				AmountExpr: &ir.CountExpr{Kind: "count", Source: ir.FilterRef{Kind: "filter", Source: zone,
					Predicate: ir.AndPredicate{Kind: "and", Terms: []ir.Predicate{
						ir.FieldPredicate{Kind: "has_trait", Trait: "golem"},
						ir.FieldPredicate{Kind: "compare", Field: "life", Op: "ge", Value: 4},
					}},
				}},
			}, session.g.instances[ids[0]], frame{})
			for n, want := range []int{1, 1, 4} {
				if got := session.g.instances[ids[n]].life; got != want {
					t.Errorf("target %d life=%d want=%d", n, got, want)
				}
			}
		})
	}
}

func TestCountAmountEmptySourceAndBudgetExhaustion(t *testing.T) {
	pack := countCardPack(t)
	for _, held := range []int{0, 2} {
		t.Run(fmt.Sprintf("held%d", held), func(t *testing.T) {
			state := testState()
			actor := state.Players["own"]
			actor.Leader.Life = 10
			for n := 0; n < held; n++ {
				actor = withInstance(actor, "hand", ir.TestInstance{InstanceID: fmt.Sprintf("%032x", n+1), CardID: 90001110, DeclaredType: "follower"})
			}
			state.Players["own"] = actor
			session, err := NewSession(pack, state, 1)
			if err != nil {
				t.Fatal(err)
			}
			policy := session.budgetPolicy
			policy.QueryVisits = 3
			session.g.budget = &budgetTracker{policy: policy}
			before := session.g.snapshot()
			session.g.execTargetEffect(ir.TargetEffect{
				Kind: "heal", Target: ir.LeaderRef{Kind: "leader", Side: "own", ValueType: "leader"},
				AmountExpr: &ir.CountExpr{Kind: "count", Source: ir.FilterRef{Kind: "filter",
					Source:    ir.ZoneRef{Kind: "zone", Side: "own", Zone: "hand", Member: "follower"},
					Predicate: ir.FieldPredicate{Kind: "has_type", CardType: "follower"},
				}},
			}, nil, frame{})
			if session.g.own.leaderLife != 10 || session.g.rng.Consumed() != 0 {
				t.Fatal("empty or incomplete count healed leader or consumed randomness")
			}
			if held > 0 && (!session.g.budget.exceeded || !reflect.DeepEqual(before, session.g.snapshot())) {
				t.Fatal("query exhaustion applied partial count")
			}
			if held == 0 && session.g.budget.exceeded {
				t.Fatal("empty count exhausted query budget")
			}
		})
	}
}

func TestCountAmountAfterRestoredChoiceReadsResumedState(t *testing.T) {
	pack := countCardPack(t)
	for n := range pack.Cards {
		card := &pack.Cards[n]
		if card.ID != 10111130 {
			continue
		}
		body := card.Abilities[0].Body
		base := ir.EffectBase(body[0])
		base.ID = strings.Repeat("c", 32)
		draw := body[0].(ir.DrawEffect)
		draw.ID = strings.Repeat("d", 32)
		card.Abilities[0].Body = []ir.Effect{body[0], ir.ModeEffect{NodeBase: base, Kind: "mode", Options: []ir.ModeOption{
			{ID: 1, Body: []ir.Effect{draw}, Origin: base.Origin}, {ID: 2, Body: []ir.Effect{}, Origin: base.Origin},
		}}, body[1]}
	}
	encodedPack, err := ir.EncodeCardPack(*pack)
	if err != nil {
		t.Fatal(err)
	}
	pack, err = ir.DecodeCardPack(encodedPack)
	if err != nil {
		t.Fatal(err)
	}
	state := testState()
	actor := state.Players["own"]
	actor.PP, actor.MaxPP, actor.Leader.Life = 8, 8, 10
	sourceID := strings.Repeat("1", 32)
	actor = withInstance(actor, "hand", ir.TestInstance{InstanceID: sourceID, CardID: 10111130, DeclaredType: "follower"})
	for _, id := range []string{strings.Repeat("2", 32), strings.Repeat("3", 32)} {
		actor = withInstance(actor, "deck", ir.TestInstance{InstanceID: id, CardID: 90001110, DeclaredType: "follower"})
	}
	state.Players["own"] = actor
	session, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	step := session.SubmitAs(strings.Repeat("a", 32), "own", SimulatorCommand{Kind: "play", Source: sourceID})
	if step.Status != StatusSuspended || len(session.g.own.hand) != 1 || session.g.own.leaderLife != 10 {
		t.Fatalf("wrong state before choice: %#v", step)
	}
	encoded, err := session.EncodeContinuation()
	if err != nil {
		t.Fatal(err)
	}
	continuation, err := DecodeContinuation(encoded)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := RestoreSession(pack, continuation)
	if err != nil {
		t.Fatal(err)
	}
	choice := ChoiceResponse{RequestID: step.Choice.RequestID, ActionID: step.Choice.ActionID, StateRevision: step.Choice.StateRevision, SelectedOptionID: 1}
	for _, run := range []*Session{session, restored} {
		if result := run.Resume(choice); result.Status != StatusCompleted {
			t.Fatal(result)
		}
		if run.g.own.leaderLife != 12 || len(run.g.own.hand) != 2 {
			t.Fatal("count used state from before resumed draw")
		}
	}
	if !reflect.DeepEqual(session.g.snapshot(), restored.g.snapshot()) {
		t.Fatal("restoration changed count effect outcome")
	}
}
