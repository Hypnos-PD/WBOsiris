package runner

import (
	"strings"
	"testing"

	"wbo/internal/ir"
)

func TestTriggerIndexUsesStableSideZoneAndDeclarationOrder(t *testing.T) {
	leader := ir.LeaderRef{Kind: "leader", Side: "own", ValueType: "leader"}
	ownFirstID, ownSecondID, oppoID := strings.Repeat("1", 32), strings.Repeat("2", 32), strings.Repeat("3", 32)
	pack := &ir.CardPack{Cards: []ir.Card{
		{ID: 12345678, CardType: "follower", Stats: &ir.Stats{Attack: 1, Life: 1}, Abilities: []ir.Ability{
			{ID: ownFirstID, Trigger: ir.EventTrigger{Kind: "event", Event: "follower_summoned", Side: "own"}, Body: []ir.Effect{ir.AdjustEffect{Kind: "adjust_resource", Resource: "maxpp", Delta: 1}}},
			{ID: ownSecondID, Trigger: ir.EventTrigger{Kind: "event", Event: "follower_summoned", Side: "own"}, Body: []ir.Effect{ir.AdjustEffect{Kind: "adjust_resource", Resource: "maxpp", Delta: 1}}},
		}},
		{ID: 23456789, CardType: "follower", Stats: &ir.Stats{Attack: 1, Life: 1}, Abilities: []ir.Ability{
			{ID: oppoID, Trigger: ir.EventTrigger{Kind: "event", Event: "follower_summoned", Side: "oppo"}, Body: []ir.Effect{
				ir.AdjustEffect{Kind: "adjust_resource", Owner: "oppo", Resource: "maxpp", Delta: 1},
				ir.IfEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("e", 32)}, Kind: "if", Condition: ir.CompareCondition{Kind: "compare", Op: "eq", Left: ir.Scalar{Kind: "scalar", Side: "oppo", Field: "maxpp"}, Right: 3}, Then: []ir.Effect{ir.TargetEffect{Kind: "heal", Target: leader, Amount: 1}}, Else: []ir.Effect{ir.TargetEffect{Kind: "heal", Target: leader, Amount: 2}}},
			}},
		}},
		{ID: 34567890, CardType: "follower", Stats: &ir.Stats{Attack: 1, Life: 1}},
	}}
	state := testState()
	own := state.Players["own"]
	own.Leader = ir.Leader{Life: 10, MaxLife: 20}
	own = withInstance(own, "field", ir.TestInstance{InstanceID: strings.Repeat("a", 32), Alias: "own-listener", CardID: 12345678, DeclaredType: "follower"})
	state.Players["own"] = own
	oppo := state.Players["oppo"]
	oppo.Leader = ir.Leader{Life: 10, MaxLife: 20}
	oppo = withInstance(oppo, "field", ir.TestInstance{InstanceID: strings.Repeat("b", 32), Alias: "oppo-listener", CardID: 23456789, DeclaredType: "follower"})
	state.Players["oppo"] = oppo

	session, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	session.actionID = strings.Repeat("c", 32)
	session.g.summon(1, 34567890)
	if len(session.g.triggers) != 3 {
		t.Fatalf("indexed listeners queued %d triggers, want 3", len(session.g.triggers))
	}
	if step := session.run(); step.Status != StatusCompleted {
		t.Fatalf("trigger resolution failed: %#v", step)
	}
	if session.g.own.maxpp != 3 || session.g.own.leaderLife != 10 || session.g.oppo.leaderLife != 11 {
		t.Fatalf("trigger controller diverged: maxpp=%d life=%d/%d", session.g.own.maxpp, session.g.own.leaderLife, session.g.oppo.leaderLife)
	}
}

func TestOpponentTriggerHonorsExplicitEffectOwners(t *testing.T) {
	abilityID := strings.Repeat("1", 32)
	pack := &ir.CardPack{Cards: []ir.Card{
		{ID: 35678901, CardType: "follower", Stats: &ir.Stats{Attack: 1, Life: 1}},
		{ID: 36789012, CardType: "follower", Stats: &ir.Stats{Attack: 1, Life: 1}, Abilities: []ir.Ability{{
			ID: abilityID, Trigger: ir.EventTrigger{Kind: "event", Event: "follower_summoned", Side: "oppo"}, Body: []ir.Effect{
				ir.DrawEffect{Kind: "draw", Owner: "oppo", SourceZone: "deck", Output: "drawn", Count: 1},
				ir.CardEffect{Kind: "add_card", Owner: "own", Destination: "hand", CardID: 35678901, Count: 1},
			},
		}}},
	}}
	state := testState()
	deckID := strings.Repeat("2", 32)
	state.Players["own"] = withInstance(state.Players["own"], "deck", ir.TestInstance{InstanceID: deckID, Alias: "top", CardID: 35678901, DeclaredType: "follower"})
	state.Players["oppo"] = withInstance(state.Players["oppo"], "field", ir.TestInstance{InstanceID: strings.Repeat("3", 32), Alias: "listener", CardID: 36789012, DeclaredType: "follower"})
	session, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	session.actionID = strings.Repeat("4", 32)
	session.g.summon(1, 35678901)
	if result := session.run(); result.Status != StatusCompleted {
		t.Fatalf("trigger failed: %#v", result)
	}
	if len(session.g.own.hand) != 1 || session.g.own.hand[0].id != deckID || len(session.g.oppo.hand) != 1 || session.g.oppo.hand[0].card.ID != 35678901 {
		t.Fatalf("explicit owners diverged: own=%#v oppo=%#v", session.g.own.hand, session.g.oppo.hand)
	}
}

func TestTriggerIndexTracksTransformAndZoneMoves(t *testing.T) {
	abilityID := strings.Repeat("4", 32)
	listener := ir.Card{ID: 45678901, CardType: "follower", Stats: &ir.Stats{Attack: 1, Life: 1}, Abilities: []ir.Ability{
		{ID: abilityID, Trigger: ir.EventTrigger{Kind: "event", Event: "follower_summoned", Side: "own"}},
	}}
	blank := ir.Card{ID: 56789012, CardType: "follower", Stats: &ir.Stats{Attack: 1, Life: 1}}
	pack := &ir.CardPack{Cards: []ir.Card{listener, blank}}
	state := testState()
	sourceID := strings.Repeat("d", 32)
	state.Players["own"] = withInstance(state.Players["own"], "field", ir.TestInstance{InstanceID: sourceID, Alias: "source", CardID: blank.ID, DeclaredType: "follower"})
	session, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	source := session.g.instances[sourceID]
	binding := frame{"source": {source}}
	session.g.execCardEffect(ir.CardEffect{Kind: "transform", Target: ir.BindingRef{Kind: "binding", Name: "source"}, CardID: listener.ID}, nil, binding)
	session.g.triggerSummoned(source)
	if len(session.g.triggers) != 1 {
		t.Fatalf("transformed listener was not indexed: %d triggers", len(session.g.triggers))
	}
	session.g.triggers = nil
	session.g.move(source, "graveyard")
	session.g.triggerSummoned(source)
	if len(session.g.triggers) != 0 {
		t.Fatal("listener remained indexed after leaving the field")
	}
}

func TestTriggerIndexDispatchesEngageEvents(t *testing.T) {
	engageAbilityID, listenerAbilityID := strings.Repeat("1", 32), strings.Repeat("2", 32)
	leader := ir.LeaderRef{Kind: "leader", Side: "own", ValueType: "leader"}
	pack := &ir.CardPack{Cards: []ir.Card{
		{ID: 61234567, CardType: "amulet", Abilities: []ir.Ability{{ID: engageAbilityID, Trigger: ir.CostTrigger{Kind: "engage", Cost: 1}}}},
		{ID: 62345678, CardType: "follower", Stats: &ir.Stats{Attack: 1, Life: 1}, Abilities: []ir.Ability{{
			ID: listenerAbilityID, Trigger: ir.EventTrigger{Kind: "event", Event: "amulet_engaged", Side: "own", SubjectType: "amulet"},
			Body: []ir.Effect{ir.TargetEffect{Kind: "heal", Target: leader, Amount: 1}},
		}}},
	}}
	state := testState()
	own := state.Players["own"]
	own.PP = 1
	own.Leader = ir.Leader{Life: 10, MaxLife: 20}
	sourceID := strings.Repeat("3", 32)
	own = withInstance(own, "field", ir.TestInstance{InstanceID: sourceID, Alias: "source", CardID: 61234567, DeclaredType: "amulet"})
	own = withInstance(own, "field", ir.TestInstance{InstanceID: strings.Repeat("4", 32), Alias: "listener", CardID: 62345678, DeclaredType: "follower"})
	state.Players["own"] = own
	session, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	result := session.Begin(strings.Repeat("5", 32), ir.SourceAction{Kind: "engage", Actor: "own", Source: sourceID})
	if result.Status != StatusCompleted || session.g.own.leaderLife != 11 || len(session.g.events) != 2 || session.g.events[0].Kind != "amulet_engaged" {
		t.Fatalf("engage event was not dispatched: result=%#v life=%d events=%#v", result, session.g.own.leaderLife, session.g.events)
	}
}

func TestDeathBatchMovesAllEntitiesBeforeQueueingLastwords(t *testing.T) {
	firstAbilityID, secondAbilityID, opposingAbilityID := strings.Repeat("5", 32), strings.Repeat("6", 32), strings.Repeat("7", 32)
	card := func(id int, abilities ...ir.Ability) ir.Card {
		return ir.Card{ID: id, CardType: "follower", Stats: &ir.Stats{Attack: 1, Life: 1}, Abilities: abilities}
	}
	pack := &ir.CardPack{Cards: []ir.Card{
		card(67890123,
			ir.Ability{ID: firstAbilityID, Trigger: ir.SimpleTrigger{Kind: "lastwords"}},
			ir.Ability{ID: secondAbilityID, Trigger: ir.SimpleTrigger{Kind: "lastwords"}},
		),
		card(78901234, ir.Ability{ID: opposingAbilityID, Trigger: ir.SimpleTrigger{Kind: "lastwords"}}),
	}}
	state := testState()
	firstID, secondID, opposingID := strings.Repeat("8", 32), strings.Repeat("9", 32), strings.Repeat("a", 32)
	state.Players["own"] = withInstance(state.Players["own"], "field", ir.TestInstance{InstanceID: firstID, Alias: "first", CardID: 67890123, DeclaredType: "follower"})
	state.Players["own"] = withInstance(state.Players["own"], "field", ir.TestInstance{InstanceID: secondID, Alias: "second", CardID: 67890123, DeclaredType: "follower"})
	state.Players["oppo"] = withInstance(state.Players["oppo"], "field", ir.TestInstance{InstanceID: opposingID, Alias: "opposing", CardID: 78901234, DeclaredType: "follower"})
	session, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	session.g.execTargetEffect(ir.TargetEffect{Kind: "damage", Target: ir.ZoneRef{Kind: "zone", Zone: "field", Member: "follower"}, Amount: 1}, nil, frame{})

	if len(session.g.own.field) != 0 || len(session.g.oppo.field) != 0 || len(session.g.own.destroyed) != 2 || len(session.g.oppo.destroyed) != 1 {
		t.Fatalf("death batch was not moved atomically: own=%d oppo=%d histories=%d/%d", len(session.g.own.field), len(session.g.oppo.field), len(session.g.own.destroyed), len(session.g.oppo.destroyed))
	}
	if len(session.g.events) != 6 || session.g.events[3].BatchID == 0 || session.g.events[3].BatchID != session.g.events[4].BatchID || session.g.events[4].BatchID != session.g.events[5].BatchID {
		t.Fatalf("death facts do not share a batch: %#v", session.g.events)
	}
	for n, event := range session.g.events {
		if event.Sequence != uint64(n+1) {
			t.Fatalf("event %d has sequence %d", n, event.Sequence)
		}
	}
	want := []string{
		abilityBlockID(67890123, firstAbilityID), abilityBlockID(67890123, secondAbilityID),
		abilityBlockID(67890123, firstAbilityID), abilityBlockID(67890123, secondAbilityID),
		abilityBlockID(78901234, opposingAbilityID),
	}
	if len(session.g.triggers) != len(want) {
		t.Fatalf("queued %d lastwords, want %d", len(session.g.triggers), len(want))
	}
	for n, trigger := range session.g.triggers {
		if trigger.blockID != want[n] {
			t.Fatalf("lastwords %d = %q, want %q", n, trigger.blockID, want[n])
		}
	}
}
