package runner

import (
	"fmt"
	"strings"
	"testing"

	"wbo/internal/ir"
)

func TestReturnFromFieldResetsCardState(t *testing.T) {
	for _, side := range []string{"own", "oppo"} {
		for _, destination := range []string{"hand", "deck"} {
			for _, cardType := range []string{"follower", "amulet"} {
				t.Run(side+"/"+destination+"/"+cardType, func(t *testing.T) {
					card := ir.Card{ID: 77772001, CardType: cardType, Cost: 4, Intrinsic: []string{"ward"},
						IntrinsicState: []ir.IntrinsicState{{Kind: "countdown", Initial: 3}, {Kind: "earthsigil", Initial: 1}, {Kind: "damage_reduction", Initial: 1}, {Kind: "attack_limit", Initial: 2}}}
					if cardType == "follower" {
						card.Stats = &ir.Stats{Attack: 2, Life: 5}
					}
					state := testState()
					id := strings.Repeat("1", 32)
					state.Players[side] = withInstance(state.Players[side], "field", ir.TestInstance{InstanceID: id, Alias: "target", CardID: card.ID, DeclaredType: cardType})
					s, err := NewSession(&ir.CardPack{Cards: []ir.Card{card}}, state, 1)
					if err != nil {
						t.Fatal(err)
					}
					i := s.g.instances[id]
					i.attack, i.life, i.cost = 9, 1, 0
					i.evolved, i.superEvolved, i.departed, i.engaged = true, true, true, true
					i.attacksUsed, i.attackLimitValue, i.earthsigil, i.countdown, i.damageReduction = 2, 4, 5, 1, 8
					i.abilities = map[string]bool{"storm": true}
					s.g.returnCard(i, destination)
					if s.g.instances[id] != i || i.alias != "target" || i.zone != destination || s.g.sideOf(i) != side {
						t.Fatal("return changed identity or ownership")
					}
					if i.cost != 4 || i.evolved || i.superEvolved || i.departed || i.engaged || i.attacksUsed != 0 || i.summoningSick || i.attackLimitValue != 2 || i.earthsigil != 1 || i.countdown != 3 || i.damageReduction != 1 || !i.abilities["ward"] || i.abilities["storm"] {
						t.Fatalf("return retained modified state: %#v", i)
					}
					if cardType == "follower" && (i.attack != 2 || i.life != 5) || cardType == "amulet" && (i.attack != 0 || i.life != 0) {
						t.Fatal("return retained damage or stat buffs")
					}
					wantEvents := 0
					if cardType == "follower" {
						wantEvents = 1
					}
					if s.g.player(side).shadows != 0 || len(s.g.events) != wantEvents || wantEvents == 1 && (s.g.events[0].Kind != "follower_left" || s.g.events[0].To != destination) {
						t.Fatal("return emitted incorrect departure events")
					}
				})
			}
		}
	}
}

func TestReturnFromHandToDeckPreservesModifiers(t *testing.T) {
	card := ir.Card{ID: 77772001, CardType: "follower", Cost: 4, Stats: &ir.Stats{Attack: 2, Life: 5}}
	state := testState()
	id := strings.Repeat("1", 32)
	state.Players["own"] = withInstance(state.Players["own"], "hand", ir.TestInstance{InstanceID: id, CardID: card.ID, DeclaredType: "follower"})
	s, err := NewSession(&ir.CardPack{Cards: []ir.Card{card}}, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	i := s.g.instances[id]
	i.attack, i.life, i.cost, i.damageReduction = 7, 8, 1, 2
	i.abilities["storm"] = true
	s.g.returnCard(i, "deck")
	s.g.move(i, "hand")
	if i.attack != 7 || i.life != 8 || i.cost != 1 || i.damageReduction != 2 || !i.abilities["storm"] {
		t.Fatal("hand-to-deck return or subsequent draw lost modifiers")
	}
}

func TestReturnToFullHandOverdrawsWithoutLastwords(t *testing.T) {
	pack := repeatCardPack(t)
	state := testState()
	id := strings.Repeat("1", 32)
	actor := withInstance(state.Players["oppo"], "field", ir.TestInstance{InstanceID: id, CardID: 10001120, DeclaredType: "follower"})
	for n := 0; n < handLimit; n++ {
		actor = withInstance(actor, "hand", ir.TestInstance{InstanceID: fmt.Sprintf("%032x", n+32), CardID: 10001110, DeclaredType: "follower"})
	}
	state.Players["oppo"] = actor
	s, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	i := s.g.instances[id]
	i.evolved, i.attack, i.life = true, 6, 1
	s.g.returnCard(i, "hand")
	if i.zone != "graveyard" || i.evolved || i.attack != 0 || i.life != 2 || s.g.oppo.shadows != 1 || len(s.g.oppo.hand) != handLimit || len(s.g.triggers) != 0 || len(s.g.oppo.destroyed) != 0 {
		t.Fatalf("overdrawn bounce did not reset or acted as destruction: %#v", i)
	}
}

func TestFieldTransformResetsStateAndAttackReadiness(t *testing.T) {
	pack := &ir.CardPack{Cards: []ir.Card{
		{ID: 77772001, CardType: "follower", Cost: 1, Stats: &ir.Stats{Attack: 1, Life: 1}, Intrinsic: []string{"storm"}},
		{ID: 77772002, CardType: "follower", Cost: 5, Stats: &ir.Stats{Attack: 3, Life: 6}, Intrinsic: []string{"ward"}},
	}}
	state := testState()
	id := strings.Repeat("1", 32)
	state.Players["own"] = withInstance(state.Players["own"], "field", ir.TestInstance{InstanceID: id, CardID: 77772001, DeclaredType: "follower"})
	s, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	i := s.g.instances[id]
	i.attack, i.life, i.cost, i.attacksUsed = 9, 2, 0, 1
	i.evolved, i.superEvolved, i.engaged, i.departed = true, true, true, true
	s.g.execCardEffect(ir.CardEffect{Kind: "transform", Target: ir.SelfRef{Kind: "self"}, CardID: 77772002}, i, nil)
	if i.card.ID != 77772002 || i.cost != 5 || i.attack != 3 || i.life != 6 || i.evolved || i.superEvolved || i.engaged || i.departed || i.attacksUsed != 0 || !i.summoningSick || !i.abilities["ward"] || i.abilities["storm"] {
		t.Fatalf("transform retained old state: %#v", i)
	}
	if len(s.g.events) != 1 || s.g.events[0].Kind != "card_transformed" || len(s.g.triggers) != 0 || len(s.g.own.field) != 1 {
		t.Fatal("transform acted as entry or destruction")
	}
	if s.g.preflightAttack(ir.AttackAction{Kind: "attack_leader", Actor: "own", Attacker: id, Defender: "oppo"}) == "" {
		t.Fatal("transformed follower attacked on its transformation turn")
	}
	s.g.returnCard(i, "hand")
	if i.card.ID != 77772002 || i.cost != 5 || i.attack != 3 || i.life != 6 {
		t.Fatal("bounce reverted transformation identity")
	}
}

func TestCompiledFusionTransformRestoresMaterialsAndCanBePlayed(t *testing.T) {
	pack := repeatCardPack(t)
	state := testState()
	id, materialID := strings.Repeat("1", 32), strings.Repeat("2", 32)
	spellID, allyID := strings.Repeat("5", 32), strings.Repeat("6", 32)
	actor := state.Players["own"]
	actor.PP, actor.MaxPP = 4, 4
	actor = withInstance(actor, "hand", ir.TestInstance{InstanceID: spellID, CardID: 10012310, DeclaredType: "spell"})
	actor = withInstance(actor, "field", ir.TestInstance{InstanceID: allyID, CardID: 10001110, DeclaredType: "follower"})
	actor = withInstance(actor, "hand", ir.TestInstance{InstanceID: id, CardID: 90071210, DeclaredType: "amulet"})
	state.Players["own"] = withInstance(actor, "hand", ir.TestInstance{InstanceID: materialID, CardID: 90071220, DeclaredType: "amulet"})
	s, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	step := s.Submit(strings.Repeat("3", 32), SimulatorCommand{Kind: "fusion", Source: id})
	if step.Status != StatusSuspended {
		t.Fatal(step)
	}
	step = s.Resume(ChoiceResponse{RequestID: step.Choice.RequestID, ActionID: step.Choice.ActionID, StateRevision: step.Choice.StateRevision, SelectedInstanceIDs: []string{materialID}})
	if step.Status != StatusCompleted {
		t.Fatal(step)
	}
	i := s.g.instances[id]
	if i.card.ID != 90072110 || i.cost != 3 || i.attack != 5 || i.life != 1 || !i.abilities["rush"] || len(i.materials) != 1 || i.materials[0].id != materialID {
		t.Fatalf("core transformation lost new stats or materials: %#v", i)
	}
	step = s.Submit(strings.Repeat("7", 32), SimulatorCommand{Kind: "play", Source: spellID})
	if step.Status != StatusSuspended {
		t.Fatal("expected a selection after fusion", step)
	}
	data, err := s.EncodeContinuation()
	if err != nil {
		t.Fatal(err)
	}
	saved, err := DecodeContinuation(data)
	if err != nil {
		t.Fatal(err)
	}
	for n := 0; n < 2; n++ {
		tampered, err := DecodeContinuation(data)
		if err != nil {
			t.Fatal(err)
		}
		tampered.Game.Events[n].PrivateTo = ""
		if _, err := RestoreSession(pack, tampered); err == nil {
			t.Fatal("restored hidden card event without its visibility constraint")
		}
	}
	s, err = RestoreSession(pack, saved)
	if err != nil {
		t.Fatal("cannot restore transformed card with attached materials", err)
	}
	if s.g.instances[id].materials[0] != s.g.instances[materialID] || s.g.instances[materialID].zone != "attached" || len(s.g.own.hand) != 1 {
		t.Fatal("restoration lost material identity or returned it to hand")
	}
	ownerEvents, guestEvents := s.EventsFor("own"), s.EventsFor("oppo")
	if len(ownerEvents) < 2 || len(ownerEvents) != len(guestEvents) || ownerEvents[0].Kind != "card_fused" || ownerEvents[1].Kind != "card_transformed" {
		t.Fatal("fusion and transform event sequence did not survive restoration")
	}
	if ownerEvents[0].Subject.CardID != 90071210 || ownerEvents[1].Subject.CardID != 90071210 || ownerEvents[1].Target.CardID != 90072110 {
		t.Fatal("event identities changed with transformed source")
	}
	for n := 0; n < 2; n++ {
		if guestEvents[n].Subject != nil || guestEvents[n].Target != nil || guestEvents[n].InstanceID != "" || guestEvents[n].CardID != 0 || guestEvents[n].Sequence != ownerEvents[n].Sequence {
			t.Fatal("restored event exposed hidden identity or changed replay index")
		}
	}
	step = s.Resume(ChoiceResponse{RequestID: step.Choice.RequestID, ActionID: step.Choice.ActionID, StateRevision: step.Choice.StateRevision, SelectedInstanceIDs: []string{allyID}})
	if step.Status != StatusCompleted {
		t.Fatal("restored choice failed", step)
	}
	step = s.Submit(strings.Repeat("4", 32), SimulatorCommand{Kind: "play", Source: id})
	if step.Status != StatusCompleted || s.g.own.pp != 0 || len(s.g.own.field) != 1 || s.g.own.field[0].life != 1 {
		t.Fatal("restored transformation could not be played at its new cost", step)
	}
}

func TestRestoreAttachedMaterialOwnership(t *testing.T) {
	card := &ir.Card{ID: 77772001, CardType: "amulet"}
	rootID, childID, nestedID := strings.Repeat("1", 32), strings.Repeat("2", 32), strings.Repeat("3", 32)
	cards := map[int]*ir.Card{card.ID: card}
	state := testState()
	state.Players["oppo"] = withInstance(state.Players["oppo"], "hand", ir.TestInstance{InstanceID: rootID, CardID: card.ID, DeclaredType: "amulet"})
	s, err := NewSession(&ir.CardPack{Cards: []ir.Card{*card}}, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	root := s.g.instances[rootID]
	child := s.g.newInstance(card, childID, "child", "attached")
	nested := s.g.newInstance(card, nestedID, "nested", "attached")
	root.materials, child.materials = []*instance{child}, []*instance{nested}
	restored, err := restoreGame(cards, snapshotContinuationGame(s.g))
	if err != nil || restored.sideOf(restored.instances[nestedID]) != "oppo" {
		t.Fatal("nested materials lost their owner's side", err)
	}
	for _, corruption := range []string{"shared", "cycle", "orphan", "hand"} {
		t.Run(corruption, func(t *testing.T) {
			saved := snapshotContinuationGame(s.g)
			switch corruption {
			case "shared":
				saved.Instances[0].Materials = append(saved.Instances[0].Materials, nestedID)
			case "cycle":
				saved.Instances[2].Materials = []string{childID}
			case "orphan":
				saved.Instances[0].Materials = nil
			case "hand":
				saved.Instances[1].Zone = "hand"
			}
			if _, err := restoreGame(cards, saved); err == nil {
				t.Fatal("accepted invalid attachment graph")
			}
		})
	}
}
