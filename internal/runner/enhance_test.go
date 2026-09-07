package runner

import (
	"strings"
	"testing"

	"wbo/internal/ir"
)

func TestEnhanceThresholdsAcrossCardTypes(t *testing.T) {
	for _, cardType := range []string{"follower", "spell", "amulet"} {
		for _, tc := range []struct {
			name                           string
			pp, cost, lower, spent, damage int
		}{
			{"ordinary", 2, 2, 3, 2, 1},
			{"lower_only", 3, 2, 3, 3, 3},
			{"both_tiers", 5, 2, 3, 5, 7},
			{"remaining_pp", 7, 2, 3, 5, 7},
			{"raised_cost", 5, 9, 3, 5, 7},
			{"reduced_cost", 5, 0, 3, 5, 7},
			{"zero_enhance", 0, 2, 0, 0, 3},
			{"zero_and_higher", 5, 2, 0, 5, 7},
		} {
			t.Run(cardType+"/"+tc.name, func(t *testing.T) {
				damage := func(amount int) []ir.Effect {
					return []ir.Effect{ir.TargetEffect{Kind: "damage", Target: ir.LeaderRef{Kind: "leader", Side: "oppo"}, Amount: amount}}
				}
				sourceID := strings.Repeat("1", 32)
				card := ir.Card{ID: 66666696, CardType: cardType, Cost: 2, PlayEffects: damage(1), Abilities: []ir.Ability{
					{ID: strings.Repeat("2", 32), Trigger: ir.CostTrigger{Kind: "enhance", Cost: 5}, Body: damage(4)},
					{ID: strings.Repeat("3", 32), Trigger: ir.CostTrigger{Kind: "enhance", Cost: tc.lower}, Body: damage(2)},
				}}
				if cardType == "follower" {
					card.Stats = &ir.Stats{Attack: 1, Life: 1}
				}
				state := testState()
				own := state.Players["own"]
				own.PP, own.MaxPP = tc.pp, tc.pp
				state.Players["own"] = withInstance(own, "hand", ir.TestInstance{InstanceID: sourceID, CardID: card.ID, DeclaredType: cardType})
				session, err := NewSession(&ir.CardPack{Cards: []ir.Card{card}}, state, 1)
				if err != nil {
					t.Fatal(err)
				}
				session.g.instances[sourceID].cost = tc.cost
				result := session.Submit(strings.Repeat("4", 32), SimulatorCommand{Kind: "play", Source: sourceID})
				if result.Status != StatusCompleted || session.g.own.pp != tc.pp-tc.spent || session.g.oppo.leaderLife != 20-tc.damage {
					t.Fatalf("result=%#v pp=%d life=%d", result, session.g.own.pp, session.g.oppo.leaderLife)
				}
			})
		}
	}
}

func TestEnhanceLowerTierRequirementAndContinuation(t *testing.T) {
	sourceID, targetID := strings.Repeat("1", 32), strings.Repeat("2", 32)
	pack := &ir.CardPack{Cards: []ir.Card{{
		ID: 66666696, CardType: "spell", Cost: 2, Abilities: []ir.Ability{
			{ID: strings.Repeat("3", 32), Trigger: ir.CostTrigger{Kind: "enhance", Cost: 3}, Body: []ir.Effect{
				ir.SelectionEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("4", 32)}, Kind: "require", Policy: "required", Binding: "target", Source: ir.ZoneRef{Kind: "zone", Side: "oppo", Zone: "field", Member: "follower"}},
				ir.TargetEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("5", 32)}, Kind: "destroy", Target: ir.BindingRef{Kind: "binding", Name: "target"}},
			}},
			{ID: strings.Repeat("6", 32), Trigger: ir.CostTrigger{Kind: "enhance", Cost: 5}, Body: []ir.Effect{
				ir.TargetEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("7", 32)}, Kind: "damage", Target: ir.LeaderRef{Kind: "leader", Side: "oppo"}, Amount: 4},
			}},
		},
	}, {ID: 66666697, CardType: "follower", Stats: &ir.Stats{Attack: 1, Life: 1}}}}
	for _, hasTarget := range []bool{false, true} {
		state := testState()
		own := state.Players["own"]
		own.PP, own.MaxPP = 5, 5
		state.Players["own"] = withInstance(own, "hand", ir.TestInstance{InstanceID: sourceID, CardID: 66666696, DeclaredType: "spell"})
		if hasTarget {
			state.Players["oppo"] = withInstance(state.Players["oppo"], "field", ir.TestInstance{InstanceID: targetID, CardID: 66666697, DeclaredType: "follower"})
		}
		session, err := NewSession(pack, state, 1)
		if err != nil {
			t.Fatal(err)
		}
		legal := false
		for _, action := range session.LegalActions() {
			legal = legal || action.Kind == "play" && action.Source == sourceID
		}
		if legal != hasTarget {
			t.Fatalf("play legality=%t, target present=%t", legal, hasTarget)
		}
		result := session.Submit(strings.Repeat("8", 32), SimulatorCommand{Kind: "play", Source: sourceID})
		if !hasTarget {
			if result.Status != StatusIllegal || result.IllegalCode != "target_required" || session.g.own.pp != 5 || session.g.instances[sourceID].zone != "hand" || session.g.oppo.leaderLife != 20 {
				t.Fatalf("missing lower-tier target did not reject atomically: %#v", result)
			}
			continue
		}
		if result.Status != StatusSuspended || session.g.own.pp != 0 {
			t.Fatalf("lower-tier choice was not requested after payment: %#v", result)
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
		result = restored.Resume(ChoiceResponse{RequestID: choice.RequestID, ActionID: choice.ActionID, StateRevision: choice.StateRevision, SelectedInstanceIDs: []string{targetID}})
		if result.Status != StatusCompleted || restored.g.instances[targetID].zone != "graveyard" || restored.g.oppo.leaderLife != 16 || restored.g.own.pp != 0 {
			t.Fatalf("restored enhancement did not finish both tiers exactly once: %#v", result)
		}
	}
}
