package runner

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"wbo/internal/ir"
)

func TestPlayCountsItselfBeforeComboRequirements(t *testing.T) {
	for _, side := range []string{"own", "oppo"} {
		for _, cardType := range []string{"spell", "follower", "amulet"} {
			for _, hasTarget := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/target_%t", side, cardType, hasTarget), func(t *testing.T) {
					sourceID, targetID := strings.Repeat("1", 32), strings.Repeat("2", 32)
					body := []ir.Effect{ir.IfEffect{
						NodeBase: ir.NodeBase{ID: strings.Repeat("3", 32)}, Kind: "if",
						Condition: ir.CompareCondition{Kind: "compare", Op: "ge", Left: ir.Scalar{Kind: "scalar", Side: "own", Field: "combo"}, Right: 3},
						Then: []ir.Effect{
							ir.SelectionEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("4", 32)}, Kind: "require", Policy: "required", Binding: "target", Source: ir.ZoneRef{Kind: "zone", Side: "oppo", Zone: "field", Member: "follower"}},
							ir.TargetEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("5", 32)}, Kind: "destroy", Target: ir.BindingRef{Kind: "binding", Name: "target"}},
							ir.CardEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("6", 32)}, Kind: "summon", Owner: "own", CardID: 66666682, Count: 1},
						},
					}}
					card := ir.Card{ID: 66666681, CardType: cardType, Cost: 1}
					if cardType == "spell" {
						card.PlayEffects = body
					} else {
						card.Abilities = []ir.Ability{{ID: strings.Repeat("7", 32), Trigger: ir.SimpleTrigger{Kind: "fanfare"}, Body: body}}
					}
					if cardType == "follower" {
						card.Stats = &ir.Stats{Attack: 1, Life: 1}
					}
					pack := &ir.CardPack{Cards: []ir.Card{card, {ID: 66666682, CardType: "follower", Stats: &ir.Stats{Attack: 1, Life: 1}}}}
					state := testState()
					state.Turn.Active = side
					actor := state.Players[side]
					actor.Combo, actor.PP, actor.MaxPP = 2, 1, 1
					state.Players[side] = withInstance(actor, "hand", ir.TestInstance{InstanceID: sourceID, CardID: card.ID, DeclaredType: cardType})
					if hasTarget {
						enemy := oppositeSide(side)
						state.Players[enemy] = withInstance(state.Players[enemy], "field", ir.TestInstance{InstanceID: targetID, CardID: 66666682, DeclaredType: "follower"})
					}
					session, err := NewSession(pack, state, 1)
					if err != nil {
						t.Fatal(err)
					}
					before := session.g.snapshot()
					for range 2 {
						canPlay := false
						for _, action := range session.LegalActionsFor(side) {
							canPlay = canPlay || action.Kind == "play" && action.Source == sourceID
						}
						if canPlay != hasTarget {
							t.Fatalf("play legality=%t, target present=%t", canPlay, hasTarget)
						}
					}
					if !reflect.DeepEqual(before, session.g.snapshot()) {
						t.Fatal("legal action enumeration mutated state")
					}
					result := session.SubmitAs(strings.Repeat("8", 32), side, SimulatorCommand{Kind: "play", Source: sourceID})
					if !hasTarget {
						if result.Status != StatusIllegal || result.IllegalCode != "target_required" || !reflect.DeepEqual(before, session.g.snapshot()) {
							t.Fatalf("missing combo target did not reject atomically: %#v", result)
						}
						return
					}
					if result.Status != StatusSuspended || session.g.player(side).combo != 3 {
						t.Fatalf("play did not count itself before selection: %#v combo=%d", result, session.g.player(side).combo)
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
					response := ChoiceResponse{RequestID: choice.RequestID, ActionID: choice.ActionID, StateRevision: choice.StateRevision, SelectedInstanceIDs: []string{targetID}}
					for _, current := range []*Session{session, restored} {
						if step := current.Resume(response); step.Status != StatusCompleted {
							t.Fatalf("resume failed: %#v", step)
						}
						view, err := current.View(side)
						if err != nil {
							t.Fatal(err)
						}
						if view.Own.Combo != 3 || view.Oppo.Combo != 0 || view.Own.PP != 0 || current.g.instances[targetID].zone != "graveyard" {
							t.Fatalf("resume or effect summon changed play count: %#v", view)
						}
					}
					if !reflect.DeepEqual(session.g.snapshot(), restored.g.snapshot()) {
						t.Fatal("restored play diverged")
					}
				})
			}
		}
	}
}
