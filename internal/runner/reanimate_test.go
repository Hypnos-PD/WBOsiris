package runner

import (
	"fmt"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"wbo/internal/ir"
	"wbo/internal/project"
	"wbo/internal/ruleset"
)

func reanimateFixture() (*ir.CardPack, ir.State) {
	pack := &ir.CardPack{Cards: []ir.Card{
		{ID: 77771001, CardType: "spell", PlayEffects: []ir.Effect{
			ir.CardEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("a", 32)}, Kind: "reanimate", Owner: "own", MaxCost: 4, TieBreak: "random", Output: "summoned"},
		}},
		{ID: 77771002, CardType: "follower", Cost: 4, Stats: &ir.Stats{Attack: 3, Life: 2}, Traits: []string{"officer"}, Intrinsic: []string{"ward"}, Abilities: []ir.Ability{
			{ID: strings.Repeat("b", 32), Trigger: ir.SimpleTrigger{Kind: "fanfare"}, Body: []ir.Effect{ir.TargetEffect{Kind: "damage", Target: ir.LeaderRef{Kind: "leader", Side: "oppo"}, Amount: 9}}},
		}},
		{ID: 77771003, CardType: "follower", Cost: 4, Stats: &ir.Stats{Attack: 2, Life: 3}},
		{ID: 77771004, CardType: "follower", Cost: 2, Stats: &ir.Stats{Attack: 1, Life: 1}},
		{ID: 77771005, CardType: "follower", Cost: 5, Stats: &ir.Stats{Attack: 5, Life: 5}},
		{ID: 77771006, CardType: "amulet", Cost: 4},
		{ID: 77771007, CardType: "follower", Cost: 0, Stats: &ir.Stats{Attack: 1, Life: 1}},
	}}
	return pack, testState()
}

func TestReanimateWeightedDestructionHistory(t *testing.T) {
	for _, side := range []string{"own", "oppo"} {
		t.Run(side, func(t *testing.T) {
			pack, state := reanimateFixture()
			state.Turn.Active = side
			sourceID := strings.Repeat("1", 32)
			actor := withInstance(state.Players[side], "hand", ir.TestInstance{InstanceID: sourceID, CardID: 77771001, DeclaredType: "spell"})
			// The two-cost, five-cost and amulet records are not eligible.
			history := []int{77771004, 77771002, 77771003, 77771003, 77771005, 77771003, 77771006, 77771003}
			for n, id := range history {
				kind := "follower"
				if id == 77771006 {
					kind = "amulet"
				}
				actor = withInstance(actor, "destroyed", ir.TestInstance{InstanceID: fmt.Sprintf("%032x", n+100), CardID: id, DeclaredType: kind})
			}
			state.Players[side] = actor
			// The opponent's higher-cost record must never enter the lottery.
			other := oppositeSide(side)
			state.Players[other] = withInstance(state.Players[other], "destroyed", ir.TestInstance{InstanceID: strings.Repeat("9", 32), CardID: 77771005, DeclaredType: "follower"})
			seen := map[int]bool{}
			for seed := uint64(0); seed < 64; seed++ {
				session, err := NewSession(pack, state, seed)
				if err != nil {
					t.Fatal(err)
				}
				before := session.g.snapshot()
				session.LegalActionsFor(side)
				if !reflect.DeepEqual(before, session.g.snapshot()) {
					t.Fatal("legality query changed history or consumed random state")
				}
				result := session.SubmitAs(strings.Repeat("c", 32), side, SimulatorCommand{Kind: "play", Source: sourceID})
				if result.Status != StatusCompleted {
					t.Fatal(result)
				}
				view, _ := session.View(side)
				// QA: one ticket for one identity, four for the other, not 50/50.
				want := []int{77771002, 77771003, 77771003, 77771003, 77771003}[ruleset.NewRNG(seed).Index(5)]
				if len(view.Own.Field) != 1 || view.Own.Field[0].CardID != want || session.g.rng.Consumed() != 1 {
					t.Fatalf("seed %d: field=%#v, want card %d with one draw", seed, view.Own.Field, want)
				}
				seen[want] = true
				if !slices.Contains(view.Own.Field[0].Traits, "departed") || view.Oppo.LeaderLife != 20 || len(view.Own.Destroyed) != len(history) {
					t.Fatalf("missing departed, fanfare ran, or history was consumed: %#v", view)
				}
			}
			if len(seen) != 2 {
				t.Fatal("test seeds did not exercise both identities")
			}
		})
	}
}

func TestReanimateEmptyUniqueAndFullField(t *testing.T) {
	for _, name := range []string{"empty", "zero_cost", "unique", "full_field"} {
		t.Run(name, func(t *testing.T) {
			pack, state := reanimateFixture()
			actor := state.Players["own"]
			if name != "empty" {
				id := 77771002
				if name == "zero_cost" {
					id = 77771007
				}
				actor = withInstance(actor, "destroyed", ir.TestInstance{InstanceID: strings.Repeat("1", 32), CardID: id, DeclaredType: "follower"})
			}
			if name == "full_field" {
				actor = withInstance(actor, "destroyed", ir.TestInstance{InstanceID: strings.Repeat("2", 32), CardID: 77771003, DeclaredType: "follower"})
				for n := 0; n < fieldLimit; n++ {
					actor = withInstance(actor, "field", ir.TestInstance{InstanceID: fmt.Sprintf("%032x", n+100), CardID: 77771004, DeclaredType: "follower"})
				}
			}
			state.Players["own"] = actor
			session, err := NewSession(pack, state, 1)
			if err != nil {
				t.Fatal(err)
			}
			bindings := frame{"summoned": bindEntities(&instance{id: "stale"})}
			maxCost := 4
			if name == "zero_cost" {
				maxCost = 0
			}
			session.g.execCardEffect(ir.CardEffect{Kind: "reanimate", Owner: "own", MaxCost: maxCost, Output: "summoned"}, nil, bindings)
			if session.g.rng.Consumed() != 0 {
				t.Fatal("reanimate consumed RNG without a realizable tie")
			}
			if name == "empty" || name == "full_field" {
				if len(bindings["summoned"]) != 0 {
					t.Fatal("failed summon retained a stale output binding")
				}
			} else if len(bindings["summoned"]) != 1 || !session.g.instances[bindings["summoned"][0].InstanceID].departed {
				t.Fatalf("unique candidate not reanimated: %#v", bindings)
			}
		})
	}
}

func TestReanimateFreshStateAndContinuation(t *testing.T) {
	pack, state := reanimateFixture()
	pack.Cards[0].PlayEffects = append(pack.Cards[0].PlayEffects,
		ir.ModeEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("d", 32)}, Kind: "mode", Options: []ir.ModeOption{{ID: 1, Body: []ir.Effect{
			ir.CardEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("e", 32)}, Kind: "reanimate", Owner: "own", MaxCost: 4, Output: "summoned"},
		}}}},
	)
	pack.Cards = append(pack.Cards, ir.Card{ID: 77771008, CardType: "follower", Stats: &ir.Stats{Attack: 1, Life: 1}, Abilities: []ir.Ability{{
		ID: strings.Repeat("f", 32), Trigger: ir.EventTrigger{Kind: "event", Event: "follower_summoned", Side: "own", SubjectType: "follower", Predicate: ir.FieldPredicate{Kind: "has_trait", Trait: "departed"}},
		Body: []ir.Effect{ir.TargetEffect{Kind: "heal", Target: ir.LeaderRef{Kind: "leader", Side: "own"}, Amount: 1}},
	}}})
	sourceID, deadID := strings.Repeat("1", 32), strings.Repeat("2", 32)
	actor := state.Players["own"]
	actor.Leader.Life = 10
	actor = withInstance(actor, "hand", ir.TestInstance{InstanceID: sourceID, CardID: 77771001, DeclaredType: "spell"})
	actor = withInstance(actor, "field", ir.TestInstance{InstanceID: strings.Repeat("3", 32), CardID: 77771008, DeclaredType: "follower"})
	actor = withInstance(actor, "destroyed", ir.TestInstance{InstanceID: deadID, CardID: 77771002, DeclaredType: "follower"})
	state.Players["own"] = withInstance(actor, "destroyed", ir.TestInstance{InstanceID: strings.Repeat("5", 32), CardID: 77771002, DeclaredType: "follower"})
	session, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	dead := session.g.instances[deadID]
	dead.attack, dead.life, dead.cost, dead.evolved, dead.superEvolved = 20, 30, 9, true, true
	dead.abilities["storm"] = true
	result := session.Submit(strings.Repeat("4", 32), SimulatorCommand{Kind: "play", Source: sourceID})
	if result.Status != StatusSuspended {
		t.Fatalf("did not reach post-reanimate choice: %#v", result)
	}
	view, _ := session.View("own")
	summoned := view.Own.Field[1]
	if summoned.InstanceID == deadID || summoned.Attack != 3 || summoned.Life != 2 || summoned.Cost != 4 || summoned.Evolved || summoned.SuperEvolved || slices.Contains(summoned.Keywords, "storm") || !slices.Contains(summoned.Keywords, "ward") || !slices.Contains(summoned.Traits, "officer") || !slices.Contains(summoned.Traits, "departed") || len(session.g.triggers) != 1 {
		t.Fatalf("reanimated follower retained old state or listener missed departed: %#v", view.Own)
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
	if !reflect.DeepEqual(session.g.snapshot(), restored.g.snapshot()) {
		t.Fatal("continuation changed reanimated state")
	}
	choice := result.Choice
	response := ChoiceResponse{RequestID: choice.RequestID, ActionID: choice.ActionID, StateRevision: choice.StateRevision, SelectedOptionID: 1}
	for _, s := range []*Session{session, restored} {
		if step := s.Resume(response); step.Status != StatusCompleted {
			t.Fatal(step)
		}
		v, _ := s.View("own")
		if len(v.Own.Field) != 3 || v.Own.LeaderLife != 12 || !slices.Contains(v.Own.Field[1].Traits, "departed") || !slices.Contains(v.Own.Field[2].Traits, "departed") {
			t.Fatalf("second reanimate or restored departed listener failed: %#v", v.Own)
		}
	}
	if !reflect.DeepEqual(session.g.snapshot(), restored.g.snapshot()) {
		t.Fatal("restored execution diverged")
	}
	if restored.g.rng.Consumed() != 2 {
		t.Fatal("continuation did not preserve both random draws")
	}
}

func TestReanimateBudgetExhaustionDoesNotSelectPartialHistory(t *testing.T) {
	for _, resource := range []string{"queries", "candidates"} {
		t.Run(resource, func(t *testing.T) {
			pack, state := reanimateFixture()
			actor := state.Players["own"]
			actor = withInstance(actor, "destroyed", ir.TestInstance{InstanceID: strings.Repeat("1", 32), CardID: 77771002, DeclaredType: "follower"})
			state.Players["own"] = withInstance(actor, "destroyed", ir.TestInstance{InstanceID: strings.Repeat("2", 32), CardID: 77771003, DeclaredType: "follower"})
			session, err := NewSession(pack, state, 1)
			if err != nil {
				t.Fatal(err)
			}
			policy := session.budgetPolicy
			if resource == "queries" {
				policy.QueryVisits = 1
			} else {
				policy.Candidates = 1
			}
			session.g.budget = &budgetTracker{policy: policy}
			bindings := frame{}
			session.g.execCardEffect(ir.CardEffect{Kind: "reanimate", Owner: "own", MaxCost: 4, Output: "summoned"}, nil, bindings)
			if !session.g.budget.exceeded || len(session.g.own.field) != 0 || len(bindings["summoned"]) != 0 || session.g.rng.Consumed() != 0 {
				t.Fatal("budget exhaustion selected from an incomplete candidate set")
			}
		})
	}
}

func TestCompiledReanimateFollower(t *testing.T) {
	root := filepath.Join("..", "..")
	loaded := project.LoadWithRoot([]string{filepath.Join(root, "cards", "10001", "10151140.wbo"), filepath.Join(root, "cards", "10001", "10103110.wbo")}, true, root)
	pack, _, err := project.BuildRuntimePacks(loaded)
	if err != nil {
		t.Fatal(err)
	}
	state := testState()
	actor := state.Players["own"]
	actor.PP, actor.MaxPP = 6, 6
	sourceID := strings.Repeat("1", 32)
	actor = withInstance(actor, "hand", ir.TestInstance{InstanceID: sourceID, CardID: 10151140, DeclaredType: "follower"})
	state.Players["own"] = withInstance(actor, "destroyed", ir.TestInstance{InstanceID: strings.Repeat("2", 32), CardID: 10103110, DeclaredType: "follower"})
	session, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	if result := session.Submit(strings.Repeat("3", 32), SimulatorCommand{Kind: "play", Source: sourceID}); result.Status != StatusCompleted {
		t.Fatal(result)
	}
	view, _ := session.View("own")
	if len(view.Own.Field) != 2 || view.Own.Field[1].CardID != 10103110 || !slices.Contains(view.Own.Field[1].Traits, "departed") || view.Own.PP != 0 {
		t.Fatalf("compiled reanimate 4 did not summon highest eligible lower-cost follower: %#v", view.Own)
	}
}

func TestReanimateContinuationPreservesRepeatedInstanceDeaths(t *testing.T) {
	pack, state := reanimateFixture()
	pack.Cards[0].PlayEffects = append([]ir.Effect{
		ir.ModeEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("d", 32)}, Kind: "mode", Options: []ir.ModeOption{{ID: 1}}},
	}, pack.Cards[0].PlayEffects...)
	sourceID, deadID := strings.Repeat("1", 32), strings.Repeat("2", 32)
	actor := withInstance(state.Players["own"], "hand", ir.TestInstance{InstanceID: sourceID, CardID: 77771001, DeclaredType: "spell"})
	state.Players["own"] = withInstance(actor, "field", ir.TestInstance{InstanceID: deadID, CardID: 77771003, DeclaredType: "follower"})
	session, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	dead := session.g.instances[deadID]
	session.g.destroyByEffect([]*instance{dead})
	session.g.returnCard(dead, "hand")
	session.g.move(dead, "field")
	session.g.destroyByEffect([]*instance{dead})
	if len(session.g.own.destroyed) != 2 {
		t.Fatal("second destruction was not recorded")
	}
	step := session.Submit(strings.Repeat("3", 32), SimulatorCommand{Kind: "play", Source: sourceID})
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
	if len(restored.g.own.destroyed) != 2 || restored.g.own.destroyed[0] != restored.g.own.destroyed[1] {
		t.Fatal("restoration deduplicated destruction records")
	}
	choice := step.Choice
	result := restored.Resume(ChoiceResponse{RequestID: choice.RequestID, ActionID: choice.ActionID, StateRevision: choice.StateRevision, SelectedOptionID: 1})
	if result.Status != StatusCompleted || restored.g.rng.Consumed() != 1 || len(restored.g.own.field) != 1 {
		t.Fatalf("repeated destruction tickets did not survive restoration: %#v", result)
	}
}
