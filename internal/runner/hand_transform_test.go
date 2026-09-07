package runner

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"wbo/internal/ir"
)

func TestRoseQueenBatchTransformPreservesIdentityPrivacyAndContinuation(t *testing.T) {
	pack := repeatCardPack(t)
	for n := range pack.Cards {
		if pack.Cards[n].ID == 10114120 {
			ability := &pack.Cards[n].Abilities[0]
			ability.Body = append(ability.Body, ir.ModeEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("e", 32)}, Kind: "mode", Options: []ir.ModeOption{{ID: 1, Body: []ir.Effect{}}}})
		}
	}
	for _, owner := range []string{"own", "oppo"} {
		state := testState()
		state.Turn.Active, state.Turn.Number = owner, 9
		p := state.Players[owner]
		p.PP, p.MaxPP = 10, 10
		source := strings.Repeat("a", 32)
		p = withInstance(p, "hand", ir.TestInstance{InstanceID: source, CardID: 10114120, DeclaredType: "follower"})
		entries := []struct {
			card, cost int
			kind       string
			transforms bool
		}{
			{90011110, 1, "follower", true}, {10112210, 2, "amulet", true}, {90011310, 0, "spell", true},
			{10113130, 2, "follower", true}, {90011110, 3, "follower", false}, {10001110, 2, "follower", false},
			{90071210, 0, "amulet", false}, {10114120, 2, "follower", true},
		}
		for n, entry := range entries {
			p = withInstance(p, "hand", ir.TestInstance{InstanceID: fmt.Sprintf("%032x", n+1), CardID: entry.card, DeclaredType: entry.kind, Overrides: ir.InstanceOverrides{Cost: &entry.cost}})
		}
		p = withInstance(p, "deck", ir.TestInstance{InstanceID: strings.Repeat("b", 32), CardID: 90011110, DeclaredType: "follower"})
		state.Players[owner] = withInstance(p, "field", ir.TestInstance{InstanceID: strings.Repeat("c", 32), CardID: 90011110, DeclaredType: "follower"})
		state.Players[oppositeSide(owner)] = withInstance(state.Players[oppositeSide(owner)], "hand", ir.TestInstance{InstanceID: strings.Repeat("d", 32), CardID: 90011110, DeclaredType: "follower"})
		s, err := NewSession(pack, state, 1)
		if err != nil {
			t.Fatal(err)
		}
		originals := append([]*instance(nil), s.g.player(owner).hand[1:]...)
		originals[3].buffStats(2, 3, owner)
		originals[3].addKeyword("barrier", owner)
		step := s.SubmitAs(strings.Repeat("f", 32), owner, SimulatorCommand{Kind: "play", Source: source})
		if step.Status != StatusSuspended || s.g.player(owner).pp != 1 || len(s.g.player(owner).hand) != len(entries) {
			t.Fatal(step)
		}
		for n, i := range s.g.player(owner).hand {
			if i != originals[n] || i.id != fmt.Sprintf("%032x", n+1) {
				t.Fatal("batch changed identity or hand order")
			}
			if entries[n].transforms {
				if i.card.ID != 90014310 || i.cost != 1 || i.attack != 0 || i.life != 0 || len(i.abilities) != 0 || len(i.temporaryStats) != 0 || len(i.temporaryKeywords) != 0 {
					t.Fatal("transformation retained mutable state", i)
				}
			} else if i.card.ID != entries[n].card || i.cost != entries[n].cost {
				t.Fatal("wrong class or current cost transformed")
			}
		}
		view, err := s.View(oppositeSide(owner))
		if err != nil || view.PendingChoice != nil || len(view.Oppo.Hand) != 0 || view.Oppo.HandCount != len(entries) {
			t.Fatal("hand transformation leaked", err)
		}
		transforms := 0
		for _, event := range s.Events() {
			if event.Kind == "card_transformed" {
				transforms++
				if event.PrivateTo != owner || event.Subject.InstanceID != event.Target.InstanceID || event.Target.CardID != 90014310 {
					t.Fatal(event)
				}
			}
		}
		if transforms != 5 || s.g.player(owner).shadows != 0 || s.g.rng.Consumed() != 0 {
			t.Fatal("wrong transformation count or side effects")
		}
		for _, event := range s.EventsFor(oppositeSide(owner)) {
			if event.Kind == "card_transformed" && (event.Subject != nil || event.Target != nil || event.CardID != 0 || event.InstanceID != "") {
				t.Fatal("private transformed identity leaked", event)
			}
		}
		data, err := s.EncodeContinuation()
		if err != nil {
			t.Fatal(err)
		}
		checkpoint, err := DecodeContinuation(data)
		if err != nil {
			t.Fatal(err)
		}
		restored, err := RestoreSession(pack, checkpoint)
		if err != nil {
			t.Fatal(err)
		}
		for _, current := range []*Session{s, restored} {
			if r := current.Resume(ChoiceResponse{RequestID: step.Choice.RequestID, ActionID: step.Choice.ActionID, StateRevision: step.Choice.StateRevision, SelectedOptionID: 1}); r.Status != StatusCompleted {
				t.Fatal(r)
			}
			play := current.SubmitAs(strings.Repeat("9", 32), owner, SimulatorCommand{Kind: "play", Source: originals[0].id})
			if play.Status != StatusSuspended {
				t.Fatal("transformed spell is not playable", play)
			}
			if r := current.Resume(ChoiceResponse{RequestID: play.Choice.RequestID, ActionID: play.Choice.ActionID, StateRevision: play.Choice.StateRevision, SelectedLeaderSides: []string{oppositeSide(owner)}}); r.Status != StatusCompleted {
				t.Fatal(r)
			}
			if current.g.player(oppositeSide(owner)).leaderLife != 17 || current.g.player(owner).pp != 0 {
				t.Fatal("transformed spell kept old behavior or cost")
			}
		}
		if !reflect.DeepEqual(s.g.snapshot(), restored.g.snapshot()) {
			t.Fatal("batch continuation diverged")
		}
	}
}

func TestTransformIgnoresHistoricalZonesAndFieldToSpell(t *testing.T) {
	state := testState()
	for n, zone := range []string{"hand", "deck", "field", "graveyard", "banished", "destroyed"} {
		state.Players["oppo"] = withInstance(state.Players["oppo"], zone, ir.TestInstance{InstanceID: fmt.Sprintf("%032x", n+1), CardID: 10001110, DeclaredType: "follower"})
	}
	s, err := NewSession(repeatCardPack(t), state, 1)
	if err != nil {
		t.Fatal(err)
	}
	var targets []*instance
	for n := 1; n <= 6; n++ {
		targets = append(targets, s.g.instances[fmt.Sprintf("%032x", n)])
	}
	s.g.execCardEffect(ir.CardEffect{Kind: "transform", CardID: 90014310, Target: ir.BindingRef{Kind: "binding", Name: "targets"}}, nil, frame{"targets": bindEntities(targets...)})
	for n, i := range targets {
		want := 10001110
		if n < 2 {
			want = 90014310
		}
		if i.card.ID != want {
			t.Fatal("transformed an invalid zone", i.zone, i.card.ID)
		}
	}
	if len(s.g.events) != 2 {
		t.Fatal("invalid transforms emitted events")
	}
	for _, event := range s.g.events {
		if event.PrivateTo != "oppo" {
			t.Fatal("deck or hand identity leaked")
		}
	}
	field := targets[2]
	field.evolved, field.engaged, field.attacksUsed = true, true, 1
	s.g.execCardEffect(ir.CardEffect{Kind: "transform", CardID: 10171140, Target: ir.SelfRef{Kind: "self"}}, field, nil)
	if field.card.ID != 10171140 || field.evolved || field.engaged || field.attacksUsed != 0 || !field.summoningSick || len(s.g.triggers) != 0 || s.g.events[2].PrivateTo != "" {
		t.Fatal("field transformation did not reset state or triggered entry")
	}
	if len(s.g.triggerIndex.abilities("follower_summoned", field)) != 1 {
		t.Fatal("new field listener missing")
	}
	if s.g.preflightAttack(ir.AttackAction{Kind: "attack_leader", Actor: "oppo", Attacker: field.id, Defender: "own"}) == "" {
		t.Fatal("transformed follower can attack immediately")
	}
}

func TestBatchTransformCancelsQueuedHandListeners(t *testing.T) {
	pack := repeatCardPack(t)
	pack.Cards = append(pack.Cards, ir.Card{ID: 88880006, CardType: "spell", PlayEffects: []ir.Effect{
		ir.TargetEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("b", 32)}, Kind: "return", Destination: "hand", Target: ir.ZoneRef{Kind: "zone", Side: "own", Zone: "field", Member: "follower"}},
		ir.CardEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("c", 32)}, Kind: "transform", CardID: 90014310, PreserveInstanceID: true, PreserveMaterials: true, Target: ir.FilterRef{Kind: "filter", Source: ir.ZoneRef{Kind: "zone", Side: "own", Zone: "hand", Member: "card"}, Predicate: ir.FieldPredicate{Kind: "has_card", CardID: 10113130}}},
		ir.ModeEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("d", 32)}, Kind: "mode", Options: []ir.ModeOption{{ID: 1, Body: []ir.Effect{}}}},
	}})
	state := testState()
	source, bayle := strings.Repeat("1", 32), strings.Repeat("2", 32)
	p := withInstance(state.Players["own"], "hand", ir.TestInstance{InstanceID: source, CardID: 88880006, DeclaredType: "spell"})
	p = withInstance(p, "hand", ir.TestInstance{InstanceID: bayle, CardID: 10113130, DeclaredType: "follower"})
	state.Players["own"] = withInstance(p, "field", ir.TestInstance{InstanceID: strings.Repeat("3", 32), CardID: 10001110, DeclaredType: "follower"})
	s, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	step := s.Submit(strings.Repeat("a", 32), SimulatorCommand{Kind: "play", Source: source})
	if step.Status != StatusSuspended || len(s.g.triggers) != 0 || s.g.instances[bayle].cost != 1 {
		t.Fatal("old listener survived transformation", step)
	}
	data, err := s.EncodeContinuation()
	if err != nil {
		t.Fatal(err)
	}
	checkpoint, err := DecodeContinuation(data)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := RestoreSession(pack, checkpoint)
	if err != nil {
		t.Fatal(err)
	}
	for _, current := range []*Session{s, restored} {
		if r := current.Resume(ChoiceResponse{RequestID: step.Choice.RequestID, ActionID: step.Choice.ActionID, StateRevision: step.Choice.StateRevision, SelectedOptionID: 1}); r.Status != StatusCompleted || current.g.instances[bayle].cost != 1 {
			t.Fatal("stale listener discounted the new card", r)
		}
	}
	if !reflect.DeepEqual(s.g.snapshot(), restored.g.snapshot()) {
		t.Fatal("listener cancellation restore diverged")
	}
}

func TestRoseQueenCanAttackTwiceAndNotThreeTimes(t *testing.T) {
	state := testState()
	source := strings.Repeat("1", 32)
	state.Players["own"] = withInstance(state.Players["own"], "field", ir.TestInstance{InstanceID: source, CardID: 10114120, DeclaredType: "follower"})
	s, err := NewSession(repeatCardPack(t), state, 1)
	if err != nil {
		t.Fatal(err)
	}
	for n := 1; n <= 3; n++ {
		r := s.Submit(fmt.Sprintf("%032x", n+1), SimulatorCommand{Kind: "attack", Source: source})
		if n < 3 && r.Status != StatusCompleted || n == 3 && r.Status != StatusIllegal {
			t.Fatal("wrong attack limit", n, r)
		}
	}
	if s.g.oppo.leaderLife != 8 || s.g.instances[source].attacksUsed != 2 {
		t.Fatal("third attack changed state")
	}
}
