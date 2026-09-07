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

func repeatCardPack(t *testing.T) *ir.CardPack {
	t.Helper()
	root := filepath.Join("..", "..")
	loaded := project.LoadWithRoot([]string{filepath.Join(root, "cards")}, true, root)
	if loaded.HasErrors() {
		t.Fatal(loaded.Diagnostics)
	}
	pack, _, err := project.BuildRuntimePacks(loaded)
	if err != nil {
		t.Fatal(err)
	}
	return pack
}

func TestCompiledRepeatedDamageDefersLastwords(t *testing.T) {
	pack := repeatCardPack(t)
	sourceID, enemyID := strings.Repeat("1", 32), strings.Repeat("2", 32)
	for _, side := range []string{"own", "oppo"} {
		for _, barrier := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/barrier%t", side, barrier), func(t *testing.T) {
				state := testState()
				state.Turn.Active = side
				state.Turn.Number = 6
				actor := state.Players[side]
				actor.EP = 1
				actor = withInstance(actor, "field", ir.TestInstance{InstanceID: sourceID, CardID: 10114130, DeclaredType: "follower"})
				for n := 0; n < 4; n++ {
					actor = withInstance(actor, "hand", ir.TestInstance{InstanceID: fmt.Sprintf("%032x", n+10), CardID: 90011110, DeclaredType: "follower"})
				}
				state.Players[side] = actor
				enemy := ir.TestInstance{InstanceID: enemyID, CardID: 10151130, DeclaredType: "follower"}
				if barrier {
					enemy.Overrides.Keywords = []string{"barrier"}
				}
				other := oppositeSide(side)
				state.Players[other] = withInstance(state.Players[other], "field", enemy)
				s, err := NewSession(pack, state, 1)
				if err != nil {
					t.Fatal(err)
				}
				result := s.SubmitAs(strings.Repeat("a", 32), side, SimulatorCommand{Kind: "evolve", Source: sourceID})
				if result.Status != StatusCompleted {
					t.Fatal(result)
				}
				foe := s.g.player(other)
				if len(foe.field) != 2 || s.g.instances[enemyID].zone != "graveyard" {
					t.Fatal("repeated damage hit a lastwords summon")
				}
				for _, card := range foe.field {
					if card.card.ID != 90051110 || card.life != 1 {
						t.Fatal("lastwords summon was damaged")
					}
				}
				wantRNG := uint64(2)
				if barrier {
					wantRNG = 3
				}
				if s.g.rng.Consumed() != wantRNG {
					t.Fatalf("random count=%d want=%d", s.g.rng.Consumed(), wantRNG)
				}
				lastDamage, firstSummon := -1, -1
				for n, event := range s.g.events {
					if event.Kind == "damaged" {
						lastDamage = n
					}
					if event.Kind == "follower_summoned" && firstSummon < 0 {
						firstSummon = n
					}
				}
				if lastDamage < 0 || firstSummon <= lastDamage {
					t.Fatal("lastwords interleaved into repeated ability")
				}
			})
		}
	}
}

func TestCompiledRepeatCardCostsAndZeroCount(t *testing.T) {
	pack := repeatCardPack(t)
	sourceID, enemyID := strings.Repeat("1", 32), strings.Repeat("2", 32)
	for _, side := range []string{"own", "oppo"} {
		for _, pixies := range []int{0, 3} {
			t.Run(fmt.Sprintf("%s/pixies%d", side, pixies), func(t *testing.T) {
				state := testState()
				state.Turn.Active = side
				state.Turn.Number = 6
				actor := state.Players[side]
				actor.PP, actor.MaxPP, actor.EP = 10, 10, 1
				actor = withInstance(actor, "hand", ir.TestInstance{InstanceID: sourceID, CardID: 10114130, DeclaredType: "follower"})
				actor = withInstance(actor, "hand", ir.TestInstance{InstanceID: strings.Repeat("3", 32), CardID: 90021110, DeclaredType: "follower"})
				for n := 0; n < pixies; n++ {
					actor = withInstance(actor, "hand", ir.TestInstance{InstanceID: fmt.Sprintf("%032x", n+10), CardID: 90011110, DeclaredType: "follower"})
				}
				state.Players[side] = actor
				enemy := oppositeSide(side)
				state.Players[enemy] = withInstance(state.Players[enemy], "field", ir.TestInstance{InstanceID: enemyID, CardID: 10011130, DeclaredType: "follower"})
				s, err := NewSession(pack, state, 1)
				if err != nil {
					t.Fatal(err)
				}
				if step := s.SubmitAs(strings.Repeat("a", 32), side, SimulatorCommand{Kind: "play", Source: sourceID}); step.Status != StatusCompleted {
					t.Fatal(step)
				}
				if s.g.instances[sourceID].attack != 2+pixies || s.g.instances[sourceID].life != 2+pixies {
					t.Fatal("fanfare counted non-pixie cards")
				}
				if step := s.SubmitAs(strings.Repeat("b", 32), side, SimulatorCommand{Kind: "evolve", Source: sourceID}); step.Status != StatusCompleted {
					t.Fatal(step)
				}
				if s.g.instances[enemyID].life != 4-pixies || s.g.rng.Consumed() != uint64(pixies) {
					t.Fatal("wrong repeat count")
				}
			})
		}
		for _, shadows := range []int{0, 2, 5} {
			t.Run(fmt.Sprintf("%s/shadows%d", side, shadows), func(t *testing.T) {
				state := testState()
				state.Turn.Active = side
				state.Turn.Number = 6
				actor := state.Players[side]
				actor.PP, actor.MaxPP, actor.EP, actor.Shadows = 10, 10, 1, shadows
				state.Players[side] = withInstance(actor, "hand", ir.TestInstance{InstanceID: sourceID, CardID: 10153120, DeclaredType: "follower"})
				enemy := oppositeSide(side)
				state.Players[enemy] = withInstance(state.Players[enemy], "field", ir.TestInstance{InstanceID: enemyID, CardID: 10012120, DeclaredType: "follower"})
				s, err := NewSession(pack, state, 1)
				if err != nil {
					t.Fatal(err)
				}
				if step := s.SubmitAs(strings.Repeat("a", 32), side, SimulatorCommand{Kind: "play", Source: sourceID}); step.Status != StatusCompleted {
					t.Fatal(step)
				}
				if s.g.player(side).shadows != shadows+2 || len(s.g.player(side).graveyard) != 0 {
					t.Fatal("shadow gain fabricated graveyard cards")
				}
				if step := s.SubmitAs(strings.Repeat("b", 32), side, SimulatorCommand{Kind: "evolve", Source: sourceID}); step.Status != StatusCompleted {
					t.Fatal(step)
				}
				wantShadows, wantLife, wantRNG := shadows+2, 6, uint64(0)
				if shadows >= 2 {
					wantShadows -= 4
					wantLife -= 4
					wantRNG = 2
				}
				if s.g.player(side).shadows != wantShadows || s.g.instances[enemyID].life != wantLife || s.g.rng.Consumed() != wantRNG {
					t.Fatal("necromancy charged per repetition or ran unpaid")
				}
			})
		}
	}
}

func repeatedChoicePack() *ir.CardPack {
	node := func(n int) ir.NodeBase { return ir.NodeBase{ID: fmt.Sprintf("%032x", n)} }
	target := ir.BindingRef{Kind: "binding", Name: "target"}
	return &ir.CardPack{Cards: []ir.Card{
		{ID: 77774001, CardType: "spell", Cost: 1, PlayEffects: []ir.Effect{
			ir.SelectionEffect{NodeBase: node(1), Kind: "random_choose", Policy: "random", Binding: "target", Source: ir.ZoneRef{Kind: "zone", Side: "own", Zone: "field", Member: "follower"}},
			ir.RepeatEffect{NodeBase: node(2), Kind: "repeat", TimesExpr: &ir.CountExpr{Kind: "count", Source: ir.ZoneRef{Kind: "zone", Side: "own", Zone: "hand"}}, Body: []ir.Effect{
				ir.RepeatEffect{NodeBase: node(3), Kind: "repeat", Times: 2, Body: []ir.Effect{
					ir.SelectionEffect{NodeBase: node(4), Kind: "choose", Policy: "optional", Binding: "target", Source: ir.ZoneRef{Kind: "zone", Side: "oppo", Zone: "field", Member: "follower"}},
					ir.TargetEffect{NodeBase: node(5), Kind: "damage", DamageType: "effect", Target: target, Amount: 1},
					ir.DrawEffect{NodeBase: node(6), Kind: "draw", Owner: "own", SourceZone: "deck", Count: 1, Output: "drawn"},
				}},
			}},
			ir.TargetEffect{NodeBase: node(7), Kind: "damage", DamageType: "effect", Target: target, Amount: 3},
		}},
		{ID: 77774002, CardType: "follower", Stats: &ir.Stats{Attack: 1, Life: 8}},
	}}
}

func TestNestedRepeatRestoresRemainingCountAndScopes(t *testing.T) {
	pack := repeatedChoicePack()
	state := testState()
	actor := state.Players["own"]
	actor.PP, actor.MaxPP = 10, 10
	sourceID, ownID, enemyID := strings.Repeat("a", 32), strings.Repeat("b", 32), strings.Repeat("c", 32)
	actor = withInstance(actor, "hand", ir.TestInstance{InstanceID: sourceID, CardID: 77774001, DeclaredType: "spell"})
	for n := 0; n < 6; n++ {
		zone := "deck"
		if n < 2 {
			zone = "hand"
		}
		actor = withInstance(actor, zone, ir.TestInstance{InstanceID: fmt.Sprintf("%032x", n+20), CardID: 77774002, DeclaredType: "follower"})
	}
	state.Players["own"] = withInstance(actor, "field", ir.TestInstance{InstanceID: ownID, CardID: 77774002, DeclaredType: "follower"})
	state.Players["oppo"] = withInstance(state.Players["oppo"], "field", ir.TestInstance{InstanceID: enemyID, CardID: 77774002, DeclaredType: "follower"})
	s, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	result := s.SubmitAs(strings.Repeat("d", 32), "own", SimulatorCommand{Kind: "play", Source: sourceID})
	seen := map[string]bool{}
	for n := 0; n < 4; n++ {
		if result.Status != StatusSuspended || seen[result.Choice.RequestID] {
			t.Fatalf("iteration %d: %#v", n, result)
		}
		seen[result.Choice.RequestID] = true
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
		if n == 0 {
			for _, mutate := range []func(*Continuation){
				func(c *Continuation) { c.Stack[2].RepeatRemaining = 0 },
				func(c *Continuation) { c.Stack[2].RepeatRemaining = 3 },
				func(c *Continuation) { c.Stack[2].RepeatBindingsID = c.Stack[2].BindingFrameID },
				func(c *Continuation) { c.Stack[0].RepeatRemaining = 1 },
			} {
				bad, _ := DecodeContinuation(data)
				mutate(bad)
				if _, err := RestoreSession(pack, bad); err == nil {
					t.Fatal("accepted corrupted repeat checkpoint")
				}
			}
		}
		choice := result.Choice
		answer := ChoiceResponse{RequestID: choice.RequestID, ActionID: choice.ActionID, StateRevision: choice.StateRevision, SelectedInstanceIDs: []string{enemyID}}
		result = s.Resume(answer)
		other := restored.Resume(answer)
		if !reflect.DeepEqual(result, other) || !reflect.DeepEqual(s.g.snapshot(), restored.g.snapshot()) || s.BudgetState() != restored.BudgetState() {
			t.Fatal("restored repeat diverged")
		}
	}
	if result.Status != StatusCompleted || len(s.g.own.hand) != 6 || s.g.instances[enemyID].life != 4 || s.g.instances[ownID].life != 5 || s.g.rng.Consumed() != 1 {
		t.Fatal("repeat re-read count or leaked inner binding")
	}
}

func TestEmptyRepeatRemainsBounded(t *testing.T) {
	pack := repeatedChoicePack()
	pack.Cards[0].PlayEffects = []ir.Effect{ir.RepeatEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("1", 32)}, Kind: "repeat", Times: 65535, Body: []ir.Effect{}}}
	state := testState()
	actor := state.Players["own"]
	actor.PP, actor.MaxPP = 1, 1
	sourceID := strings.Repeat("a", 32)
	state.Players["own"] = withInstance(actor, "hand", ir.TestInstance{InstanceID: sourceID, CardID: 77774001, DeclaredType: "spell"})
	s, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	s.budgetPolicy.Instructions = 20
	step := s.SubmitAs(strings.Repeat("b", 32), "own", SimulatorCommand{Kind: "play", Source: sourceID})
	if step.Status != StatusFault || step.ErrorCode != executionBudgetExceeded || s.BudgetState().Instructions != 20 || s.BudgetState().MaxStackDepth != 2 || len(s.stack) != 2 {
		t.Fatalf("unbounded repeat: %#v %#v", step, s.BudgetState())
	}
}
