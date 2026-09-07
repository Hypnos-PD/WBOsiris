package runner

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"wbo/internal/ir"
	"wbo/internal/ruleset"
)

func playerCostPredicate(side, field string) ir.FieldPredicate {
	return ir.FieldPredicate{Kind: "compare", Field: "cost", Op: "eq", ValueScalar: &ir.Scalar{Kind: "scalar", Side: side, Field: field}}
}

func TestGrasshopperDrawUsesComboIncludingItsOwnPlay(t *testing.T) {
	pack := repeatCardPack(t)
	for _, side := range []string{"own", "oppo"} {
		for _, combo := range []int{0, 2, 9} {
			t.Run(fmt.Sprintf("%s/%d", side, combo), func(t *testing.T) {
				state := testState()
				state.Turn.Active = side
				p := state.Players[side]
				p.Combo, p.PP, p.MaxPP = combo, 3, 3
				source := strings.Repeat("a", 32)
				p = withInstance(p, "hand", ir.TestInstance{InstanceID: source, CardID: 10111140, DeclaredType: "follower"})
				for n, cost := range []int{combo, combo + 1, combo + 1, combo + 2} {
					p = withInstance(p, "deck", ir.TestInstance{InstanceID: fmt.Sprintf("%032x", n+1), CardID: 10001110, DeclaredType: "follower", Overrides: ir.InstanceOverrides{Cost: &cost}})
				}
				cost := combo + 1
				p = withInstance(p, "deck", ir.TestInstance{InstanceID: strings.Repeat("f", 32), CardID: 10131320, DeclaredType: "spell", Overrides: ir.InstanceOverrides{Cost: &cost}})
				state.Players[side] = p
				s, err := NewSession(pack, state, 42)
				if err != nil {
					t.Fatal(err)
				}
				before := s.g.snapshot()
				s.LegalActionsFor(side)
				if !reflect.DeepEqual(before, s.g.snapshot()) || s.g.rng.Consumed() != 0 {
					t.Fatal("legal action query changed draw state")
				}
				step := s.SubmitAs(strings.Repeat("b", 32), side, SimulatorCommand{Kind: "play", Source: source})
				if step.Status != StatusCompleted {
					t.Fatal(step)
				}
				player := s.g.player(side)
				wantID := fmt.Sprintf("%032x", 2+ruleset.NewRNG(42).Index(2))
				if player.combo != combo+1 || len(player.hand) != 1 || player.hand[0].id != wantID || player.hand[0].cost != combo+1 || s.g.rng.Consumed() != 1 {
					t.Fatal("wrong combo draw", player.hand, player.combo)
				}
				view, _ := s.View(oppositeSide(side))
				if len(view.Oppo.Hand) != 0 {
					t.Fatal("opponent can see the filtered draw")
				}
			})
		}
	}
}

func TestGrasshopperNoMatchDoesNotCauseDeckOut(t *testing.T) {
	for _, side := range []string{"own", "oppo"} {
		for _, empty := range []bool{true, false} {
			state := testState()
			state.Turn.Active = side
			p := state.Players[side]
			p.PP, p.MaxPP = 3, 3
			id := strings.Repeat("a", 32)
			p = withInstance(p, "hand", ir.TestInstance{InstanceID: id, CardID: 10111140, DeclaredType: "follower"})
			if !empty {
				p = withInstance(p, "deck", ir.TestInstance{InstanceID: strings.Repeat("b", 32), CardID: 10001110, DeclaredType: "follower"})
			}
			state.Players[side] = p
			s, err := NewSession(repeatCardPack(t), state, 7)
			if err != nil {
				t.Fatal(err)
			}
			step := s.SubmitAs(strings.Repeat("c", 32), side, SimulatorCommand{Kind: "play", Source: id})
			if step.Status != StatusCompleted || s.g.gameOver || s.g.rng.Consumed() != 0 || len(s.g.player(side).hand) != 0 || len(s.g.player(side).deck) != len(p.Zones["deck"]) {
				t.Fatal("unsuccessful search caused a draw or defeat", side, empty, step)
			}
		}
	}
}

func TestScalarPredicateContextAcrossQueriesEffectsFusionAndTriggers(t *testing.T) {
	for _, side := range []string{"own", "oppo"} {
		state := testState()
		p := state.Players[side]
		p.Combo, p.Shadows = 2, 1
		id := strings.Repeat("a", 32)
		p = withInstance(p, "field", ir.TestInstance{InstanceID: id, CardID: 10111140, DeclaredType: "follower"})
		for n, cost := range []int{1, 2} {
			p = withInstance(p, "hand", ir.TestInstance{InstanceID: fmt.Sprintf("%032x", n+1), CardID: 10001110, DeclaredType: "follower", Overrides: ir.InstanceOverrides{Cost: &cost}})
		}
		state.Players[side] = p
		other := state.Players[oppositeSide(side)]
		other.Combo = 5
		cost := 2
		other = withInstance(other, "deck", ir.TestInstance{InstanceID: strings.Repeat("b", 32), CardID: 10001110, DeclaredType: "follower", Overrides: ir.InstanceOverrides{Cost: &cost}})
		state.Players[oppositeSide(side)] = other
		s, err := NewSession(repeatCardPack(t), state, 5)
		if err != nil {
			t.Fatal(err)
		}
		source := s.g.instances[id]
		predicate := playerCostPredicate("own", "combo")
		ref := ir.FilterRef{Kind: "filter", Source: ir.ZoneRef{Kind: "zone", Side: "oppo", Zone: "deck"}, Predicate: predicate}
		if found := s.g.fromRef(ref, source, nil); len(found) != 1 || found[0].cost != 2 {
			t.Fatal("query read candidate owner's combo", found)
		}
		if got := s.g.numericValue(&ir.CountExpr{Kind: "count", Source: ref}, source, nil); got != 1 {
			t.Fatal("aggregate lost predicate context", got)
		}
		ref.Predicate = playerCostPredicate("oppo", "combo")
		if found := s.g.fromRef(ref, source, nil); len(found) != 0 {
			t.Fatal("opposing scalar used the ability controller's value", found)
		}
		ability := ir.FusionAbility{MaterialFilter: ir.MaterialFilter{Source: ir.ZoneRef{Kind: "zone", Side: "own", Zone: "hand"}, Minimum: 1, Predicate: predicate}}
		if found := s.g.fusionCandidates(source, &ability); len(found) != 1 || found[0].cost != 2 {
			t.Fatal("fusion lost predicate context", found)
		}
		s.g.draw(ir.DrawEffect{Kind: "draw", Owner: "oppo", Count: 1, Output: "drawn", Predicate: predicate}, source, frame{})
		if len(s.g.player(oppositeSide(side)).hand) != 1 {
			t.Fatal("draw used its recipient as the ability controller")
		}
		// Select the entire target set before destruction changes the resource.
		predicate = playerCostPredicate("own", "shadows")
		predicate.Op = "le"
		for _, card := range append([]*instance(nil), s.g.player(side).hand...) {
			s.g.removeFromPlayer(s.g.player(side), card)
			card.zone = "field"
			s.g.player(side).field = append(s.g.player(side).field, card)
		}
		s.g.execTargetEffect(ir.TargetEffect{Kind: "destroy", Target: ir.ZoneRef{Kind: "zone", Side: "own", Zone: "field"}, Predicate: predicate}, source, nil)
		if s.g.player(side).shadows != 2 || len(s.g.player(side).field) != 2 {
			t.Fatal("destruction reevaluated the filter after resource mutation")
		}
		trigger := ir.EventTrigger{Kind: "event", Event: "follower_summoned", Side: "oppo", SubjectType: "follower", Predicate: playerCostPredicate("own", "combo")}
		source.card = &ir.Card{ID: source.card.ID, CardType: "follower", Abilities: []ir.Ability{{ID: strings.Repeat("e", 32), Trigger: trigger, Body: []ir.Effect{ir.DrawEffect{Kind: "draw", Count: 1}}}}}
		s.g.rebuildTriggerIndex()
		s.g.summonFor(source, "oppo", 1, 10001110, false)
		if len(s.g.triggers) != 1 {
			t.Fatal("opposing event used the subject's controller", side, s.g.triggers)
		}
	}
}

func TestScalarPredicateDrawAfterChoiceRestoresDeterministically(t *testing.T) {
	for _, side := range []string{"own", "oppo"} {
		pack := repeatCardPack(t)
		pack.Cards = append(pack.Cards, ir.Card{ID: 77773006, CardType: "spell", PlayEffects: []ir.Effect{
			ir.ModeEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("c", 32)}, Kind: "mode", Options: []ir.ModeOption{{ID: 1, Body: []ir.Effect{
				ir.AdjustEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("d", 32)}, Kind: "adjust_resource", Owner: "own", Resource: "combo", Delta: 2},
			}}}},
			ir.DrawEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("e", 32)}, Kind: "draw", Owner: "own", Count: 1, Output: "drawn", Predicate: playerCostPredicate("own", "combo")},
		}})
		state := testState()
		state.Turn.Active = side
		id := strings.Repeat("a", 32)
		p := withInstance(state.Players[side], "hand", ir.TestInstance{InstanceID: id, CardID: 77773006, DeclaredType: "spell"})
		for n, cost := range []int{1, 3, 3} {
			p = withInstance(p, "deck", ir.TestInstance{InstanceID: fmt.Sprintf("%032x", n+1), CardID: 10001110, DeclaredType: "follower", Overrides: ir.InstanceOverrides{Cost: &cost}})
		}
		state.Players[side] = p
		s, err := NewSession(pack, state, 33)
		if err != nil {
			t.Fatal(err)
		}
		step := s.SubmitAs(strings.Repeat("b", 32), side, SimulatorCommand{Kind: "play", Source: id})
		if step.Status != StatusSuspended || s.g.rng.Consumed() != 0 {
			t.Fatal(step)
		}
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
		response := ChoiceResponse{RequestID: step.Choice.RequestID, ActionID: step.Choice.ActionID, StateRevision: step.Choice.StateRevision, SelectedOptionID: 1}
		for _, current := range []*Session{s, restored} {
			if r := current.Resume(response); r.Status != StatusCompleted {
				t.Fatal(r)
			}
			hand := current.g.player(side).hand
			if len(hand) != 1 || hand[0].cost != 3 || current.g.rng.Consumed() != 1 {
				t.Fatal("draw used the scalar from before the choice", hand)
			}
		}
		if !reflect.DeepEqual(s.g.snapshot(), restored.g.snapshot()) {
			t.Fatal("dynamic filtered draw diverged after restore")
		}
	}
}
