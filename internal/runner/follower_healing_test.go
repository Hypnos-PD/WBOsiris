package runner

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"wbo/internal/ir"
)

func healingState(side string) ir.State {
	state := testState()
	state.Turn.Active = side
	p := state.Players[side]
	p.Leader.Life = 12
	p = withInstance(p, "field", ir.TestInstance{InstanceID: strings.Repeat("1", 32), CardID: 10411110, DeclaredType: "follower", Overrides: ir.InstanceOverrides{DamageTaken: intPtr(4)}})
	state.Players[side] = p
	return state
}

func TestKouAndYouHealOnBothAttacksAndPreviewIsPure(t *testing.T) {
	for _, side := range []string{"own", "oppo"} {
		s, err := NewSession(repeatCardPack(t), healingState(side), 50)
		if err != nil {
			t.Fatal(err)
		}
		p := s.g.player(side)
		source := p.field[0]
		for n, want := range []int{5, 6} {
			before := s.g.snapshot()
			s.LegalActionsFor(side)
			if !reflect.DeepEqual(before, s.g.snapshot()) {
				t.Fatal("preview healed or consumed attacks")
			}
			if r := s.SubmitAs(fmt.Sprintf("%032x", n+1), side, SimulatorCommand{Kind: "attack", Source: source.id}); r.Status != StatusCompleted {
				t.Fatal(r)
			}
			if source.life != want || source.maxLife() != 6 || source.damageTaken != 6-want || p.leaderLife != 15+3*n {
				t.Fatal("attack healing or cap is incorrect")
			}
		}
		before := s.g.snapshot()
		if r := s.SubmitAs(strings.Repeat("b", 32), side, SimulatorCommand{Kind: "attack", Source: source.id}); r.Status != StatusIllegal {
			t.Fatal("third attack was legal", r)
		}
		if !reflect.DeepEqual(before, s.g.snapshot()) {
			t.Fatal("illegal third attack changed healing or state")
		}
		state, err := s.View(side)
		if err != nil {
			t.Fatal(err)
		}
		view := state.Own.Field[0]
		if view.Life != 6 || view.MaxLife != 6 {
			t.Fatal("public health cap is incorrect", view)
		}
	}
}

func TestFollowerHealingTracksDamageAcrossStatChanges(t *testing.T) {
	s, err := NewSession(repeatCardPack(t), healingState("own"), 50)
	if err != nil {
		t.Fatal(err)
	}
	i := s.g.own.field[0]
	check := func(life, cap, wounds int) {
		t.Helper()
		if i.life != life || i.maxLife() != cap || i.damageTaken != wounds {
			t.Fatalf("got life=%d cap=%d wounds=%d, want %d/%d with %d wounds", i.life, i.maxLife(), i.damageTaken, life, cap, wounds)
		}
	}
	check(2, 6, 4)
	i.buffStats(0, 3, "own")
	check(5, 9, 4)
	i.buffStats(0, -2, "")
	check(3, 7, 4)
	s.g.healFollower(i, 2)
	check(5, 7, 2)
	s.g.applyEvolution(i, false)
	check(7, 9, 2)
	s.g.expireTurnEffects("own")
	check(4, 6, 2)
	s.g.healFollower(i, 99)
	check(6, 6, 0)
	before := len(s.Events())
	for _, amount := range []int{0, -1, 3} {
		s.g.healFollower(i, amount)
	}
	if len(s.Events()) != before {
		t.Fatal("zero, negative or full healing emitted an event")
	}
	s.g.damageInstance(i, 4)
	s.g.execTargetEffect(ir.TargetEffect{Kind: "set_life", Target: ir.SelfRef{Kind: "self"}, Amount: 1}, i, frame{})
	s.g.healFollower(i, 99)
	check(1, 1, 0)
	if event := s.Events()[0]; event.Kind != "healed" || event.Actual != 2 || event.Target.InstanceID != i.id {
		t.Fatal("follower healing fact is incorrect", event)
	}
}

func TestFollowerHealingCopyAndReturnKeepIndependentHealth(t *testing.T) {
	s, err := NewSession(repeatCardPack(t), healingState("own"), 50)
	if err != nil {
		t.Fatal(err)
	}
	original := s.g.own.field[0]
	copy := s.g.copyInstance(original)
	s.g.addToZone(&s.g.own, copy, "field")
	copy.buffStats(0, 3, "")
	s.g.healFollower(copy, 99)
	if copy.life != 9 || copy.maxLife() != 9 || original.life != 2 || original.damageTaken != 4 {
		t.Fatal("copy shared wounds or lost the original health cap")
	}
	s.g.returnCard(original, "hand")
	if original.life != 6 || original.damageTaken != 0 || original.maxLife() != 6 {
		t.Fatal("return retained old damage")
	}
	s.g.damageInstance(copy, 8)
	s.g.execCardEffect(ir.CardEffect{Kind: "transform", Target: ir.SelfRef{Kind: "self"}, CardID: 10001110}, copy, frame{})
	s.g.healFollower(copy, 99)
	if copy.life != 2 || copy.maxLife() != 2 || copy.damageTaken != 0 {
		t.Fatal("transform retained old damage")
	}
}

func TestFollowerHealingDoesNotResurrectOrHealOtherZones(t *testing.T) {
	s, err := NewSession(repeatCardPack(t), healingState("own"), 50)
	if err != nil {
		t.Fatal(err)
	}
	i := s.g.own.field[0]
	s.g.damageInstance(i, 2)
	before := len(s.Events())
	s.g.healFollower(i, 10)
	if i.life != 0 || len(s.Events()) != before {
		t.Fatal("healing rescued a lethally damaged follower")
	}
	s.g.resolveDeathBatch(nil)
	before = len(s.Events())
	s.g.healFollower(i, 10)
	if i.zone != "graveyard" || len(s.Events()) != before {
		t.Fatal("healing changed a graveyard card")
	}
	for _, cardID := range []int{10001110, 10001210, 90044320} {
		card := s.g.newInstance(s.g.cards[cardID], fmt.Sprintf("%032x", cardID), "", "hand")
		s.g.addToZone(&s.g.own, card, "hand")
		before := s.g.snapshot()
		s.g.healFollower(card, 10)
		if !reflect.DeepEqual(before, s.g.snapshot()) {
			t.Fatal("healing changed a card outside the field")
		}
	}
}

func TestFollowerHealingContinuationRetainsWoundsAndAttackCheckpoint(t *testing.T) {
	for _, side := range []string{"own", "oppo"} {
		pack := repeatCardPack(t)
		for n := range pack.Cards {
			if pack.Cards[n].ID == 10411110 {
				a := &pack.Cards[n].Abilities[0]
				a.Body = append([]ir.Effect{ir.ModeEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("e", 32)}, Kind: "mode", Options: []ir.ModeOption{{ID: 1, Body: []ir.Effect{}}}}}, a.Body...)
			}
		}
		s, err := NewSession(pack, healingState(side), 50)
		if err != nil {
			t.Fatal(err)
		}
		step := s.SubmitAs(strings.Repeat("a", 32), side, SimulatorCommand{Kind: "attack", Source: strings.Repeat("1", 32)})
		if step.Status != StatusSuspended {
			t.Fatal(step)
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
		response := ChoiceResponse{RequestID: step.Choice.RequestID, ActionID: step.Choice.ActionID, StateRevision: step.Choice.StateRevision, SelectedOptionID: 1}
		for _, current := range []*Session{s, restored} {
			if r := current.Resume(response); r.Status != StatusCompleted {
				t.Fatal(r)
			}
			i := current.g.player(side).field[0]
			if i.life != 5 || i.maxLife() != 6 || i.damageTaken != 1 {
				t.Fatal("restored attack forgot the healing cap")
			}
		}
		if !reflect.DeepEqual(s.g.snapshot(), restored.g.snapshot()) {
			t.Fatal("restored healing changed state or events")
		}
		bad, _ := DecodeContinuation(data)
		bad.Game.Instances[0].DamageTaken = -1
		if _, err := RestoreSession(pack, bad); err == nil {
			t.Fatal("negative saved damage accepted")
		}
		old, _ := DecodeContinuation(data)
		old.Version = "0.33.0"
		if _, err := RestoreSession(pack, old); err == nil {
			t.Fatal("accepted an old continuation without reliable follower wounds")
		}
	}
}

func TestFollowerHealingMixedBindingsAndDamagePrevention(t *testing.T) {
	for _, side := range []string{"own", "oppo"} {
		s, err := NewSession(repeatCardPack(t), healingState(side), 50)
		if err != nil {
			t.Fatal(err)
		}
		p := s.g.player(side)
		i := p.field[0]
		bindings := frame{"target": append(bindEntities(i), ir.EventTarget{Kind: "leader", Side: side})}
		s.g.execTargetEffect(ir.TargetEffect{Kind: "heal", Target: ir.BindingRef{Kind: "binding", Name: "target"}, Amount: 2}, i, bindings)
		if i.life != 4 || i.damageTaken != 2 || p.leaderLife != 14 {
			t.Fatal("mixed binding failed to heal both target types")
		}
		i.addKeyword("barrier", "")
		s.g.damageInstance(i, 9)
		if i.damageTaken != 2 || i.abilities["barrier"] {
			t.Fatal("barrier prevented damage was recorded as a wound")
		}
		s.g.applyEvolution(i, true)
		s.g.damageInstance(i, 9)
		if i.damageTaken != 2 || i.maxLife() != 9 || i.life != 7 {
			t.Fatal("super evolution protection changed wounds or cap")
		}
		s.g.healFollower(i, 99)
		if i.life != 9 || i.damageTaken != 0 {
			t.Fatal("healing used prevented damage")
		}
	}
}
