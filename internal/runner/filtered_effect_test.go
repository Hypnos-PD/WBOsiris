package runner

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"wbo/internal/ir"
	"wbo/internal/project"
)

func TestCompiledNoahOnlyBuffsPuppetryFollowersInHand(t *testing.T) {
	root := filepath.Join("..", "..")
	loaded := project.LoadWithRoot([]string{
		filepath.Join(root, "cards", "10001", "10172130.wbo"),
		filepath.Join(root, "cards", "90000", "90071110.wbo"),
		filepath.Join(root, "cards", "90000", "90001110.wbo"),
		filepath.Join(root, "cards", "10000", "10031310.wbo"),
	}, true, root)
	if loaded.HasErrors() {
		t.Fatal(loaded.Diagnostics)
	}
	pack, _, err := project.BuildRuntimePacks(loaded)
	if err != nil {
		t.Fatal(err)
	}
	for _, side := range []string{"own", "oppo"} {
		t.Run(side, func(t *testing.T) {
			state := testState()
			state.Turn.Active = side
			sourceID, puppetID, otherID, spellID, fieldID := strings.Repeat("1", 32), strings.Repeat("2", 32), strings.Repeat("3", 32), strings.Repeat("4", 32), strings.Repeat("5", 32)
			actor := state.Players[side]
			actor.PP, actor.MaxPP = 6, 6
			actor = withInstance(actor, "hand", ir.TestInstance{InstanceID: sourceID, CardID: 10172130, DeclaredType: "follower"})
			actor = withInstance(actor, "hand", ir.TestInstance{InstanceID: puppetID, CardID: 90071110, DeclaredType: "follower"})
			actor = withInstance(actor, "hand", ir.TestInstance{InstanceID: otherID, CardID: 90001110, DeclaredType: "follower"})
			actor = withInstance(actor, "hand", ir.TestInstance{InstanceID: spellID, CardID: 10031310, DeclaredType: "spell"})
			state.Players[side] = withInstance(actor, "field", ir.TestInstance{InstanceID: fieldID, CardID: 90071110, DeclaredType: "follower"})
			otherSide := oppositeSide(side)
			state.Players[otherSide] = withInstance(state.Players[otherSide], "hand", ir.TestInstance{InstanceID: strings.Repeat("6", 32), CardID: 90071110, DeclaredType: "follower"})
			session, err := NewSession(pack, state, 1)
			if err != nil {
				t.Fatal(err)
			}
			if result := session.SubmitAs(strings.Repeat("a", 32), side, SimulatorCommand{Kind: "play", Source: sourceID}); result.Status != StatusCompleted {
				t.Fatal(result)
			}
			view, _ := session.View(side)
			if len(view.Own.Hand) != 6 {
				t.Fatalf("wrong number of cards after adding three puppets: %d", len(view.Own.Hand))
			}
			for _, card := range view.Own.Hand {
				want := 0
				if card.CardID == 90071110 {
					want = 2
				} else if card.CardID == 90001110 {
					want = 1
				}
				if card.Attack != want {
					t.Errorf("card %d attack=%d, want %d", card.CardID, card.Attack, want)
				}
			}
			if session.g.instances[fieldID].attack != 1 || session.g.instances[strings.Repeat("6", 32)].attack != 1 || session.g.instances[sourceID].attack != 5 {
				t.Fatal("Noah buff escaped the controller's hand")
			}
		})
	}
}

func TestTargetEffectFiltersApplyBeforeMutation(t *testing.T) {
	for _, kind := range []string{"buff_stats", "damage", "destroy", "banish"} {
		t.Run(kind, func(t *testing.T) {
			pack := &ir.CardPack{Cards: []ir.Card{
				{ID: 77772001, CardType: "follower", Stats: &ir.Stats{Attack: 1, Life: 4}, Traits: []string{"puppetry"}},
				{ID: 77772002, CardType: "follower", Stats: &ir.Stats{Attack: 1, Life: 3}, Traits: []string{"puppetry"}},
				{ID: 77772003, CardType: "follower", Stats: &ir.Stats{Attack: 1, Life: 4}, Traits: []string{"officer"}},
			}}
			state := testState()
			ids := []string{strings.Repeat("1", 32), strings.Repeat("2", 32), strings.Repeat("3", 32)}
			for n, id := range ids {
				state.Players["own"] = withInstance(state.Players["own"], "field", ir.TestInstance{InstanceID: id, CardID: 77772001 + n, DeclaredType: "follower"})
			}
			session, err := NewSession(pack, state, 1)
			if err != nil {
				t.Fatal(err)
			}
			effect := ir.TargetEffect{
				Kind: kind, Target: ir.ZoneRef{Kind: "zone", Side: "own", Zone: "field", Member: "follower"},
				Predicate: ir.AndPredicate{Kind: "and", Terms: []ir.Predicate{
					ir.FieldPredicate{Kind: "has_trait", Trait: "puppetry"},
					ir.FieldPredicate{Kind: "compare", Field: "life", Op: "ne", Value: 3},
				}},
			}
			effect.Amount, effect.AttackDelta, effect.LifeDelta = 2, 1, -1
			session.g.execTargetEffect(effect, nil, frame{})
			for _, id := range ids[1:] {
				i := session.g.instances[id]
				if i.zone != "field" || i.attack != 1 || i.life != i.card.Stats.Life {
					t.Fatalf("effect changed nonmatching follower: %#v", i)
				}
			}
			selected := session.g.instances[ids[0]]
			switch kind {
			case "buff_stats":
				if selected.attack != 2 || selected.life != 3 {
					t.Fatal("matching follower missed buff")
				}
			case "damage":
				if selected.life != 2 {
					t.Fatal("matching follower missed damage")
				}
			case "destroy":
				if selected.zone != "graveyard" {
					t.Fatal("matching follower survived destruction")
				}
			case "banish":
				if selected.zone != "banished" {
					t.Fatal("matching follower was not banished")
				}
			}
			// A false predicate must produce no events or state changes.
			effect.Predicate = ir.FieldPredicate{Kind: "has_trait", Trait: "pixie"}
			before := session.g.snapshot()
			session.g.execTargetEffect(effect, nil, frame{})
			if !reflect.DeepEqual(before, session.g.snapshot()) {
				t.Fatal("empty filtered target set changed state")
			}
		})
	}
}

func TestTargetEffectFilterBudgetDoesNotMutatePartialResults(t *testing.T) {
	pack := &ir.CardPack{Cards: []ir.Card{{ID: 77772001, CardType: "follower", Stats: &ir.Stats{Attack: 1, Life: 2}, Traits: []string{"puppetry"}}}}
	state := testState()
	for _, id := range []string{strings.Repeat("1", 32), strings.Repeat("2", 32)} {
		state.Players["own"] = withInstance(state.Players["own"], "field", ir.TestInstance{InstanceID: id, CardID: 77772001, DeclaredType: "follower"})
	}
	session, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	policy := session.budgetPolicy
	// Two visits resolve the zone; the filter exhausts its budget after one match.
	policy.QueryVisits = 3
	session.g.budget = &budgetTracker{policy: policy}
	before := session.g.snapshot()
	session.g.execTargetEffect(ir.TargetEffect{
		Kind: "buff_stats", Target: ir.ZoneRef{Kind: "zone", Side: "own", Zone: "field", Member: "follower"},
		AttackDelta: 2, LifeDelta: 2, Predicate: ir.FieldPredicate{Kind: "has_trait", Trait: "puppetry"},
	}, nil, frame{})
	if !session.g.budget.exceeded || !reflect.DeepEqual(before, session.g.snapshot()) {
		t.Fatal("query budget exhaustion applied a partial buff")
	}
}
