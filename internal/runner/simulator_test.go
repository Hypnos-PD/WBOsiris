package runner

import (
	"strings"
	"testing"

	"wbo/internal/ir"
)

func TestSimulatorViewHidesPrivateZonesAndFlipsPerspective(t *testing.T) {
	pack := &ir.CardPack{Cards: []ir.Card{
		{ID: 12345678, CardType: "spell"},
		{ID: 23456789, CardType: "follower", Stats: &ir.Stats{Attack: 2, Life: 3}, Intrinsic: []string{"ward"}},
	}}
	state := testState()
	ownHand, oppoHand, fieldID := strings.Repeat("1", 32), strings.Repeat("2", 32), strings.Repeat("3", 32)
	state.Players["own"] = withInstance(state.Players["own"], "hand", ir.TestInstance{InstanceID: ownHand, Alias: "own-hand", CardID: 12345678, DeclaredType: "spell"})
	state.Players["oppo"] = withInstance(state.Players["oppo"], "hand", ir.TestInstance{InstanceID: oppoHand, Alias: "oppo-hand", CardID: 12345678, DeclaredType: "spell"})
	state.Players["oppo"] = withInstance(state.Players["oppo"], "field", ir.TestInstance{InstanceID: fieldID, Alias: "visible", CardID: 23456789, DeclaredType: "follower"})
	session, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}

	own, err := session.View("own")
	if err != nil {
		t.Fatal(err)
	}
	if len(own.Own.Hand) != 1 || own.Own.Hand[0].InstanceID != ownHand || len(own.Oppo.Hand) != 0 || own.Oppo.HandCount != 1 || len(own.Oppo.Field) != 1 || own.Oppo.Field[0].Keywords[0] != "ward" {
		t.Fatalf("own view leaked or omitted state: %#v", own)
	}
	oppo, err := session.View("oppo")
	if err != nil {
		t.Fatal(err)
	}
	if len(oppo.Own.Hand) != 1 || oppo.Own.Hand[0].InstanceID != oppoHand || len(oppo.Oppo.Hand) != 0 || oppo.Turn.Active != "oppo" {
		t.Fatalf("opposing view did not flip perspective: %#v", oppo)
	}
}

func TestSimulatorLegalActionsUsePreflightAndOwnership(t *testing.T) {
	playableID, blockedID, opponentID := strings.Repeat("4", 32), strings.Repeat("5", 32), strings.Repeat("6", 32)
	pack := &ir.CardPack{Cards: []ir.Card{
		{ID: 34567890, CardType: "spell", Cost: 1},
		{ID: 45678901, CardType: "spell", Cost: 1, PlayEffects: []ir.Effect{
			ir.SelectionEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("7", 32)}, Kind: "require", Policy: "required", Binding: "target", Source: ir.ZoneRef{Kind: "zone", Side: "oppo", Zone: "field", Member: "follower"}},
		}},
	}}
	state := testState()
	own := state.Players["own"]
	own.PP, own.MaxPP = 1, 1
	own = withInstance(own, "hand", ir.TestInstance{InstanceID: playableID, Alias: "playable", CardID: 34567890, DeclaredType: "spell"})
	own = withInstance(own, "hand", ir.TestInstance{InstanceID: blockedID, Alias: "blocked", CardID: 45678901, DeclaredType: "spell"})
	state.Players["own"] = own
	state.Players["oppo"] = withInstance(state.Players["oppo"], "hand", ir.TestInstance{InstanceID: opponentID, Alias: "opponent", CardID: 34567890, DeclaredType: "spell"})
	session, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	actions := session.LegalActions()
	if len(actions) != 2 || actions[0].Kind != "play" || actions[0].Source != playableID || actions[1].Kind != "end_turn" {
		t.Fatalf("unexpected legal actions: %#v", actions)
	}
	result := session.Submit(strings.Repeat("8", 32), SimulatorCommand{Kind: "play", Source: opponentID})
	if result.Status != StatusIllegal || !contains(session.g.oppo.hand, session.g.instances[opponentID]) {
		t.Fatalf("opponent card was accepted as own source: %#v", result)
	}
	invalidAttack := session.Submit(strings.Repeat("9", 32), SimulatorCommand{Kind: "attack", Source: playableID})
	if invalidAttack.Status != StatusIllegal || invalidAttack.IllegalCode != "invalid_attacker" {
		t.Fatalf("invalid attack was not preflighted: %#v", invalidAttack)
	}
	ended := session.Submit(strings.Repeat("a", 32), SimulatorCommand{Kind: "end_turn"})
	if ended.Status != StatusCompleted || session.g.turn.Active != "oppo" {
		t.Fatalf("end turn was not exposed by simulator: result=%#v turn=%#v", ended, session.g.turn)
	}
}

func TestSimulatorCapabilitiesMatchImplementedCommands(t *testing.T) {
	capabilities := SupportedSimulatorCapabilities()
	if !capabilities.Play || !capabilities.Engage || !capabilities.SuperEvolve || !capabilities.TargetChoice || !capabilities.ModeChoice {
		t.Fatalf("implemented capability missing: %#v", capabilities)
	}
	if !capabilities.Evolve {
		t.Fatalf("implemented evolution rule missing: %#v", capabilities)
	}
}

func TestSimulatorProjectsCombatReadinessAndLegalAttacks(t *testing.T) {
	followerID := strings.Repeat("1", 32)
	pack := &ir.CardPack{Cards: []ir.Card{{ID: 78901234, CardType: "follower", Cost: 1, Stats: &ir.Stats{Attack: 2, Life: 2}}}}
	state := testState()
	own := state.Players["own"]
	own.PP, own.MaxPP = 1, 1
	own = withInstance(own, "hand", ir.TestInstance{InstanceID: followerID, Alias: "follower", CardID: 78901234, DeclaredType: "follower"})
	state.Players["own"] = own
	session, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	if result := session.Submit(strings.Repeat("2", 32), SimulatorCommand{Kind: "play", Source: followerID}); result.Status != StatusCompleted {
		t.Fatalf("play failed: %#v", result)
	}
	view, _ := session.View("own")
	if len(view.Own.Field) != 1 || !view.Own.Field[0].SummoningSick || view.Own.Field[0].AttacksUsed != 0 || hasLegalAttack(session.LegalActions(), followerID) {
		t.Fatalf("sick follower was projected as ready: view=%#v actions=%#v", view.Own.Field, session.LegalActions())
	}
	if result := session.Submit(strings.Repeat("3", 32), SimulatorCommand{Kind: "end_turn"}); result.Status != StatusCompleted {
		t.Fatalf("own turn did not end: %#v", result)
	}
	if result := session.SubmitAs(strings.Repeat("4", 32), "oppo", SimulatorCommand{Kind: "end_turn"}); result.Status != StatusCompleted {
		t.Fatalf("opponent turn did not end: %#v", result)
	}
	if !hasLegalAttack(session.LegalActions(), followerID) {
		t.Fatalf("ready follower was omitted from legal actions: %#v", session.LegalActions())
	}
	if result := session.Submit(strings.Repeat("5", 32), SimulatorCommand{Kind: "attack", Source: followerID}); result.Status != StatusCompleted {
		t.Fatalf("attack failed: %#v", result)
	}
	view, _ = session.View("own")
	if view.Own.Field[0].AttacksUsed != 1 || view.Own.Field[0].SummoningSick || !view.Own.AttackedThisTurn || hasLegalAttack(session.LegalActions(), followerID) {
		t.Fatalf("used attack state diverged: view=%#v actions=%#v", view.Own, session.LegalActions())
	}
}

func hasLegalAttack(actions []LegalAction, source string) bool {
	for _, action := range actions {
		if action.Source == source && (action.Kind == "attack_leader" || action.Kind == "attack_entity") {
			return true
		}
	}
	return false
}

func TestSimulatorViewOnlyShowsChoiceToItsController(t *testing.T) {
	selectionID, abilityID := strings.Repeat("a", 32), strings.Repeat("b", 32)
	pack := &ir.CardPack{Cards: []ir.Card{
		{ID: 56789012, CardType: "follower", Stats: &ir.Stats{Attack: 1, Life: 1}},
		{ID: 67890123, CardType: "follower", Stats: &ir.Stats{Attack: 1, Life: 1}, Abilities: []ir.Ability{{
			ID: abilityID, Trigger: ir.EventTrigger{Kind: "event", Event: "follower_summoned", Side: "oppo"},
			Body: []ir.Effect{ir.SelectionEffect{NodeBase: ir.NodeBase{ID: selectionID}, Kind: "require", Policy: "required", Binding: "target", Source: ir.ZoneRef{Kind: "zone", Side: "own", Zone: "hand", Member: "card"}}},
		}}},
	}}
	state := testState()
	oppoHand := strings.Repeat("c", 32)
	state.Players["oppo"] = withInstance(state.Players["oppo"], "field", ir.TestInstance{InstanceID: strings.Repeat("d", 32), Alias: "listener", CardID: 67890123, DeclaredType: "follower"})
	state.Players["oppo"] = withInstance(state.Players["oppo"], "hand", ir.TestInstance{InstanceID: oppoHand, Alias: "private", CardID: 56789012, DeclaredType: "follower"})
	session, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	session.actionID = strings.Repeat("e", 32)
	session.g.summon(1, 56789012)
	if result := session.run(); result.Status != StatusSuspended || result.Choice.PublicTo != "oppo" {
		t.Fatalf("opponent choice did not suspend privately: %#v", result)
	}
	ownView, _ := session.View("own")
	oppoView, _ := session.View("oppo")
	if ownView.PendingChoice != nil || oppoView.PendingChoice == nil || oppoView.PendingChoice.PublicTo != "own" || oppoView.PendingChoice.Candidates[0].InstanceID != oppoHand {
		t.Fatalf("choice visibility diverged: own=%#v oppo=%#v", ownView.PendingChoice, oppoView.PendingChoice)
	}
}
