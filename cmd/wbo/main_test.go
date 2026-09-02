package main

import (
	"strings"
	"testing"

	"wbo/internal/ir"
	"wbo/internal/runner"
)

func TestSimulationInputCannotAnswerOpponentChoice(t *testing.T) {
	modeID, abilityID := strings.Repeat("1", 32), strings.Repeat("2", 32)
	spellID, listenerID := strings.Repeat("3", 32), strings.Repeat("4", 32)
	pack := &ir.CardPack{Cards: []ir.Card{
		{ID: 12345678, CardType: "spell", PlayEffects: []ir.Effect{ir.CardEffect{Kind: "summon", Owner: "own", CardID: 23456789, Count: 1, Output: "summoned"}}},
		{ID: 23456789, CardType: "follower", Stats: &ir.Stats{Attack: 1, Life: 1}},
		{ID: 34567890, CardType: "follower", Stats: &ir.Stats{Attack: 1, Life: 1}, Abilities: []ir.Ability{{
			ID: abilityID, Trigger: ir.EventTrigger{Kind: "event", Event: "follower_summoned", Side: "oppo"},
			Body: []ir.Effect{ir.ModeEffect{NodeBase: ir.NodeBase{ID: modeID}, Kind: "mode", Options: []ir.ModeOption{{ID: 1}, {ID: 2}}}},
		}}},
	}}
	emptyZones := func() map[string][]ir.TestInstance {
		return map[string][]ir.TestInstance{"deck": {}, "hand": {}, "field": {}, "graveyard": {}, "banished": {}, "destroyed": {}}
	}
	ownZones, oppoZones := emptyZones(), emptyZones()
	ownZones["hand"] = []ir.TestInstance{{InstanceID: spellID, Alias: "spell", CardID: 12345678, DeclaredType: "spell"}}
	oppoZones["field"] = []ir.TestInstance{{InstanceID: listenerID, Alias: "listener", CardID: 34567890, DeclaredType: "follower"}}
	state := ir.State{
		Turn: ir.Turn{Active: "own", Number: 1}, Phase: "main", Aliases: map[string]string{"spell": spellID, "listener": listenerID},
		Players: map[string]ir.PlayerState{
			"own":  {Leader: ir.Leader{Life: 20, MaxLife: 20}, Zones: ownZones},
			"oppo": {Leader: ir.Leader{Life: 20, MaxLife: 20}, Zones: oppoZones},
		},
	}
	session, err := runner.NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	var ordinal uint64
	first := applySimulationInput(session, simulationInput{Kind: "play", Source: spellID}, &ordinal)
	if first.Status != runner.StatusSuspended || first.Choice == nil || first.Choice.PublicTo != "oppo" {
		t.Fatalf("opponent choice did not suspend: %#v", first)
	}
	second := applySimulationInput(session, simulationInput{Kind: "select_mode", SelectedOptionID: 1}, &ordinal)
	if second.Status != runner.StatusRejected || second.ErrorCode != "choice_not_available" || session.PendingChoice() == nil {
		t.Fatalf("opponent choice was answered by own client: %#v", second)
	}
}

func TestSimulationInputCanSubmitOpponentAction(t *testing.T) {
	instanceID := strings.Repeat("5", 32)
	pack := &ir.CardPack{Cards: []ir.Card{{ID: 12345678, CardType: "follower", Cost: 1, Stats: &ir.Stats{Attack: 1, Life: 1}}}}
	zones := func() map[string][]ir.TestInstance {
		return map[string][]ir.TestInstance{"deck": {}, "hand": {}, "field": {}, "graveyard": {}, "banished": {}, "destroyed": {}}
	}
	oppoZones := zones()
	oppoZones["hand"] = []ir.TestInstance{{InstanceID: instanceID, Alias: "oppo-card", CardID: 12345678, DeclaredType: "follower"}}
	state := ir.State{Turn: ir.Turn{Active: "own", Number: 1}, Phase: "main", Players: map[string]ir.PlayerState{
		"own":  {Leader: ir.Leader{Life: 20, MaxLife: 20}, Zones: zones()},
		"oppo": {Leader: ir.Leader{Life: 20, MaxLife: 20}, PP: 1, MaxPP: 1, Zones: oppoZones},
	}}
	session, err := runner.NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	var ordinal uint64
	if result := applySimulationInput(session, simulationInput{Kind: "end_turn"}, &ordinal); result.Status != runner.StatusCompleted {
		t.Fatalf("end turn failed: %#v", result)
	}
	result := applySimulationInput(session, simulationInput{Kind: "play", Actor: "oppo", Source: instanceID}, &ordinal)
	view, err := session.View("own")
	if result.Status != runner.StatusCompleted || err != nil || len(view.Oppo.Field) != 1 || !view.Oppo.Field[0].SummoningSick || view.Oppo.Field[0].AttacksUsed != 0 {
		t.Fatalf("opponent command was not applied through CLI input: result=%#v", result)
	}
}
