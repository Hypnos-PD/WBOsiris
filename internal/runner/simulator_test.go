package runner

import (
	"fmt"
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

func TestExtraPPEarlyAndLateWindows(t *testing.T) {
	cardID, cardInstance := 34567891, strings.Repeat("f", 32)
	pack := &ir.CardPack{Cards: []ir.Card{{ID: cardID, CardType: "spell", Cost: 1}}}
	makeState := func(turn int) ir.State {
		state := testState()
		state.FirstPlayer = "own"
		state.Turn = ir.Turn{Active: "oppo", Number: turn}
		oppo := state.Players["oppo"]
		oppo.ExtraPPEarly, oppo.ExtraPPLate = true, true
		oppo = withInstance(oppo, "hand", ir.TestInstance{InstanceID: cardInstance, Alias: "extra-pp-card", CardID: cardID, DeclaredType: "spell"})
		state.Players["oppo"] = oppo
		return state
	}

	session, err := NewSession(pack, makeState(3), 1)
	if err != nil {
		t.Fatal(err)
	}
	if result := session.SubmitAs(strings.Repeat("1", 32), "oppo", SimulatorCommand{Kind: "use_extra_pp"}); result.Status != StatusCompleted || session.g.oppo.pp != 1 || !session.g.oppo.extraPPActive {
		t.Fatalf("early extra PP did not activate: result=%#v player=%#v", result, session.g.oppo)
	}
	if result := session.SubmitAs(strings.Repeat("2", 32), "oppo", SimulatorCommand{Kind: "end_turn"}); result.Status != StatusCompleted || session.g.oppo.pp != 0 || !session.g.oppo.extraPPEarly {
		t.Fatalf("unused extra PP was not restored: result=%#v player=%#v", result, session.g.oppo)
	}

	session, _ = NewSession(pack, makeState(3), 1)
	session.SubmitAs(strings.Repeat("3", 32), "oppo", SimulatorCommand{Kind: "use_extra_pp"})
	if result := session.SubmitAs(strings.Repeat("4", 32), "oppo", SimulatorCommand{Kind: "play", Source: cardInstance}); result.Status != StatusCompleted || session.g.oppo.extraPPEarly || session.g.oppo.extraPPActive {
		t.Fatalf("spent early extra PP was not consumed: result=%#v player=%#v", result, session.g.oppo)
	}

	late := makeState(6)
	oppo := late.Players["oppo"]
	oppo.ExtraPPEarly = false
	late.Players["oppo"] = oppo
	session, _ = NewSession(pack, late, 1)
	if result := session.SubmitAs(strings.Repeat("5", 32), "oppo", SimulatorCommand{Kind: "use_extra_pp"}); result.Status != StatusCompleted || !session.g.oppo.extraPPActive {
		t.Fatalf("late extra PP was unavailable: result=%#v", result)
	}
}

func TestEvolutionTurnWindowsAndSharedLimit(t *testing.T) {
	cardID := 34567892
	pack := &ir.CardPack{Cards: []ir.Card{{ID: cardID, CardType: "follower", Stats: &ir.Stats{Attack: 1, Life: 1}, ActionPlans: []ir.ActionPlan{{Action: "evolve"}, {Action: "superevolve"}}}}}
	makeState := func(active string, turn int) ir.State {
		state := testState()
		state.FirstPlayer = "own"
		state.Turn = ir.Turn{Active: active, Number: turn}
		player := state.Players[active]
		player.EP, player.SEP = 2, 2
		player = withInstance(player, "field", ir.TestInstance{InstanceID: strings.Repeat("6", 32), Alias: "first", CardID: cardID, DeclaredType: "follower"})
		player = withInstance(player, "field", ir.TestInstance{InstanceID: strings.Repeat("7", 32), Alias: "second", CardID: cardID, DeclaredType: "follower"})
		state.Players[active] = player
		return state
	}

	session, _ := NewSession(pack, makeState("own", 4), 1)
	if result := session.Submit(strings.Repeat("8", 32), SimulatorCommand{Kind: "evolve", Source: strings.Repeat("6", 32)}); result.Status != StatusIllegal {
		t.Fatalf("first player evolved before turn 5: %#v", result)
	}
	session, _ = NewSession(pack, makeState("oppo", 4), 1)
	if result := session.SubmitAs(strings.Repeat("9", 32), "oppo", SimulatorCommand{Kind: "evolve", Source: strings.Repeat("6", 32)}); result.Status != StatusCompleted {
		t.Fatalf("second player could not evolve on turn 4: %#v", result)
	}
	if result := session.SubmitAs(strings.Repeat("a", 32), "oppo", SimulatorCommand{Kind: "superevolve", Source: strings.Repeat("7", 32)}); result.Status != StatusIllegal {
		t.Fatalf("player evolved twice in one turn: %#v", result)
	}
	session, _ = NewSession(pack, makeState("oppo", 6), 1)
	if result := session.SubmitAs(strings.Repeat("b", 32), "oppo", SimulatorCommand{Kind: "superevolve", Source: strings.Repeat("6", 32)}); result.Status != StatusCompleted {
		t.Fatalf("second player could not super-evolve on turn 6: %#v", result)
	}
}

func TestHandLimitAndDeckOut(t *testing.T) {
	cardID := 34567893
	pack := &ir.CardPack{Cards: []ir.Card{{ID: cardID, CardType: "spell"}}}
	state := testState()
	own := state.Players["own"]
	for n := 0; n < 8; n++ {
		own = withInstance(own, "hand", ir.TestInstance{InstanceID: fmt.Sprintf("%032x", n+1), Alias: fmt.Sprintf("hand-%d", n), CardID: cardID, DeclaredType: "spell"})
	}
	for n := 0; n < 2; n++ {
		own = withInstance(own, "deck", ir.TestInstance{InstanceID: fmt.Sprintf("%032x", n+20), Alias: fmt.Sprintf("deck-%d", n), CardID: cardID, DeclaredType: "spell"})
	}
	state.Players["own"] = own
	session, _ := NewSession(pack, state, 1)
	session.g.draw(ir.DrawEffect{Kind: "draw", Owner: "own", Count: 2}, nil, frame{})
	if len(session.g.own.hand) != handLimit || len(session.g.own.graveyard) != 1 {
		t.Fatalf("overdraw did not enter graveyard: hand=%d graveyard=%d", len(session.g.own.hand), len(session.g.own.graveyard))
	}

	empty := testState()
	empty.FirstPlayer = "own"
	session, _ = NewSession(pack, empty, 1)
	session.g.draw(ir.DrawEffect{Kind: "draw", Owner: "own", Count: 1}, nil, frame{})
	if !session.g.gameOver || session.g.winner != "oppo" {
		t.Fatalf("empty deck draw did not end game: gameOver=%t winner=%q", session.g.gameOver, session.g.winner)
	}
}

func TestStartMatchDrawsForFirstPlayer(t *testing.T) {
	cardID := 34567894
	pack := &ir.CardPack{Cards: []ir.Card{{ID: cardID, CardType: "spell"}}}
	state := testState()
	state.FirstPlayer = "own"
	state.Players["own"] = withInstance(state.Players["own"], "deck", ir.TestInstance{InstanceID: strings.Repeat("1", 32), CardID: cardID, DeclaredType: "spell"})
	session, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	session.StartMatch()
	if len(session.g.own.hand) != 1 || len(session.g.own.deck) != 0 || len(session.g.oppo.hand) != 0 {
		t.Fatalf("initial turn draw diverged: own=%#v oppo=%#v", session.g.own, session.g.oppo)
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
