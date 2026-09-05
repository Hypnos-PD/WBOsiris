package runner

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"wbo/internal/ir"
	"wbo/internal/ruleset"
)

func TestContinuationRoundTripPreservesSharedBindings(t *testing.T) {
	sourceID, targetID, attackerID := strings.Repeat("1", 32), strings.Repeat("2", 32), strings.Repeat("6", 32)
	ifID, choiceID := strings.Repeat("3", 32), strings.Repeat("4", 32)
	pack := &ir.CardPack{Cards: []ir.Card{
		{ID: 12345678, CardType: "spell", PlayEffects: []ir.Effect{
			ir.IfEffect{NodeBase: ir.NodeBase{ID: ifID}, Kind: "if", Condition: ir.CompareCondition{Kind: "compare", Op: "eq", Left: ir.Scalar{Kind: "scalar", Side: "own", Field: "combo"}, Right: 0}, Then: []ir.Effect{
				ir.SelectionEffect{NodeBase: ir.NodeBase{ID: choiceID}, Kind: "require", Policy: "required", Binding: "target", Source: ir.ZoneRef{Kind: "zone", Side: "oppo", Zone: "field", Member: "follower"}},
			}},
			ir.TargetEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("5", 32)}, Kind: "destroy", Target: ir.BindingRef{Kind: "binding", Name: "target"}},
		}},
		{ID: 23456789, CardType: "follower", Stats: &ir.Stats{Attack: 1, Life: 1}},
	}}
	state := testState()
	state.Players["own"] = withInstance(state.Players["own"], "hand", ir.TestInstance{InstanceID: sourceID, Alias: "source", CardID: 12345678, DeclaredType: "spell"})
	state.Players["own"] = withInstance(state.Players["own"], "field", ir.TestInstance{InstanceID: attackerID, Alias: "attacker", CardID: 23456789, DeclaredType: "follower"})
	state.Players["oppo"] = withInstance(state.Players["oppo"], "field", ir.TestInstance{InstanceID: targetID, Alias: "target", CardID: 23456789, DeclaredType: "follower"})

	session, err := NewSession(pack, state, 7)
	if err != nil {
		t.Fatal(err)
	}
	step := session.Begin(strings.Repeat("a", 32), ir.SourceAction{Kind: "play", Actor: "own", Source: sourceID})
	if step.Status != StatusSuspended {
		t.Fatalf("action did not suspend: %#v", step)
	}
	session.g.instances[attackerID].attacksUsed = 1
	session.g.instances[targetID].summoningSick = true
	session.g.own.attackedThisTurn = true
	encoded, err := session.EncodeContinuation()
	if err != nil {
		t.Fatal(err)
	}
	again, err := session.EncodeContinuation()
	if err != nil || !bytes.Equal(encoded, again) {
		t.Fatal("continuation encoding is not deterministic")
	}
	decoded, err := DecodeContinuation(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Budget.Instructions != 2 || decoded.Budget.Candidates != 1 || decoded.Budget.MaxStackDepth != 2 {
		t.Fatalf("continuation lost budget state: %#v", decoded.Budget)
	}
	session = nil
	restored, err := RestoreSession(pack, decoded)
	if err != nil {
		t.Fatal(err)
	}
	if restored.g.instances[attackerID].attacksUsed != 1 || !restored.g.instances[targetID].summoningSick || !restored.g.own.attackedThisTurn {
		t.Fatalf("continuation lost combat state: attacker=%#v target=%#v player=%#v", restored.g.instances[attackerID], restored.g.instances[targetID], restored.g.own)
	}
	result := restored.Resume(ChoiceResponse{RequestID: step.Choice.RequestID, ActionID: step.Choice.ActionID, StateRevision: step.Choice.StateRevision, SelectedInstanceIDs: []string{targetID}})
	if result.Status != StatusCompleted || restored.g.instances[targetID].zone != "graveyard" {
		t.Fatalf("restored shared binding was not visible to the parent frame: %#v", result)
	}
}

func TestContinuationRoundTripPreservesRNGAndTriggerQueue(t *testing.T) {
	sourceID, listenerID, secondListenerID := strings.Repeat("6", 32), strings.Repeat("7", 32), strings.Repeat("8", 32)
	listenerAbilityID := strings.Repeat("9", 32)
	leader := ir.LeaderRef{Kind: "leader", Side: "own", ValueType: "leader"}
	pack := &ir.CardPack{Cards: []ir.Card{
		{ID: 34567890, CardType: "spell", PlayEffects: []ir.Effect{
			ir.SelectionEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("b", 32)}, Kind: "random_choose", Policy: "random", Binding: "random", Source: ir.ZoneRef{Kind: "zone", Side: "own", Zone: "field", Member: "follower"}},
			ir.CardEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("a", 32)}, Kind: "summon", Owner: "own", CardID: 45678901, Count: 1, Output: "summoned"},
		}},
		{ID: 45678901, CardType: "follower", Stats: &ir.Stats{Attack: 1, Life: 1}},
		{ID: 56789012, CardType: "follower", Stats: &ir.Stats{Attack: 1, Life: 1}, Abilities: []ir.Ability{
			{ID: listenerAbilityID, Trigger: ir.EventTrigger{Kind: "event", Event: "follower_summoned", Side: "own"}, Body: []ir.Effect{
				ir.ModeEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("c", 32)}, Kind: "mode", Options: []ir.ModeOption{{ID: 1, Body: []ir.Effect{
					ir.TargetEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("d", 32)}, Kind: "heal", Target: leader, Amount: 1},
				}}}},
			}},
		}},
	}}
	state := testState()
	own := state.Players["own"]
	own.Leader = ir.Leader{Life: 10, MaxLife: 20}
	own = withInstance(own, "hand", ir.TestInstance{InstanceID: sourceID, Alias: "source", CardID: 34567890, DeclaredType: "spell"})
	own = withInstance(own, "field", ir.TestInstance{InstanceID: listenerID, Alias: "listener", CardID: 56789012, DeclaredType: "follower"})
	own = withInstance(own, "field", ir.TestInstance{InstanceID: secondListenerID, Alias: "listener2", CardID: 56789012, DeclaredType: "follower"})
	state.Players["own"] = own

	session, err := NewSession(pack, state, 0xfedcba9876543210)
	if err != nil {
		t.Fatal(err)
	}
	step := session.Begin(strings.Repeat("e", 32), ir.SourceAction{Kind: "play", Actor: "own", Source: sourceID})
	if step.Status != StatusSuspended || session.g.rng.Consumed() != 1 || len(session.g.triggers) != 1 || !session.drainingTrigger {
		t.Fatalf("unexpected suspended state: step=%#v rng=%d triggers=%d", step, session.g.rng.Consumed(), len(session.g.triggers))
	}
	encoded, err := session.EncodeContinuation()
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeContinuation(encoded)
	if err != nil {
		t.Fatal(err)
	}
	t.Run("version", func(t *testing.T) {
		copy := *decoded
		copy.Version = "0.5.0"
		if _, err := RestoreSession(pack, &copy); err == nil {
			t.Fatal("old continuation version was accepted")
		}
	})
	restored, err := RestoreSession(pack, decoded)
	if err != nil {
		t.Fatal(err)
	}
	result := restored.Resume(ChoiceResponse{RequestID: step.Choice.RequestID, ActionID: step.Choice.ActionID, StateRevision: step.Choice.StateRevision, SelectedOptionID: 1})
	if result.Status != StatusSuspended {
		t.Fatalf("second queued trigger did not suspend: %#v", result)
	}
	secondEncoded, err := restored.EncodeContinuation()
	if err != nil {
		t.Fatal(err)
	}
	secondDecoded, err := DecodeContinuation(secondEncoded)
	if err != nil {
		t.Fatal(err)
	}
	restored, err = RestoreSession(pack, secondDecoded)
	if err != nil {
		t.Fatal(err)
	}
	result = restored.Resume(ChoiceResponse{RequestID: result.Choice.RequestID, ActionID: result.Choice.ActionID, StateRevision: result.Choice.StateRevision, SelectedOptionID: 1})
	if result.Status != StatusCompleted || restored.g.rng.Consumed() != 1 || restored.g.own.leaderLife != 12 || restored.g.instances["summoned-1"] == nil {
		t.Fatalf("restored queue or RNG diverged: result=%#v rng=%d life=%d", result, restored.g.rng.Consumed(), restored.g.own.leaderLife)
	}
}

func TestContinuationRestoreRebuildsTriggerIndex(t *testing.T) {
	sourceID, listenerID := strings.Repeat("a", 32), strings.Repeat("b", 32)
	modeID, abilityID := strings.Repeat("c", 32), strings.Repeat("d", 32)
	leader := ir.LeaderRef{Kind: "leader", Side: "own", ValueType: "leader"}
	pack := &ir.CardPack{Cards: []ir.Card{
		{ID: 89012345, CardType: "spell", PlayEffects: []ir.Effect{
			ir.ModeEffect{NodeBase: ir.NodeBase{ID: modeID}, Kind: "mode", Options: []ir.ModeOption{{ID: 1, Body: []ir.Effect{
				ir.CardEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("e", 32)}, Kind: "summon", Owner: "own", CardID: 90123456, Count: 1, Output: "summoned"},
			}}}},
		}},
		{ID: 90123456, CardType: "follower", Stats: &ir.Stats{Attack: 1, Life: 1}},
		{ID: 91234567, CardType: "follower", Stats: &ir.Stats{Attack: 1, Life: 1}, Abilities: []ir.Ability{
			{ID: abilityID, Trigger: ir.EventTrigger{Kind: "event", Event: "follower_summoned", Side: "own"}, Body: []ir.Effect{ir.TargetEffect{Kind: "heal", Target: leader, Amount: 1}}},
		}},
	}}
	state := testState()
	own := state.Players["own"]
	own.Leader = ir.Leader{Life: 10, MaxLife: 20}
	own = withInstance(own, "hand", ir.TestInstance{InstanceID: sourceID, Alias: "source", CardID: 89012345, DeclaredType: "spell"})
	own = withInstance(own, "field", ir.TestInstance{InstanceID: listenerID, Alias: "listener", CardID: 91234567, DeclaredType: "follower"})
	state.Players["own"] = own

	session, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	step := session.Begin(strings.Repeat("f", 32), ir.SourceAction{Kind: "play", Actor: "own", Source: sourceID})
	if step.Status != StatusSuspended {
		t.Fatalf("action did not suspend: %#v", step)
	}
	data, err := session.EncodeContinuation()
	if err != nil {
		t.Fatal(err)
	}
	continuation, err := DecodeContinuation(data)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := RestoreSession(pack, continuation)
	if err != nil {
		t.Fatal(err)
	}
	result := restored.Resume(ChoiceResponse{RequestID: step.Choice.RequestID, ActionID: step.Choice.ActionID, StateRevision: step.Choice.StateRevision, SelectedOptionID: 1})
	if result.Status != StatusCompleted || restored.g.own.leaderLife != 11 {
		t.Fatalf("restored trigger index missed summon: result=%#v life=%d", result, restored.g.own.leaderLife)
	}
}

func TestContinuationRoundTripPreservesDeathBatch(t *testing.T) {
	spellID, victimID := strings.Repeat("1", 32), strings.Repeat("2", 32)
	lastwordsID, modeID := strings.Repeat("3", 32), strings.Repeat("4", 32)
	pack := &ir.CardPack{Cards: []ir.Card{
		{ID: 93456789, CardType: "spell", PlayEffects: []ir.Effect{
			ir.TargetEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("5", 32)}, Kind: "destroy", Target: ir.ZoneRef{Kind: "zone", Side: "oppo", Zone: "field", Member: "follower"}},
		}},
		{ID: 94567890, CardType: "follower", Stats: &ir.Stats{Attack: 1, Life: 1}, Abilities: []ir.Ability{{
			ID: lastwordsID, Trigger: ir.SimpleTrigger{Kind: "lastwords"}, Body: []ir.Effect{
				ir.ModeEffect{NodeBase: ir.NodeBase{ID: modeID}, Kind: "mode", Options: []ir.ModeOption{{ID: 1}}},
			},
		}}},
	}}
	state := testState()
	state.Players["own"] = withInstance(state.Players["own"], "hand", ir.TestInstance{InstanceID: spellID, Alias: "spell", CardID: 93456789, DeclaredType: "spell"})
	state.Players["oppo"] = withInstance(state.Players["oppo"], "field", ir.TestInstance{InstanceID: victimID, Alias: "victim", CardID: 94567890, DeclaredType: "follower"})
	session, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	step := session.Begin(strings.Repeat("6", 32), ir.SourceAction{Kind: "play", Actor: "own", Source: spellID})
	if step.Status != StatusSuspended || session.g.instances[victimID].zone != "graveyard" || len(session.g.oppo.destroyed) != 1 {
		t.Fatalf("lastwords did not suspend after death batch: %#v", step)
	}
	data, err := session.EncodeContinuation()
	if err != nil {
		t.Fatal(err)
	}
	continuation, err := DecodeContinuation(data)
	if err != nil {
		t.Fatal(err)
	}
	if continuation.Game.EventSequence != 1 || continuation.Game.DeathBatchSerial != 1 || len(continuation.Game.Events) != 1 || continuation.Game.Events[0].BatchID != 1 {
		t.Fatalf("continuation lost death batch state: %#v", continuation.Game)
	}
	restored, err := RestoreSession(pack, continuation)
	if err != nil {
		t.Fatal(err)
	}
	result := restored.Resume(ChoiceResponse{RequestID: step.Choice.RequestID, ActionID: step.Choice.ActionID, StateRevision: step.Choice.StateRevision, SelectedOptionID: 1})
	if result.Status != StatusCompleted || restored.g.eventSequence != 1 || restored.g.deathBatchSerial != 1 || len(restored.g.oppo.destroyed) != 1 {
		t.Fatalf("restored death batch diverged: result=%#v events=%d batches=%d", result, restored.g.eventSequence, restored.g.deathBatchSerial)
	}
}

func TestContinuationRoundTripPreservesTurnTransition(t *testing.T) {
	abilityID, modeID, listenerID := strings.Repeat("7", 32), strings.Repeat("8", 32), strings.Repeat("9", 32)
	pack := &ir.CardPack{Cards: []ir.Card{{
		ID: 95678901, CardType: "follower", Stats: &ir.Stats{Attack: 1, Life: 1}, Abilities: []ir.Ability{{
			ID: abilityID, Trigger: ir.EventTrigger{Kind: "event", Event: "turn_ended", Side: "own"}, Body: []ir.Effect{
				ir.ModeEffect{NodeBase: ir.NodeBase{ID: modeID}, Kind: "mode", Options: []ir.ModeOption{{ID: 1}, {ID: 2}}},
			}},
		}},
	}}
	state := testState()
	state.Players["own"] = withInstance(state.Players["own"], "field", ir.TestInstance{InstanceID: listenerID, Alias: "listener", CardID: 95678901, DeclaredType: "follower"})
	session, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	step := session.Begin(strings.Repeat("a", 32), ir.SourceAction{Kind: "end_turn", Actor: "own"})
	if step.Status != StatusSuspended || session.g.turnTransition != "ending" {
		t.Fatalf("turn transition did not suspend: %#v", step)
	}
	data, err := session.EncodeContinuation()
	if err != nil {
		t.Fatal(err)
	}
	continuation, err := DecodeContinuation(data)
	if err != nil || continuation.Game.TurnTransition != "ending" {
		t.Fatalf("turn transition was not encoded: err=%v game=%#v", err, continuation.Game)
	}
	restored, err := RestoreSession(pack, continuation)
	if err != nil {
		t.Fatal(err)
	}
	result := restored.Resume(ChoiceResponse{RequestID: step.Choice.RequestID, ActionID: step.Choice.ActionID, StateRevision: step.Choice.StateRevision, SelectedOptionID: 1})
	if result.Status != StatusCompleted || restored.g.turn.Active != "oppo" || restored.g.turnTransition != "" {
		t.Fatalf("restored turn transition diverged: result=%#v turn=%#v transition=%q", result, restored.g.turn, restored.g.turnTransition)
	}
}

func TestContinuationStrictDecodeAndRestoreRejections(t *testing.T) {
	pack, state, sourceID, _ := continuationTargetFixture()
	session, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	if step := session.Begin(strings.Repeat("f", 32), ir.SourceAction{Kind: "play", Actor: "own", Source: sourceID}); step.Status != StatusSuspended {
		t.Fatalf("action did not suspend: %#v", step)
	}
	encoded, err := session.EncodeContinuation()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeContinuation([]byte(`{"version":"0.1.0","version":"0.1.0"}`)); err == nil {
		t.Fatal("duplicate continuation key was accepted")
	}
	var object map[string]any
	if err := json.Unmarshal(encoded, &object); err != nil {
		t.Fatal(err)
	}
	object["unknown"] = true
	unknown, _ := json.Marshal(object)
	if _, err := DecodeContinuation(unknown); err == nil {
		t.Fatal("unknown continuation field was accepted")
	}

	decoded, err := DecodeContinuation(encoded)
	if err != nil {
		t.Fatal(err)
	}
	t.Run("card pack", func(t *testing.T) {
		changed := *pack
		changed.Cards = append([]ir.Card(nil), pack.Cards...)
		changed.Cards[0].Cost++
		if _, err := RestoreSession(&changed, decoded); err == nil {
			t.Fatal("mismatched card pack was accepted")
		}
	})
	t.Run("ruleset", func(t *testing.T) {
		copy := *decoded
		copy.RulesetHash = "sha256:" + strings.Repeat("0", 64)
		if _, err := RestoreSession(pack, &copy); err == nil {
			t.Fatal("mismatched ruleset was accepted")
		}
	})
	t.Run("revision", func(t *testing.T) {
		copy := *decoded
		copy.StateRevision++
		if _, err := RestoreSession(pack, &copy); err == nil {
			t.Fatal("mismatched revision was accepted")
		}
	})
	t.Run("block", func(t *testing.T) {
		copy := *decoded
		copy.Stack = append([]ContinuationFrame(nil), decoded.Stack...)
		copy.Stack[0].BlockID = "missing"
		if _, err := RestoreSession(pack, &copy); err == nil {
			t.Fatal("unknown block was accepted")
		}
	})
	t.Run("budget", func(t *testing.T) {
		copy := *decoded
		copy.Budget.Instructions = ruleset.Default().ExecutionBudget.Instructions + 1
		if _, err := RestoreSession(pack, &copy); err == nil {
			t.Fatal("invalid budget state was accepted")
		}
	})
	t.Run("event sequence", func(t *testing.T) {
		copy := *decoded
		copy.Game.EventSequence++
		if _, err := RestoreSession(pack, &copy); err == nil {
			t.Fatal("invalid event sequence was accepted")
		}
	})
	t.Run("opponent turn", func(t *testing.T) {
		copy := *decoded
		copy.Game.Turn.Active = "oppo"
		if _, err := RestoreSession(pack, &copy); err != nil {
			t.Fatalf("opponent-turn continuation was rejected: %v", err)
		}
	})
	t.Run("choice controller", func(t *testing.T) {
		copy := *decoded
		copy.Pending.Request.PublicTo = "oppo"
		if _, err := RestoreSession(pack, &copy); err == nil {
			t.Fatal("choice with the wrong controller was accepted")
		}
	})
}

func TestRestoreGameRejectsGeneratedSerialAndHistoryOwner(t *testing.T) {
	card := &ir.Card{ID: 12345678, CardType: "follower", Stats: &ir.Stats{Attack: 1, Life: 1}}
	cards := map[int]*ir.Card{card.ID: card}
	base := ContinuationGame{Turn: ir.Turn{Active: "own", Number: 1}, Phase: "main", Legal: true}

	t.Run("generated serial", func(t *testing.T) {
		saved := base
		saved.Serial = 1
		saved.Instances = []ContinuationEntity{{ID: "added-2", Zone: "hand", CardID: card.ID}}
		saved.Own.Hand = []string{"added-2"}
		if _, err := restoreGame(cards, saved); err == nil {
			t.Fatal("stale generated instance serial was accepted")
		}
	})

	t.Run("history owner", func(t *testing.T) {
		saved := base
		id := strings.Repeat("f", 32)
		saved.Instances = []ContinuationEntity{{ID: id, Zone: "graveyard", CardID: card.ID}}
		saved.Oppo.Graveyard = []string{id}
		saved.Own.Destroyed = []string{id}
		if _, err := restoreGame(cards, saved); err == nil {
			t.Fatal("cross-owner destroyed history was accepted")
		}
	})

	t.Run("negative attacks", func(t *testing.T) {
		saved := base
		id := strings.Repeat("e", 32)
		saved.Instances = []ContinuationEntity{{ID: id, Zone: "field", CardID: card.ID, AttacksUsed: -1}}
		saved.Own.Field = []string{id}
		if _, err := restoreGame(cards, saved); err == nil {
			t.Fatal("negative attack count was accepted")
		}
	})

	t.Run("excessive attacks", func(t *testing.T) {
		saved := base
		id := strings.Repeat("c", 32)
		saved.Instances = []ContinuationEntity{{ID: id, Zone: "field", CardID: card.ID, AttacksUsed: 2}}
		saved.Own.Field = []string{id}
		if _, err := restoreGame(cards, saved); err == nil {
			t.Fatal("unsupported attack count was accepted")
		}
	})

	t.Run("sick non-field entity", func(t *testing.T) {
		saved := base
		id := strings.Repeat("d", 32)
		saved.Instances = []ContinuationEntity{{ID: id, Zone: "hand", CardID: card.ID, SummoningSick: true}}
		saved.Own.Hand = []string{id}
		if _, err := restoreGame(cards, saved); err == nil {
			t.Fatal("non-field summoning sickness was accepted")
		}
	})
}

func TestContinuationDeathBatchesMustBeContiguous(t *testing.T) {
	id := strings.Repeat("a", 32)
	instances := map[string]*instance{id: {id: id}}
	destroyed := map[string]bool{id: true}
	subject := func() *ir.EventTarget { return &ir.EventTarget{Kind: "instance", InstanceID: id} }
	events := []ir.RuntimeEvent{
		{Kind: "destroyed", Subject: subject(), Sequence: 1, BatchID: 1},
		{Kind: "healed", Sequence: 2},
		{Kind: "destroyed", Subject: subject(), Sequence: 3, BatchID: 1},
	}
	if err := validateContinuationEvents(events, 3, 1, instances, destroyed); err == nil {
		t.Fatal("split death batch was accepted")
	}
	liveID := strings.Repeat("b", 32)
	instances[liveID] = &instance{id: liveID}
	badSubject := []ir.RuntimeEvent{{Kind: "destroyed", Subject: &ir.EventTarget{Kind: "instance", InstanceID: liveID}, Sequence: 1, BatchID: 1}}
	if err := validateContinuationEvents(badSubject, 1, 1, instances, destroyed); err == nil {
		t.Fatal("live entity was accepted as a destroyed subject")
	}
}

func TestContinuationAllowsAdjacentDeathBatches(t *testing.T) {
	firstID, secondID := strings.Repeat("c", 32), strings.Repeat("d", 32)
	instances := map[string]*instance{firstID: {id: firstID}, secondID: {id: secondID}}
	destroyed := map[string]bool{firstID: true, secondID: true}
	events := []ir.RuntimeEvent{
		{Kind: "destroyed", Subject: &ir.EventTarget{Kind: "instance", InstanceID: firstID}, Sequence: 1, BatchID: 1},
		{Kind: "destroyed", Subject: &ir.EventTarget{Kind: "instance", InstanceID: secondID}, Sequence: 2, BatchID: 2},
	}
	if err := validateContinuationEvents(events, 2, 2, instances, destroyed); err != nil {
		t.Fatalf("adjacent death batches were rejected: %v", err)
	}
}

func continuationTargetFixture() (*ir.CardPack, ir.State, string, string) {
	sourceID, targetID := strings.Repeat("1", 32), strings.Repeat("2", 32)
	pack := &ir.CardPack{Cards: []ir.Card{
		{ID: 67890123, CardType: "spell", PlayEffects: []ir.Effect{
			ir.SelectionEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("3", 32)}, Kind: "require", Policy: "required", Binding: "target", Source: ir.ZoneRef{Kind: "zone", Side: "oppo", Zone: "field", Member: "follower"}},
		}},
		{ID: 78901234, CardType: "follower", Stats: &ir.Stats{Attack: 1, Life: 1}},
	}}
	state := testState()
	state.Players["own"] = withInstance(state.Players["own"], "hand", ir.TestInstance{InstanceID: sourceID, Alias: "source", CardID: 67890123, DeclaredType: "spell"})
	state.Players["oppo"] = withInstance(state.Players["oppo"], "field", ir.TestInstance{InstanceID: targetID, Alias: "target", CardID: 78901234, DeclaredType: "follower"})
	return pack, state, sourceID, targetID
}
