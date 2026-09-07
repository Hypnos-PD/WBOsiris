package runner

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"wbo/internal/ir"
	"wbo/internal/project"
)

func costBoostAbility() ir.Ability {
	return ir.Ability{ID: strings.Repeat("a", 32), Trigger: ir.SimpleTrigger{Kind: "spellboost"}, Body: []ir.Effect{
		ir.AdjustEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("b", 32)}, Kind: "adjust_entity_field", Field: "cost", Target: ir.SelfRef{Kind: "self"}, Delta: -1, Minimum: 0},
	}}
}

func TestCompiledDrawSpellBoostsHeldCardBeforeNextPlay(t *testing.T) {
	root := filepath.Join("..", "..")
	loaded := project.LoadWithRoot([]string{
		filepath.Join(root, "cards", "10000", "10031310.wbo"),
		filepath.Join(root, "cards", "10000", "10032120.wbo"),
	}, false, root)
	if loaded.HasErrors() {
		t.Fatalf("card loading failed: %v", loaded.Diagnostics)
	}
	pack, _, err := project.BuildRuntimePacks(loaded)
	if err != nil {
		t.Fatal(err)
	}
	for _, side := range []string{"own", "oppo"} {
		t.Run(side, func(t *testing.T) {
			sourceID, heldID, drawnID := strings.Repeat("1", 32), strings.Repeat("2", 32), strings.Repeat("3", 32)
			state := testState()
			state.Turn.Active = side
			actor := state.Players[side]
			actor.PP, actor.MaxPP = 10, 10
			actor = withInstance(actor, "hand", ir.TestInstance{InstanceID: sourceID, CardID: 10031310, DeclaredType: "spell"})
			actor = withInstance(actor, "hand", ir.TestInstance{InstanceID: heldID, CardID: 10032120, DeclaredType: "follower"})
			state.Players[side] = withInstance(actor, "deck", ir.TestInstance{InstanceID: drawnID, CardID: 10032120, DeclaredType: "follower"})
			session, err := NewSession(pack, state, 1)
			if err != nil {
				t.Fatal(err)
			}
			if result := session.SubmitAs(strings.Repeat("4", 32), side, SimulatorCommand{Kind: "play", Source: sourceID}); result.Status != StatusCompleted {
				t.Fatalf("draw spell failed: %#v", result)
			}
			view, err := session.View(side)
			if err != nil {
				t.Fatal(err)
			}
			if view.Own.Combo != 1 || view.Own.PP != 9 || len(view.Own.Hand) != 2 || view.Own.Hand[0].InstanceID != heldID || view.Own.Hand[0].Cost != 9 || view.Own.Hand[1].InstanceID != drawnID || view.Own.Hand[1].Cost != 10 {
				t.Fatalf("compiled spell produced incorrect public state: %#v", view.Own)
			}
			for _, action := range session.LegalActionsFor(side) {
				if action.Kind == "play" && action.Source == drawnID {
					t.Fatal("newly drawn card received a retroactive boost")
				}
			}
			if result := session.SubmitAs(strings.Repeat("5", 32), side, SimulatorCommand{Kind: "play", Source: heldID}); result.Status != StatusCompleted {
				t.Fatalf("boosted card was not playable for nine PP: %#v", result)
			}
			view, err = session.View(side)
			if err != nil {
				t.Fatal(err)
			}
			if view.Own.Combo != 2 || view.Own.PP != 0 || len(view.Own.Field) != 1 || view.Own.Field[0].Attack != 8 || view.Own.Field[0].Life != 6 || len(view.Own.Hand) != 1 || view.Own.Hand[0].Cost != 10 {
				t.Fatalf("boosted follower play produced incorrect state: %#v", view.Own)
			}
		})
	}
}

func TestSpellPlayBoostsOnlyExistingControllerHand(t *testing.T) {
	for _, side := range []string{"own", "oppo"} {
		t.Run(side, func(t *testing.T) {
			sourceID, heldID, drawnID := strings.Repeat("1", 32), strings.Repeat("2", 32), strings.Repeat("3", 32)
			fieldID, enemyID := strings.Repeat("4", 32), strings.Repeat("5", 32)
			pack := &ir.CardPack{Cards: []ir.Card{
				{ID: 66666691, CardType: "spell", Cost: 2, Abilities: []ir.Ability{costBoostAbility()}, PlayEffects: []ir.Effect{
					ir.DrawEffect{Kind: "draw", Count: 1},
					ir.AdjustEffect{Kind: "spellboost", Target: ir.ZoneRef{Kind: "zone", Side: "own", Zone: "hand"}, Times: 2},
				}},
				{ID: 66666692, CardType: "follower", Cost: 6, Stats: &ir.Stats{Attack: 1, Life: 1}, Abilities: []ir.Ability{costBoostAbility()}},
			}}
			state := testState()
			state.Turn.Active = side
			player := state.Players[side]
			player.PP, player.MaxPP = 2, 2
			player = withInstance(player, "hand", ir.TestInstance{InstanceID: sourceID, CardID: 66666691, DeclaredType: "spell"})
			for zone, id := range map[string]string{"hand": heldID, "deck": drawnID, "field": fieldID} {
				player = withInstance(player, zone, ir.TestInstance{InstanceID: id, CardID: 66666692, DeclaredType: "follower"})
			}
			state.Players[side] = player
			enemy := oppositeSide(side)
			state.Players[enemy] = withInstance(state.Players[enemy], "hand", ir.TestInstance{InstanceID: enemyID, CardID: 66666692, DeclaredType: "follower"})
			session, err := NewSession(pack, state, 1)
			if err != nil {
				t.Fatal(err)
			}
			before := session.g.snapshot()
			for range 2 {
				session.LegalActionsFor(side)
			}
			if !reflect.DeepEqual(before, session.g.snapshot()) || session.g.instances[heldID].cost != 6 {
				t.Fatal("legality enumeration mutated spellboost state")
			}
			result := session.SubmitAs(strings.Repeat("6", 32), side, SimulatorCommand{Kind: "play", Source: sourceID})
			if result.Status != StatusCompleted {
				t.Fatalf("play failed: %#v", result)
			}
			for id, expected := range map[string]int{sourceID: 2, heldID: 3, drawnID: 4, fieldID: 6, enemyID: 6} {
				if actual := session.g.instances[id].cost; actual != expected {
					t.Fatalf("instance %s cost=%d, want %d", id, actual, expected)
				}
			}
		})
	}
}

func TestSpellboostSurvivesReturnToDeckAndContinuation(t *testing.T) {
	for _, redraw := range []bool{false, true} {
		t.Run(map[bool]string{false: "left_in_deck", true: "redrawn"}[redraw], func(t *testing.T) {
			sourceID, heldID, otherID := strings.Repeat("1", 32), strings.Repeat("2", 32), strings.Repeat("3", 32)
			drawCard := 66666693
			if redraw {
				drawCard = 66666692
			}
			pack := &ir.CardPack{Cards: []ir.Card{
				{ID: 66666691, CardType: "spell", PlayEffects: []ir.Effect{
					ir.TargetEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("4", 32)}, Kind: "return", Target: ir.ZoneRef{Kind: "zone", Side: "own", Zone: "hand"}, Destination: "deck"},
					ir.ModeEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("5", 32)}, Kind: "mode", Options: []ir.ModeOption{{ID: 1}}},
					ir.DrawEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("6", 32)}, Kind: "draw", Count: 1, Predicate: ir.FieldPredicate{Kind: "has_card", CardID: drawCard}},
					ir.AdjustEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("7", 32)}, Kind: "spellboost", Target: ir.ZoneRef{Kind: "zone", Side: "own", Zone: "hand"}, Times: 5},
				}},
				{ID: 66666692, CardType: "follower", Cost: 6, Stats: &ir.Stats{Attack: 1, Life: 1}, Abilities: []ir.Ability{costBoostAbility()}},
				{ID: 66666693, CardType: "follower", Cost: 6, Stats: &ir.Stats{Attack: 1, Life: 1}, Abilities: []ir.Ability{costBoostAbility()}},
			}}
			state := testState()
			own := state.Players["own"]
			own = withInstance(own, "hand", ir.TestInstance{InstanceID: sourceID, CardID: 66666691, DeclaredType: "spell"})
			own = withInstance(own, "hand", ir.TestInstance{InstanceID: heldID, CardID: 66666692, DeclaredType: "follower"})
			state.Players["own"] = withInstance(own, "deck", ir.TestInstance{InstanceID: otherID, CardID: 66666693, DeclaredType: "follower"})
			session, err := NewSession(pack, state, 1)
			if err != nil {
				t.Fatal(err)
			}
			result := session.Submit(strings.Repeat("8", 32), SimulatorCommand{Kind: "play", Source: sourceID})
			if result.Status != StatusSuspended || session.g.instances[heldID].zone != "deck" || session.g.instances[heldID].cost != 6 || len(session.g.triggers) != 1 {
				t.Fatalf("spellboost did not wait for spell resolution: result=%#v cost=%d triggers=%d", result, session.g.instances[heldID].cost, len(session.g.triggers))
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
			choice := result.Choice
			response := ChoiceResponse{RequestID: choice.RequestID, ActionID: choice.ActionID, StateRevision: choice.StateRevision, SelectedOptionID: 1}
			for _, current := range []*Session{session, restored} {
				if step := current.Resume(response); step.Status != StatusCompleted {
					t.Fatalf("resume failed: %#v", step)
				}
				wantCost, wantZone, otherCost := 5, "deck", 1
				if redraw {
					wantCost, wantZone, otherCost = 0, "hand", 6
				}
				card := current.g.instances[heldID]
				if card.cost != wantCost || card.zone != wantZone || current.g.instances[otherID].cost != otherCost {
					t.Fatalf("return/redraw spellboost mismatch: cost=%d zone=%s other=%d", card.cost, card.zone, current.g.instances[otherID].cost)
				}
			}
			if !reflect.DeepEqual(session.g.snapshot(), restored.g.snapshot()) {
				t.Fatal("restored and uninterrupted spell resolution diverged")
			}
		})
	}
}

func TestRejectedSpellAndNonSpellDoNotSpellboost(t *testing.T) {
	for _, cardType := range []string{"spell", "follower", "amulet"} {
		t.Run(cardType, func(t *testing.T) {
			sourceID, heldID := strings.Repeat("1", 32), strings.Repeat("2", 32)
			source := ir.Card{ID: 66666691, CardType: cardType}
			if cardType == "spell" {
				source.Cost = 1
			} else if cardType == "follower" {
				source.Stats = &ir.Stats{Attack: 1, Life: 1}
			}
			pack := &ir.CardPack{Cards: []ir.Card{source, {ID: 66666692, CardType: "spell", Cost: 6, Abilities: []ir.Ability{costBoostAbility()}}}}
			state := testState()
			own := withInstance(state.Players["own"], "hand", ir.TestInstance{InstanceID: sourceID, CardID: source.ID, DeclaredType: cardType})
			state.Players["own"] = withInstance(own, "hand", ir.TestInstance{InstanceID: heldID, CardID: 66666692, DeclaredType: "spell"})
			session, err := NewSession(pack, state, 1)
			if err != nil {
				t.Fatal(err)
			}
			result := session.Submit(strings.Repeat("3", 32), SimulatorCommand{Kind: "play", Source: sourceID})
			want := StatusCompleted
			if cardType == "spell" {
				want = StatusIllegal
			}
			if result.Status != want || session.g.instances[heldID].cost != 6 || len(session.g.triggers) != 0 {
				t.Fatalf("unexpected boost: result=%#v cost=%d", result, session.g.instances[heldID].cost)
			}
		})
	}
}
