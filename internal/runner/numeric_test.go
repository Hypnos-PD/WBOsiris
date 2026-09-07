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

func numericCardPack(t *testing.T) *ir.CardPack {
	t.Helper()
	root := filepath.Join("..", "..")
	var paths []string
	for _, file := range []string{"10001/10113140.wbo", "10001/10131110.wbo", "10001/10132120.wbo", "90000/90031110.wbo", "10000/10031310.wbo"} {
		paths = append(paths, filepath.Join(root, "cards", file))
	}
	loaded := project.LoadWithRoot(paths, true, root)
	if loaded.HasErrors() {
		t.Fatal(loaded.Diagnostics)
	}
	pack, _, err := project.BuildRuntimePacks(loaded)
	if err != nil {
		t.Fatal(err)
	}
	return pack
}

func TestCompiledNumericCardEffects(t *testing.T) {
	pack := numericCardPack(t)
	sourceID, otherID, enemyID, spellID := strings.Repeat("1", 32), strings.Repeat("2", 32), strings.Repeat("3", 32), strings.Repeat("4", 32)
	for _, side := range []string{"own", "oppo"} {
		for _, combo := range []int{0, 2, 7} {
			t.Run(fmt.Sprintf("%s/combo%d", side, combo), func(t *testing.T) {
				state := testState()
				state.Turn.Active = side
				actor := state.Players[side]
				actor.Combo, actor.PP, actor.MaxPP = combo, 10, 10
				state.Players[side] = withInstance(actor, "hand", ir.TestInstance{InstanceID: sourceID, CardID: 10113140, DeclaredType: "follower"})
				s, err := NewSession(pack, state, 1)
				if err != nil {
					t.Fatal(err)
				}
				if step := s.SubmitAs(strings.Repeat("a", 32), side, SimulatorCommand{Kind: "play", Source: sourceID}); step.Status != StatusCompleted {
					t.Fatal(step)
				}
				card := s.g.instances[sourceID]
				if card.attack != combo+1 || card.life != 2 || !card.abilities["storm"] || s.g.player(side).combo != combo+1 {
					t.Fatalf("wrong combo buff: %#v", card)
				}
			})
		}
		t.Run(side+"/boost_then_choose_restore", func(t *testing.T) {
			state := testState()
			state.Turn.Active = side
			actor := state.Players[side]
			actor.PP, actor.MaxPP = 10, 10
			actor = withInstance(actor, "hand", ir.TestInstance{InstanceID: sourceID, CardID: 10131110, DeclaredType: "follower"})
			emmylouID := strings.Repeat("5", 32)
			actor = withInstance(actor, "hand", ir.TestInstance{InstanceID: emmylouID, CardID: 10132120, DeclaredType: "follower"})
			actor = withInstance(actor, "hand", ir.TestInstance{InstanceID: spellID, CardID: 10031310, DeclaredType: "spell"})
			state.Players[side] = withInstance(actor, "deck", ir.TestInstance{InstanceID: otherID, CardID: 10131110, DeclaredType: "follower"})
			enemy := oppositeSide(side)
			state.Players[enemy] = withInstance(state.Players[enemy], "field", ir.TestInstance{InstanceID: enemyID, CardID: 90031110, DeclaredType: "follower", Overrides: ir.InstanceOverrides{Stats: &ir.Stats{Attack: 2, Life: 10}}})
			s, err := NewSession(pack, state, 1)
			if err != nil {
				t.Fatal(err)
			}
			if step := s.SubmitAs(strings.Repeat("a", 32), side, SimulatorCommand{Kind: "play", Source: spellID}); step.Status != StatusCompleted {
				t.Fatal(step)
			}
			if s.g.instances[sourceID].attack != 2 || s.g.instances[otherID].attack != 1 || s.g.instances[emmylouID].cost != 4 {
				t.Fatal("spellboost affected wrong hand snapshot")
			}
			step := s.SubmitAs(strings.Repeat("b", 32), side, SimulatorCommand{Kind: "play", Source: sourceID})
			if step.Status != StatusSuspended {
				t.Fatal(step)
			}
			encoded, err := s.EncodeContinuation()
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
			choice := step.Choice
			for _, session := range []*Session{s, restored} {
				result := session.Resume(ChoiceResponse{RequestID: choice.RequestID, ActionID: choice.ActionID, StateRevision: choice.StateRevision, SelectedInstanceIDs: []string{enemyID}})
				if result.Status != StatusCompleted || session.g.instances[enemyID].life != 8 || session.g.instances[sourceID].life != 2 {
					t.Fatalf("wrong current attack damage: %#v", result)
				}
			}
			if !reflect.DeepEqual(s.g.snapshot(), restored.g.snapshot()) {
				t.Fatal("restored scalar amount diverged")
			}
		})
		for _, full := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/golem_full%t", side, full), func(t *testing.T) {
				state := testState()
				state.Turn.Active = side
				state.Turn.Number = 6
				actor := state.Players[side]
				actor.EP = 1
				actor = withInstance(actor, "field", ir.TestInstance{InstanceID: sourceID, CardID: 10132120, DeclaredType: "follower"})
				actor = withInstance(actor, "field", ir.TestInstance{InstanceID: otherID, CardID: 90031110, DeclaredType: "follower"})
				if full {
					for n := 0; n < 3; n++ {
						actor = withInstance(actor, "field", ir.TestInstance{InstanceID: fmt.Sprintf("%032x", n+10), CardID: 10131110, DeclaredType: "follower"})
					}
				}
				state.Players[side] = actor
				enemy := oppositeSide(side)
				state.Players[enemy] = withInstance(state.Players[enemy], "field", ir.TestInstance{InstanceID: enemyID, CardID: 90031110, DeclaredType: "follower", Overrides: ir.InstanceOverrides{Stats: &ir.Stats{Attack: 2, Life: 10}}})
				s, err := NewSession(pack, state, 1)
				if err != nil {
					t.Fatal(err)
				}
				step := s.SubmitAs(strings.Repeat("a", 32), side, SimulatorCommand{Kind: "evolve", Source: sourceID})
				want := 8
				if full {
					want = 9
				}
				if step.Status != StatusCompleted || s.g.instances[enemyID].life != want {
					t.Fatalf("summon-before-count: %#v life=%d", step, s.g.instances[enemyID].life)
				}
			})
		}
	}
}

func TestBuffNumericInputsShareOneSnapshot(t *testing.T) {
	pack := numericCardPack(t)
	state := testState()
	sourceID, otherID := strings.Repeat("1", 32), strings.Repeat("2", 32)
	actor := withInstance(state.Players["own"], "field", ir.TestInstance{InstanceID: sourceID, CardID: 90031110, DeclaredType: "follower", Overrides: ir.InstanceOverrides{Stats: &ir.Stats{Attack: 2, Life: 3}}})
	state.Players["own"] = withInstance(actor, "field", ir.TestInstance{InstanceID: otherID, CardID: 90031110, DeclaredType: "follower"})
	s, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	source, other := s.g.instances[sourceID], s.g.instances[otherID]
	s.g.execTargetEffect(ir.TargetEffect{Kind: "buff_stats", Target: ir.ZoneRef{Kind: "zone", Side: "own", Zone: "field", Member: "follower"}, AttackExpr: &ir.Scalar{Kind: "self_scalar", Field: "life"}, LifeExpr: &ir.Scalar{Kind: "self_scalar", Field: "attack"}}, source, frame{})
	if source.attack != 5 || source.life != 5 || other.attack != 5 || other.life != 4 {
		t.Fatalf("numeric inputs changed between targets: source=%d/%d other=%d/%d", source.attack, source.life, other.attack, other.life)
	}
	count := &ir.CountExpr{Kind: "count", Source: ir.ZoneRef{Kind: "zone", Side: "own", Zone: "field", Member: "follower"}}
	s.g.execTargetEffect(ir.TargetEffect{Kind: "buff_stats", Target: ir.SelfRef{Kind: "self"}, AttackExpr: &ir.NegateExpr{Kind: "negate", Value: count}, LifeExpr: count}, source, frame{})
	if source.attack != 3 || source.life != 7 {
		t.Fatal("signed count buff failed")
	}
	source.attack = -3
	s.g.execTargetEffect(ir.TargetEffect{Kind: "damage", Target: ir.LeaderRef{Kind: "leader", Side: "oppo"}, AmountExpr: &ir.Scalar{Kind: "self_scalar", Field: "attack"}}, source, frame{})
	if s.g.oppo.leaderLife != 20 || source.attack != -3 {
		t.Fatal("negative attack healed enemy or lost stored modifier")
	}
	policy := s.budgetPolicy
	policy.QueryVisits = 3
	s.g.budget = &budgetTracker{policy: policy}
	before := s.g.snapshot()
	s.g.execTargetEffect(ir.TargetEffect{Kind: "buff_stats", Target: ir.SelfRef{Kind: "self"}, AttackExpr: count, LifeExpr: count}, source, frame{})
	if !s.g.budget.exceeded || !reflect.DeepEqual(before, s.g.snapshot()) {
		t.Fatal("second numeric query exhaustion applied a partial buff")
	}
}
