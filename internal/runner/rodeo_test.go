package runner

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"wbo/internal/ir"
	"wbo/internal/ruleset"
)

func TestRodeoDiscardAndDeckSummonRestoreForBothPlayers(t *testing.T) {
	pack := repeatCardPack(t)
	for _, owner := range []string{"own", "oppo"} {
		state := crestState(owner)
		source, held := strings.Repeat("a", 32), strings.Repeat("b", 32)
		p := state.Players[owner]
		p.Zones["deck"] = nil
		p = withInstance(p, "hand", ir.TestInstance{InstanceID: source, CardID: 10164110, DeclaredType: "follower"})
		p = withInstance(p, "hand", ir.TestInstance{InstanceID: held, CardID: 10001110, DeclaredType: "follower"})
		ids := []int{10001210, 10062210, 10001210, 10002210, 10061210, 10031310, 10001110}
		for n, id := range ids {
			kind := "amulet"
			if id == 10031310 {
				kind = "spell"
			}
			if id == 10001110 {
				kind = "follower"
			}
			p = withInstance(p, "deck", ir.TestInstance{InstanceID: fmt.Sprintf("%032x", n+1), CardID: id, DeclaredType: kind})
		}
		state.Players[owner] = p
		s, err := NewSession(pack, state, 8)
		if err != nil {
			t.Fatal(err)
		}
		before := s.g.snapshot()
		s.LegalActionsFor(owner)
		if !reflect.DeepEqual(before, s.g.snapshot()) {
			t.Fatal("preflight consumed a deck or random choice")
		}
		step := s.SubmitAs(strings.Repeat("c", 32), owner, SimulatorCommand{Kind: "play", Source: source})
		if step.Status != StatusSuspended || len(s.g.player(owner).deck) != len(ids) {
			t.Fatal("deck changed before discard", step)
		}
		other, _ := s.View(oppositeSide(owner))
		if other.PendingChoice != nil || len(other.Oppo.Hand) != 0 {
			t.Fatal("discard choice leaked")
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
			if r := current.Resume(ChoiceResponse{RequestID: step.Choice.RequestID, ActionID: step.Choice.ActionID, StateRevision: step.Choice.StateRevision, SelectedInstanceIDs: []string{held}}); r.Status != StatusCompleted {
				t.Fatal(r)
			}
			p := current.g.player(owner)
			if len(p.field) != 4 || len(p.deck) != 4 || len(p.hand) != 0 || p.shadows != 1 || p.combo != 1 || p.pp != 3 {
				t.Fatal("wrong deck summon zones or paid costs")
			}
			seen := map[int]bool{}
			chosen := map[string]bool{}
			for _, i := range p.field[1:] {
				if seen[i.card.ID] || i.zone != "field" || i.card.CardType != "amulet" {
					t.Fatal("duplicated name or wrong type")
				}
				seen[i.card.ID] = true
				chosen[i.id] = true
				if current.g.instances[i.id] != i {
					t.Fatal("deck card identity replaced")
				}
			}
			if !seen[10001210] || !seen[10002210] || !seen[10062210] {
				t.Fatal("wrong filtered names", seen)
			}
			var kept []string
			for n := range ids {
				id := fmt.Sprintf("%032x", n+1)
				if !chosen[id] {
					kept = append(kept, id)
				}
			}
			if !reflect.DeepEqual(instanceIDs(p.deck), kept) {
				t.Fatal("unselected deck order changed")
			}
			discarded := -1
			entries := 0
			for n, event := range current.g.events {
				if event.Kind == "card_drawn" {
					t.Fatal("deck summon triggered draw or fanfare")
				}
				if event.Kind == "card_discarded" {
					discarded = n
				}
				if event.Kind == "amulet_summoned" {
					entries++
					if discarded < 0 || n < discarded || event.Side != owner || event.PrivateTo != "" {
						t.Fatal("wrong entry order/visibility")
					}
				}
			}
			if entries != 3 {
				t.Fatal("missing amulet entries")
			}
			for _, viewer := range []string{"own", "oppo"} {
				v, _ := current.View(viewer)
				own := v.Own
				if viewer != owner {
					own = v.Oppo
				}
				if len(own.Field) != 4 || len(current.EventsFor(viewer)) != len(current.g.events) {
					t.Fatal("public summoned cards hidden")
				}
			}
		}
		if !reflect.DeepEqual(s.g.snapshot(), restored.g.snapshot()) {
			t.Fatal("restored deck summon diverged")
		}
	}
}

func TestDeckSummonWeightsCopiesThenExcludesTheChosenName(t *testing.T) {
	pack := &ir.CardPack{Cards: []ir.Card{{ID: 77779001, CardType: "amulet", Cost: 1}, {ID: 77779002, CardType: "amulet", Cost: 2}, {ID: 77779003, CardType: "amulet", Cost: 3}}}
	for _, owner := range []string{"own", "oppo"} {
		for seed := uint64(0); seed < 40; seed++ {
			state := testState()
			state.Turn.Active = owner
			cards := []int{77779001, 77779001, 77779001, 77779002, 77779002, 77779003}
			p := state.Players[owner]
			for n, id := range cards {
				p = withInstance(p, "deck", ir.TestInstance{InstanceID: fmt.Sprintf("%032x", n+1), CardID: id, DeclaredType: "amulet"})
			}
			state.Players[owner] = p
			s, err := NewSession(pack, state, seed)
			if err != nil {
				t.Fatal(err)
			}
			// The source establishes the relative controller without adding a board slot.
			self := s.g.player(owner).deck[0]
			e := ir.DeckSummonEffect{Kind: "summon_from_deck", Source: ir.ZoneRef{Kind: "zone", Side: "own", Zone: "deck", Member: "amulet"}, Count: 2, DistinctNames: true, Output: "summoned"}
			out := s.g.summonFromDeck(e, self, nil)
			rng := ruleset.NewRNG(seed)
			first := cards[rng.Index(len(cards))]
			var remaining []int
			for _, id := range cards {
				if id != first {
					remaining = append(remaining, id)
				}
			}
			second := remaining[rng.Index(len(remaining))]
			if len(out) != 2 || out[0].card.ID != first || out[1].card.ID != second || first == second {
				t.Fatal("wrong copy-weighted sample", owner, seed)
			}
		}
	}
}

func TestDeckSummonPreservesModifiersCapacityAndBudget(t *testing.T) {
	pack := repeatCardPack(t)
	for _, owner := range []string{"own", "oppo"} {
		state := testState()
		state.Turn.Active = owner
		zero, raised := 0, 4
		p := state.Players[owner]
		p = withInstance(p, "deck", ir.TestInstance{InstanceID: strings.Repeat("a", 32), CardID: 10061210, DeclaredType: "amulet", Overrides: ir.InstanceOverrides{Cost: &zero}})
		p = withInstance(p, "deck", ir.TestInstance{InstanceID: strings.Repeat("b", 32), CardID: 10062210, DeclaredType: "amulet", Overrides: ir.InstanceOverrides{Cost: &raised}})
		state.Players[owner] = p
		s, err := NewSession(pack, state, 1)
		if err != nil {
			t.Fatal(err)
		}
		self := s.g.player(owner).deck[0]
		e := ir.DeckSummonEffect{Kind: "summon_from_deck", Source: ir.FilterRef{Kind: "filter", Source: ir.ZoneRef{Kind: "zone", Side: "own", Zone: "deck", Member: "amulet"}, Predicate: ir.FieldPredicate{Kind: "compare", Field: "cost", Op: "le", Value: 3}}, Count: 3, DistinctNames: true, Output: "summoned"}
		out := s.g.summonFromDeck(e, self, nil)
		if len(out) != 1 || out[0] != self || self.cost != 0 || self.countdown != 2 || len(s.g.player(owner).deck) != 1 || s.g.rng.Consumed() != 0 {
			t.Fatal("current cost or moved identity lost")
		}
		// Empty selection does not draw, end the match, or change the survivor.
		if len(s.g.summonFromDeck(e, self, nil)) != 0 || s.g.gameOver {
			t.Fatal("empty summon caused deck-out")
		}
	}
	state := testState()
	p := state.Players["own"]
	for n := 0; n < 4; n++ {
		p = withInstance(p, "field", ir.TestInstance{InstanceID: fmt.Sprintf("%032x", n+10), CardID: 10001110, DeclaredType: "follower"})
	}
	for n := 0; n < 3; n++ {
		p = withInstance(p, "deck", ir.TestInstance{InstanceID: fmt.Sprintf("%032x", n+1), CardID: 10001210, DeclaredType: "amulet"})
	}
	state.Players["own"] = p
	s, err := NewSession(pack, state, 3)
	if err != nil {
		t.Fatal(err)
	}
	e := ir.DeckSummonEffect{Kind: "summon_from_deck", Source: ir.ZoneRef{Kind: "zone", Side: "own", Zone: "deck", Member: "amulet"}, Count: 3, Output: "summoned"}
	if len(s.g.summonFromDeck(e, nil, nil)) != 1 || len(s.g.own.deck) != 2 || s.g.rng.Consumed() != 1 {
		t.Fatal("overfilled the field or selected impossible cards")
	}
	if len(s.g.summonFromDeck(e, nil, nil)) != 0 || s.g.rng.Consumed() != 1 {
		t.Fatal("full field consumed randomness")
	}
	for _, candidateBudget := range []bool{false, true} {
		s, err := NewSession(pack, state, 3)
		if err != nil {
			t.Fatal(err)
		}
		before := s.g.snapshot()
		policy := ruleset.Default().ExecutionBudget
		if candidateBudget {
			policy.Candidates = 1
		} else {
			policy.QueryVisits = 1
		}
		s.g.budget = &budgetTracker{}
		s.g.budget.reset(policy)
		if len(s.g.summonFromDeck(e, nil, nil)) != 0 || !s.g.budget.exceeded || !reflect.DeepEqual(before, s.g.snapshot()) {
			t.Fatal("budget failure partially summoned or shuffled deck")
		}
	}
}

func TestDeckSummonFollowersKeepCopiesAndOutputWithoutFanfare(t *testing.T) {
	pack := &ir.CardPack{Cards: []ir.Card{
		{ID: 77779001, CardType: "spell", PlayEffects: []ir.Effect{
			ir.DeckSummonEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("c", 32)}, Kind: "summon_from_deck", Source: ir.ZoneRef{Kind: "zone", Side: "own", Zone: "deck", Member: "follower"}, Count: 3, Output: "summoned"},
			ir.TargetEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("d", 32)}, Kind: "buff_stats", Target: ir.BindingRef{Kind: "binding", Name: "summoned"}, AttackDelta: 1, LifeDelta: 2},
		}},
		{ID: 77779002, CardType: "follower", Cost: 2, Stats: &ir.Stats{Attack: 2, Life: 2}, Abilities: []ir.Ability{{ID: strings.Repeat("e", 32), Trigger: ir.SimpleTrigger{Kind: "fanfare"}, Body: []ir.Effect{
			ir.TargetEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("f", 32)}, Kind: "destroy", Target: ir.SelfRef{Kind: "self"}},
		}}}},
	}}
	for _, owner := range []string{"own", "oppo"} {
		state := testState()
		state.Turn.Active = owner
		p := withInstance(state.Players[owner], "hand", ir.TestInstance{InstanceID: strings.Repeat("a", 32), CardID: 77779001, DeclaredType: "spell"})
		zero := 0
		for n := 1; n <= 3; n++ {
			p = withInstance(p, "deck", ir.TestInstance{InstanceID: fmt.Sprintf("%032x", n), CardID: 77779002, DeclaredType: "follower", Overrides: ir.InstanceOverrides{Cost: &zero, Stats: &ir.Stats{Attack: 4, Life: 5}, Keywords: []string{"barrier"}}})
		}
		state.Players[owner] = p
		s, err := NewSession(pack, state, 3)
		if err != nil {
			t.Fatal(err)
		}
		if r := s.SubmitAs(strings.Repeat("b", 32), owner, SimulatorCommand{Kind: "play", Source: strings.Repeat("a", 32)}); r.Status != StatusCompleted {
			t.Fatal(r)
		}
		if len(s.g.player(owner).deck) != 0 || len(s.g.player(owner).field) != 3 || s.g.rng.Consumed() != 2 || s.g.player(owner).combo != 1 {
			t.Fatal("non-distinct summon did not move all physical copies")
		}
		for n := 1; n <= 3; n++ {
			i := s.g.instances[fmt.Sprintf("%032x", n)]
			if i.zone != "field" || i.cost != 0 || i.attack != 5 || i.life != 7 || !i.abilities["barrier"] || !i.summoningSick {
				t.Fatal("summon lost modifiers, output binding, or entry waiting")
			}
			if code := s.g.preflightAttack(ir.AttackAction{Kind: "attack_leader", Actor: owner, Attacker: i.id, Defender: oppositeSide(owner)}); code == "" {
				t.Fatal("summoned follower could attack immediately")
			}
		}
	}
}

func TestAmuletEntryPathsKeepFollowerListenersSeparate(t *testing.T) {
	pack := repeatCardPack(t)
	for n := range pack.Cards {
		if pack.Cards[n].ID == 10001110 {
			for j, kind := range []string{"follower", "amulet"} {
				pack.Cards[n].Abilities = append(pack.Cards[n].Abilities, ir.Ability{ID: fmt.Sprintf("%032x", j+1), Trigger: ir.EventTrigger{Kind: "event", Event: kind + "_summoned", Side: "own", SubjectType: kind}, Body: []ir.Effect{
					ir.TargetEffect{NodeBase: ir.NodeBase{ID: fmt.Sprintf("%032x", j+3)}, Kind: "buff_stats", Target: ir.SelfRef{Kind: "self"}, AttackDelta: j + 1},
				}})
			}
		}
	}
	for _, method := range []string{"play", "summon", "copy", "history", "follower"} {
		t.Run(method, func(t *testing.T) {
			state := crestState("own")
			watcher, amulet := strings.Repeat("a", 32), strings.Repeat("b", 32)
			p := withInstance(state.Players["own"], "field", ir.TestInstance{InstanceID: watcher, CardID: 10001110, DeclaredType: "follower"})
			p = withInstance(p, "hand", ir.TestInstance{InstanceID: amulet, CardID: 10001210, DeclaredType: "amulet"})
			p = withInstance(p, "destroyed", ir.TestInstance{InstanceID: strings.Repeat("c", 32), CardID: 10001210, DeclaredType: "amulet"})
			state.Players["own"] = p
			s, err := NewSession(pack, state, 1)
			if err != nil {
				t.Fatal(err)
			}
			switch method {
			case "play":
				if r := s.SubmitAs(strings.Repeat("d", 32), "own", SimulatorCommand{Kind: "play", Source: amulet}); r.Status != StatusCompleted || s.g.instances[watcher].attack != 4 {
					t.Fatal("hand entry did not fire only amulet listener", r)
				}
			case "summon":
				s.g.summon(1, 10001210)
			case "copy":
				s.g.execCardEffect(ir.CardEffect{Kind: "summon_copies", Owner: "own", Target: ir.BindingRef{Kind: "binding", Name: "target"}, Output: "summoned"}, s.g.instances[watcher], frame{"target": bindEntities(s.g.instances[amulet])})
			case "history":
				s.g.summonFromHistory(ir.HistorySummonEffect{Kind: "summon_from_history", Owner: "own", Source: ir.ZoneRef{Kind: "zone", Side: "own", Zone: "destroyed", Member: "amulet"}, Count: 1, Output: "summoned"}, s.g.instances[watcher], nil)
			case "follower":
				s.g.summon(1, 10001120)
			}
			kind, delta := "amulet_summoned", 2
			if method == "follower" {
				kind, delta = "follower_summoned", 1
			}
			entries := 0
			for _, event := range s.g.events {
				if event.Kind == "amulet_summoned" || event.Kind == "follower_summoned" {
					entries++
					if event.Kind != kind {
						t.Fatal("wrong entry event type", event)
					}
				}
			}
			if entries != 1 {
				t.Fatal("entry event missing or duplicated")
			}
			if method != "play" && (len(s.g.triggers) != 1 || s.g.triggers[0].body[0].(ir.TargetEffect).AttackDelta != delta) {
				t.Fatal("entry queued the wrong listener")
			}
		})
	}
}

func TestRodeoSuperEvolutionDestroysBeforeDamageAndLastwords(t *testing.T) {
	pack := repeatCardPack(t)
	for _, owner := range []string{"own", "oppo"} {
		for _, kind := range []string{"evolve", "superevolve"} {
			state := crestState(owner)
			source, high, low := strings.Repeat("a", 32), strings.Repeat("b", 32), strings.Repeat("c", 32)
			state.Players[owner] = withInstance(state.Players[owner], "field", ir.TestInstance{InstanceID: source, CardID: 10164110, DeclaredType: "follower"})
			foe := oppositeSide(owner)
			p := withInstance(state.Players[foe], "field", ir.TestInstance{InstanceID: high, CardID: 10001120, DeclaredType: "follower", Overrides: ir.InstanceOverrides{Stats: &ir.Stats{Attack: 9, Life: 5}, Keywords: []string{"stealth", "aura", "barrier"}}})
			state.Players[foe] = withInstance(p, "field", ir.TestInstance{InstanceID: low, CardID: 10001110, DeclaredType: "follower", Overrides: ir.InstanceOverrides{Stats: &ir.Stats{Attack: 2, Life: 1}}})
			s, err := NewSession(pack, state, 1)
			if err != nil {
				t.Fatal(err)
			}
			if r := s.SubmitAs(strings.Repeat("d", 32), owner, SimulatorCommand{Kind: kind, Source: source}); r.Status != StatusCompleted {
				t.Fatal(r)
			}
			if kind == "evolve" {
				if len(s.g.player(foe).field) != 2 || len(s.g.player(foe).hand) != 0 {
					t.Fatal("normal evolution fired super ability")
				}
				continue
			}
			if len(s.g.player(foe).field) != 0 || len(s.g.player(foe).hand) != 1 || s.g.instances[source].attack != 8 {
				t.Fatal("super evolution missed its targets")
			}
			destroyed, damaged, drawn := -1, -1, -1
			for n, e := range s.g.events {
				if e.Kind == "destroyed" && e.Subject.InstanceID == high {
					destroyed = n
				}
				if e.Kind == "damaged" && e.Target.InstanceID == low {
					damaged = n
				}
				if e.Kind == "card_drawn" {
					drawn = n
				}
			}
			if destroyed < 0 || damaged <= destroyed || drawn <= damaged {
				t.Fatal("wrong destruction/damage/lastwords ordering")
			}
		}
	}
}

func TestAmuletEntriesQueueAndResumeWithPublicIdentity(t *testing.T) {
	pack := repeatCardPack(t)
	for n := range pack.Cards {
		if pack.Cards[n].ID == 10164110 {
			pack.Cards[n].Abilities = append(pack.Cards[n].Abilities, ir.Ability{ID: strings.Repeat("e", 32), Trigger: ir.EventTrigger{Kind: "event", Event: "amulet_summoned", Side: "own", SubjectType: "amulet"}, Body: []ir.Effect{
				ir.ModeEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("f", 32)}, Kind: "mode", Options: []ir.ModeOption{{ID: 1, Body: []ir.Effect{}}}},
				ir.TargetEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("9", 32)}, Kind: "return", Target: ir.BindingRef{Kind: "binding", Name: "summoned"}, Destination: "hand"},
			}})
		}
	}
	for _, owner := range []string{"own", "oppo"} {
		state := crestState(owner)
		source, amulet := strings.Repeat("a", 32), strings.Repeat("b", 32)
		p := state.Players[owner]
		p.Zones["deck"] = nil
		p = withInstance(p, "hand", ir.TestInstance{InstanceID: source, CardID: 10164110, DeclaredType: "follower"})
		state.Players[owner] = withInstance(p, "deck", ir.TestInstance{InstanceID: amulet, CardID: 10001210, DeclaredType: "amulet"})
		s, err := NewSession(pack, state, 1)
		if err != nil {
			t.Fatal(err)
		}
		step := s.SubmitAs(strings.Repeat("c", 32), owner, SimulatorCommand{Kind: "play", Source: source})
		if step.Status != StatusSuspended || len(s.g.player(owner).deck) != 0 || s.g.instances[amulet].zone != "field" {
			t.Fatal("amulet entry did not queue after empty-hand fanfare", step)
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
			if r := current.Resume(ChoiceResponse{RequestID: step.Choice.RequestID, ActionID: step.Choice.ActionID, StateRevision: step.Choice.StateRevision, SelectedOptionID: 1}); r.Status != StatusCompleted {
				t.Fatal(r)
			}
			if current.g.instances[amulet].zone != "hand" || len(current.g.player(owner).hand) != 1 {
				t.Fatal("entry binding did not survive restore")
			}
		}
		if !reflect.DeepEqual(s.g.snapshot(), restored.g.snapshot()) {
			t.Fatal("restored amulet entry diverged")
		}
	}
}
