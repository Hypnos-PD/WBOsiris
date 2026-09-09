package runner

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"wbo/internal/ir"
	"wbo/internal/ruleset"
)

var apocalypseRecipe = []ir.DeckEntry{{CardID: 90004110, Count: 3}, {CardID: 90004120, Count: 3}, {CardID: 90004130, Count: 3}, {CardID: 90004310, Count: 1}}

func TestRulerReplacesOnlyItsDeckWithFreshShuffledCards(t *testing.T) {
	pack := repeatCardPack(t)
	for _, owner := range []string{"own", "oppo"} {
		for _, seed := range []uint64{1, 7, 999} {
			state := crestState(owner)
			source := strings.Repeat("a", 32)
			state.Players[owner] = withInstance(state.Players[owner], "hand", ir.TestInstance{InstanceID: source, CardID: 10104120, DeclaredType: "follower"})
			s, err := NewSession(pack, state, seed)
			if err != nil {
				t.Fatal(err)
			}
			p := s.g.player(owner)
			old := append([]*instance(nil), p.deck...)
			old[0].cost = 0
			old[0].buffStats(5, 7, owner)
			old[0].addKeyword("storm", owner)
			before := s.g.snapshot()
			foe := snapshotPlayer(*s.g.player(oppositeSide(owner)))
			s.LegalActionsFor(owner)
			if !reflect.DeepEqual(before, s.g.snapshot()) {
				t.Fatal("legal action preview mutated deck or RNG")
			}
			if r := s.SubmitAs(strings.Repeat("b", 32), owner, SimulatorCommand{Kind: "play", Source: source}); r.Status != StatusCompleted {
				t.Fatal(r)
			}
			if len(p.deck) != 10 || len(p.retiredDeck) != 12 || len(p.graveyard) != 0 || len(p.banished) != 0 || len(p.destroyed) != 0 || p.shadows != 0 || p.combo != 1 || p.pp != 0 {
				t.Fatal("replacement changed unrelated zones or resources")
			}
			if !reflect.DeepEqual(foe, snapshotPlayer(*s.g.player(oppositeSide(owner)))) {
				t.Fatal("replacement changed opponent")
			}
			want := []int{90004110, 90004110, 90004110, 90004120, 90004120, 90004120, 90004130, 90004130, 90004130, 90004310}
			rng := ruleset.NewRNG(seed)
			for n := len(want) - 1; n > 0; n-- {
				j := rng.Index(n + 1)
				want[n], want[j] = want[j], want[n]
			}
			for n, i := range p.deck {
				if i.card.ID != want[n] || i.cost != i.card.Cost || len(i.temporaryStats) != 0 || len(i.temporaryKeywords) != 0 || i.zone != "deck" {
					t.Fatal("wrong composition, order or fresh state")
				}
				if i.card.Stats != nil && (i.attack != i.card.Stats.Attack || i.life != i.card.Stats.Life) {
					t.Fatal("replacement retained old stats")
				}
			}
			if s.g.rng.Consumed() != 9 {
				t.Fatal("shuffle must consume nine decisions")
			}
			for _, i := range old {
				if i.zone != "retired_deck" || s.g.owner(i) != p {
					t.Fatal("retired identity lost its owner")
				}
			}
			if old[0].attack != 7 || old[0].cost != 0 || !old[0].abilities["storm"] {
				t.Fatal("retirement rewrote historical identity")
			}
			for _, viewer := range []string{"own", "oppo"} {
				view, err := s.View(viewer)
				if err != nil {
					t.Fatal(err)
				}
				payload, _ := json.Marshal(view)
				for _, i := range append(old, p.deck...) {
					if strings.Contains(string(payload), i.id) {
						t.Fatal("hidden old or new deck identity leaked")
					}
				}
				found := 0
				for _, event := range s.EventsFor(viewer) {
					if event.Kind == "deck_replaced" {
						found++
						if event.Count != 10 || event.Side != owner || event.CardID != 0 || event.InstanceID != "" {
							t.Fatal(event)
						}
					}
					if event.Kind == "card_drawn" || event.Kind == "destroyed" || event.Kind == "zone_moved" {
						t.Fatal("replacement emitted card movement", event)
					}
				}
				if found != 1 {
					t.Fatal("missing public replacement count")
				}
			}
			restored, err := restoreGame(s.g.cards, snapshotContinuationGame(s.g))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(s.g.snapshot(), restored.snapshot()) {
				t.Fatal("replacement continuation diverged")
			}
			for _, current := range []*game{s.g, restored} {
				current.draw(ir.DrawEffect{Kind: "draw", Owner: "own", SourceZone: "deck", Count: 1, Output: "drawn"}, current.instances[source], frame{})
			}
			if s.g.player(owner).hand[0].card.ID != want[0] || !reflect.DeepEqual(s.g.snapshot(), restored.snapshot()) {
				t.Fatal("restored deck drew a different card")
			}
		}
	}
}

func TestDeckReplacementRetainsInertBindingsAcrossPause(t *testing.T) {
	pack := repeatCardPack(t)
	for n := range pack.Cards {
		if pack.Cards[n].ID == 10104120 {
			body := pack.Cards[n].Abilities[0].Body
			body = append([]ir.Effect{ir.SelectionEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("c", 32)}, Kind: "random_choose", Policy: "random", Binding: "old", Source: ir.ZoneRef{Kind: "zone", Side: "own", Zone: "deck", Member: "follower"}}}, body...)
			body = append(body,
				ir.ModeEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("d", 32)}, Kind: "mode", Options: []ir.ModeOption{{ID: 1, Body: []ir.Effect{}}}},
				ir.TargetEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("e", 32)}, Kind: "return", Target: ir.BindingRef{Kind: "binding", Name: "old"}, Destination: "hand"},
				ir.DrawEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("f", 32)}, Kind: "draw", Owner: "own", SourceZone: "deck", Count: 1, Output: "drawn"})
			pack.Cards[n].Abilities[0].Body = body
		}
	}
	for _, owner := range []string{"own", "oppo"} {
		state := crestState(owner)
		source := strings.Repeat("a", 32)
		state.Players[owner] = withInstance(state.Players[owner], "hand", ir.TestInstance{InstanceID: source, CardID: 10104120, DeclaredType: "follower"})
		s, err := NewSession(pack, state, 77)
		if err != nil {
			t.Fatal(err)
		}
		r := s.SubmitAs(strings.Repeat("b", 32), owner, SimulatorCommand{Kind: "play", Source: source})
		if r.Status != StatusSuspended || len(s.g.player(owner).deck) != 10 {
			t.Fatal(r)
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
		for _, current := range []*Session{s, restored} {
			step := current.Resume(ChoiceResponse{RequestID: r.Choice.RequestID, ActionID: r.Choice.ActionID, StateRevision: r.Choice.StateRevision, SelectedOptionID: 1})
			if step.Status != StatusCompleted || len(current.g.player(owner).hand) != 1 || len(current.g.player(owner).retiredDeck) != 12 {
				t.Fatal("retired binding returned to hand", step)
			}
		}
		if !reflect.DeepEqual(s.g.snapshot(), restored.g.snapshot()) {
			t.Fatal("resumed replacement diverged")
		}
		// A retired entity cannot also occupy a live zone, disappear, or change zones in a save.
		for _, change := range []func(*Continuation){
			func(c *Continuation) {
				if owner == "own" {
					c.Game.Own.Deck = append(c.Game.Own.Deck, c.Game.Own.RetiredDeck[0])
				} else {
					c.Game.Oppo.Deck = append(c.Game.Oppo.Deck, c.Game.Oppo.RetiredDeck[0])
				}
			},
			func(c *Continuation) {
				if owner == "own" {
					c.Game.Own.RetiredDeck = nil
				} else {
					c.Game.Oppo.RetiredDeck = nil
				}
			},
			func(c *Continuation) {
				for n := range c.Game.Instances {
					if c.Game.Instances[n].Zone == "retired_deck" {
						c.Game.Instances[n].Zone = "deck"
						break
					}
				}
			},
		} {
			bad, err := DecodeContinuation(data)
			if err != nil {
				t.Fatal(err)
			}
			change(bad)
			if _, err := RestoreSession(pack, bad); err == nil {
				t.Fatal("accepted invalid retirement")
			}
		}
	}
}

func TestDeckReplacementBudgetEmptyDeckAndSingleton(t *testing.T) {
	pack := repeatCardPack(t)
	for _, budget := range []string{"instances", "queries"} {
		state := crestState("own")
		s, err := NewSession(pack, state, 3)
		if err != nil {
			t.Fatal(err)
		}
		before := s.g.snapshot()
		policy := ruleset.Default().ExecutionBudget
		if budget == "instances" {
			policy.CreatedInstances = 9
		} else {
			policy.QueryVisits = 1
		}
		s.g.budget = &budgetTracker{}
		s.g.budget.reset(policy)
		s.g.replaceDeck(ir.DeckReplaceEffect{Kind: "replace_deck", Owner: "own", Cards: apocalypseRecipe}, nil)
		if !s.g.budget.exceeded || !reflect.DeepEqual(before, s.g.snapshot()) {
			t.Fatal("budget failure partly replaced deck", budget)
		}
	}
	s, err := NewSession(pack, testState(), 5)
	if err != nil {
		t.Fatal(err)
	}
	e := ir.DeckReplaceEffect{Kind: "replace_deck", Owner: "oppo", Cards: []ir.DeckEntry{{CardID: 90004120, Count: 1}}}
	s.g.replaceDeck(e, nil)
	first := s.g.oppo.deck[0]
	if s.g.gameOver || s.g.rng.Consumed() != 0 || len(s.g.oppo.retiredDeck) != 0 {
		t.Fatal("empty deck replacement failed")
	}
	s.g.replaceDeck(e, nil)
	if s.g.oppo.deck[0] == first || len(s.g.oppo.retiredDeck) != 1 || s.g.rng.Consumed() != 0 {
		t.Fatal("repeated replacement did not create a fresh singleton")
	}
	s.g.returnCard(first, "hand")
	if len(s.g.oppo.hand) != 0 || first.zone != "retired_deck" {
		t.Fatal("retired card re-entered play")
	}
}

func TestAstarothSetsMaximumWithoutDamageOrHealing(t *testing.T) {
	pack := repeatCardPack(t)
	for _, owner := range []string{"own", "oppo"} {
		state := crestState(owner)
		source := strings.Repeat("a", 32)
		state.Players[owner] = withInstance(state.Players[owner], "hand", ir.TestInstance{InstanceID: source, CardID: 90004310, DeclaredType: "spell"})
		foe := oppositeSide(owner)
		p := state.Players[foe]
		p.Leader = ir.Leader{Life: 13, MaxLife: 30}
		state.Players[foe] = p
		s, err := NewSession(pack, state, 1)
		if err != nil {
			t.Fatal(err)
		}
		if r := s.SubmitAs(strings.Repeat("b", 32), owner, SimulatorCommand{Kind: "play", Source: source}); r.Status != StatusCompleted {
			t.Fatal(r)
		}
		if s.g.player(foe).leaderLife != 1 || s.g.player(foe).leaderMax != 1 || s.g.gameOver {
			t.Fatal("maximum was not clamped")
		}
		for _, event := range s.g.events {
			if event.Kind == "damaged" || event.Kind == "healed" {
				t.Fatal("setting maximum produced damage/healing")
			}
		}
		restored, err := restoreGame(s.g.cards, snapshotContinuationGame(s.g))
		if err != nil {
			t.Fatal(err)
		}
		for _, g := range []*game{s.g, restored} {
			self := g.instances[source]
			g.execTargetEffect(ir.TargetEffect{Kind: "heal", Target: ir.LeaderRef{Kind: "leader", Side: "oppo"}, Amount: 20}, self, frame{})
			if g.player(foe).leaderLife != 1 {
				t.Fatal("healing exceeded new maximum")
			}
			g.setLeaderMaxLife(ir.LeaderMaxLifeEffect{Kind: "set_leader_max_life", Side: "oppo", Amount: 40}, self)
			if g.player(foe).leaderLife != 1 || g.player(foe).leaderMax != 40 {
				t.Fatal("raising maximum healed leader")
			}
			g.damageLeader(g.player(foe), foe, 1)
			if !g.gameOver || g.winner != owner {
				t.Fatal("one damage after maximum change did not win")
			}
		}
		if !reflect.DeepEqual(s.g.snapshot(), restored.snapshot()) {
			t.Fatal("maximum life restore diverged")
		}
	}
}

func TestReplacementPublicEventValidation(t *testing.T) {
	for _, kind := range []string{"deck_replaced", "leader_max_life_set"} {
		e := ir.RuntimeEvent{Kind: kind, Side: "own", Count: 10, Sequence: 1}
		if err := validateContinuationEvents([]ir.RuntimeEvent{e}, 1, 0, nil, nil); err != nil {
			t.Fatal(err)
		}
		for n, change := range []func(*ir.RuntimeEvent){func(e *ir.RuntimeEvent) { e.Count = 0 }, func(e *ir.RuntimeEvent) { e.CardID = 90004120 }, func(e *ir.RuntimeEvent) { e.PrivateTo = "own" }, func(e *ir.RuntimeEvent) { e.Side = "all" }} {
			bad := e
			change(&bad)
			if validateContinuationEvents([]ir.RuntimeEvent{bad}, 1, 0, nil, nil) == nil {
				t.Fatal(fmt.Sprintf("accepted %s mutation %d", kind, n))
			}
		}
	}
}

func TestFiendChoiceRestoresAndDefersLastwordsUntilLeaderDamage(t *testing.T) {
	pack := repeatCardPack(t)
	for _, owner := range []string{"own", "oppo"} {
		state := crestState(owner)
		source, a, b, protected := strings.Repeat("a", 32), strings.Repeat("b", 32), strings.Repeat("c", 32), strings.Repeat("d", 32)
		state.Players[owner] = withInstance(state.Players[owner], "hand", ir.TestInstance{InstanceID: source, CardID: 90004130, DeclaredType: "follower"})
		foe := oppositeSide(owner)
		p := withInstance(state.Players[foe], "field", ir.TestInstance{InstanceID: a, CardID: 10001120, DeclaredType: "follower"})
		p = withInstance(p, "field", ir.TestInstance{InstanceID: b, CardID: 10001120, DeclaredType: "follower"})
		state.Players[foe] = withInstance(p, "field", ir.TestInstance{InstanceID: protected, CardID: 10001110, DeclaredType: "follower", Overrides: ir.InstanceOverrides{Keywords: []string{"aura"}}})
		s, err := NewSession(pack, state, 1)
		if err != nil {
			t.Fatal(err)
		}
		r := s.SubmitAs(strings.Repeat("e", 32), owner, SimulatorCommand{Kind: "play", Source: source})
		if r.Status != StatusSuspended || len(r.Choice.Candidates) != 2 || r.Choice.MinSelections != 2 || r.Choice.MaxSelections != 2 {
			t.Fatal(r)
		}
		answer := ChoiceResponse{RequestID: r.Choice.RequestID, ActionID: r.Choice.ActionID, StateRevision: r.Choice.StateRevision, SelectedInstanceIDs: []string{a, a}}
		before := s.g.snapshot()
		if bad := s.Resume(answer); bad.Status != StatusRejected || !reflect.DeepEqual(before, s.g.snapshot()) {
			t.Fatal("duplicate targets were accepted", bad)
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
		for n, current := range []*Session{s, restored} {
			answer.SelectedInstanceIDs = []string{a, b}
			if n == 1 {
				answer.SelectedInstanceIDs = []string{b, a}
			}
			if step := current.Resume(answer); step.Status != StatusCompleted {
				t.Fatal(step)
			}
			p := current.g.player(foe)
			if p.leaderLife != 14 || len(p.field) != 1 || p.field[0].id != protected || len(p.hand) != 2 {
				t.Fatal("Fiend damage or lastwords mismatch")
			}
			leader, draw := -1, -1
			for n, event := range current.g.events {
				if event.Kind == "damaged" && event.Target.Kind == "leader" {
					leader = n
				}
				if event.Kind == "card_drawn" && draw < 0 {
					draw = n
				}
			}
			if leader < 0 || draw <= leader {
				t.Fatal("lastwords ran before leader damage")
			}
		}
		if !reflect.DeepEqual(s.g.snapshot(), restored.g.snapshot()) {
			t.Fatal("Fiend selection order or resume diverged")
		}
	}
}

func TestSilentRiderCanAttackOnItsEntryTurn(t *testing.T) {
	pack := repeatCardPack(t)
	for _, owner := range []string{"own", "oppo"} {
		state := crestState(owner)
		source := strings.Repeat("a", 32)
		state.Players[owner] = withInstance(state.Players[owner], "hand", ir.TestInstance{InstanceID: source, CardID: 90004110, DeclaredType: "follower"})
		s, err := NewSession(pack, state, 1)
		if err != nil {
			t.Fatal(err)
		}
		if r := s.SubmitAs(strings.Repeat("b", 32), owner, SimulatorCommand{Kind: "play", Source: source}); r.Status != StatusCompleted {
			t.Fatal(r)
		}
		if r := s.SubmitAs(strings.Repeat("c", 32), owner, SimulatorCommand{Kind: "attack", Source: source}); r.Status != StatusCompleted {
			t.Fatal(r)
		}
		if s.g.player(oppositeSide(owner)).leaderLife != 10 {
			t.Fatal("Storm attack failed")
		}
	}
}

func TestDeckReplacementKeepsExistingDestructionRecords(t *testing.T) {
	pack := repeatCardPack(t)
	state := crestState("own")
	old := strings.Repeat("a", 32)
	state.Players["own"] = withInstance(state.Players["own"], "field", ir.TestInstance{InstanceID: old, CardID: 10001110, DeclaredType: "follower"})
	s, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	g := s.g
	original := g.instances[old]
	g.destroyByEffect([]*instance{original})
	g.returnCard(original, "deck")
	history := cloneDestructionHistory(g.own.destroyed)
	g.replaceDeck(ir.DeckReplaceEffect{Kind: "replace_deck", Owner: "own", Cards: apocalypseRecipe}, nil)
	if !reflect.DeepEqual(history, g.own.destroyed) || original.zone != "retired_deck" || g.own.shadows != 1 {
		t.Fatal("replacement rewrote existing destruction history")
	}
	if _, err := restoreGame(g.cards, snapshotContinuationGame(g)); err != nil {
		t.Fatal("retired history reference did not restore", err)
	}
}
