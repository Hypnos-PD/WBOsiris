package runner

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"wbo/internal/ir"
	"wbo/internal/project"
	"wbo/internal/ruleset"
)

func TestBaseRulesScenarios(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	path := filepath.Join(root, "tests", "10000", "base-rules.wbotest")
	loaded := project.LoadWithRoot([]string{path}, false, root)
	if loaded.HasErrors() {
		for _, d := range loaded.Diagnostics {
			t.Log(d.String())
		}
		t.Fatal("fixture did not load")
	}
	cards, tests, err := project.BuildRuntimePacks(loaded)
	if err != nil {
		t.Fatal(err)
	}
	results := Run(cards, tests)
	if len(results) != 20 {
		t.Fatalf("got %d scenarios, want 20", len(results))
	}
	for _, result := range results {
		if !result.Passed() {
			t.Errorf("%s: %v", result.Scenario, result.Failures)
		}
	}
}

func TestRunRejectsTamperedRulesetDependency(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	loaded := project.LoadWithRoot([]string{filepath.Join(root, "tests", "10000", "base-rules.wbotest")}, false, root)
	cards, tests, err := project.BuildRuntimePacks(loaded)
	if err != nil {
		t.Fatal(err)
	}
	tests.Ruleset.OrderingPolicy.Collections = "tampered"
	results := RunWithRuleset(cards, tests, ruleset.DefaultID)
	if len(results) != 1 || results[0].Passed() {
		t.Fatalf("tampered ruleset executed: %#v", results)
	}
	_, tests, err = project.BuildRuntimePacks(loaded)
	if err != nil {
		t.Fatal(err)
	}
	tests.Ruleset.ExecutionBudget.Instructions--
	results = RunWithRuleset(cards, tests, ruleset.DefaultID)
	if len(results) != 1 || results[0].Passed() {
		t.Fatalf("tampered execution budget ran: %#v", results)
	}
}

func TestRandomTargetConsumesRNGOnlyWithCandidates(t *testing.T) {
	g := &game{rng: ruleset.NewRNG(1)}
	effect := ir.SelectionEffect{Kind: "random_choose", Binding: "target", Source: ir.ZoneRef{Kind: "zone", Side: "oppo", Zone: "field", Member: "follower"}}
	session := &Session{g: g, actionID: strings.Repeat("a", 32), stack: []execFrame{{body: []ir.Effect{effect}, bindings: frame{}}}}
	if step := session.run(); step.Status != StatusCompleted {
		t.Fatalf("empty random selection did not complete: %#v", step)
	}
	if g.rng.Consumed() != 0 {
		t.Fatalf("empty candidate set consumed %d RNG values", g.rng.Consumed())
	}
	g.oppo.field = []*instance{{card: &ir.Card{CardType: "follower"}}}
	session = &Session{g: g, actionID: strings.Repeat("b", 32), stack: []execFrame{{body: []ir.Effect{effect}, bindings: frame{}}}}
	if step := session.run(); step.Status != StatusCompleted {
		t.Fatalf("non-empty random selection did not complete: %#v", step)
	}
	if g.rng.Consumed() != 1 {
		t.Fatalf("non-empty candidate set consumed %d RNG values, want 1", g.rng.Consumed())
	}
}

func TestSessionPreflightAndTargetContinuation(t *testing.T) {
	sourceID, targetID := strings.Repeat("1", 32), strings.Repeat("2", 32)
	nodeID := strings.Repeat("3", 32)
	pack := &ir.CardPack{Cards: []ir.Card{
		{ID: 12345678, CardType: "spell", PlayEffects: []ir.Effect{
			ir.SelectionEffect{NodeBase: ir.NodeBase{ID: nodeID}, Kind: "require", Policy: "required", Binding: "target", Source: ir.ZoneRef{Kind: "zone", Side: "oppo", Zone: "field", Member: "follower"}},
			ir.TargetEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("4", 32)}, Kind: "destroy", Target: ir.BindingRef{Kind: "binding", Name: "target"}},
		}},
		{ID: 23456789, CardType: "follower", Stats: &ir.Stats{Attack: 1, Life: 1}},
	}}
	state := testState()
	state.Players["own"] = withInstance(state.Players["own"], "hand", ir.TestInstance{InstanceID: sourceID, Alias: "source", CardID: 12345678, DeclaredType: "spell"})
	state.Players["oppo"] = withInstance(state.Players["oppo"], "field", ir.TestInstance{InstanceID: targetID, Alias: "target", CardID: 23456789, DeclaredType: "follower"})
	session, err := NewSession(pack, state, 7)
	if err != nil {
		t.Fatal(err)
	}
	step := session.Begin(strings.Repeat("a", 32), ir.SourceAction{Kind: "play", Actor: "own", Source: sourceID})
	if step.Status != StatusSuspended || step.Choice.Kind != "target" || len(step.Choice.Candidates) != 1 || step.Choice.Candidates[0].InstanceID != targetID {
		t.Fatalf("unexpected target request: %#v", step)
	}
	if session.g.instances[sourceID].zone != "graveyard" || session.g.instances[targetID].zone != "field" {
		t.Fatal("resolution did not pause at the target request")
	}
	continuation := session.Continuation()
	if continuation == nil || continuation.RequestID != step.Choice.RequestID || len(continuation.Stack) == 0 {
		t.Fatalf("missing continuation: %#v", continuation)
	}
	if _, err := json.Marshal(continuation); err != nil {
		t.Fatalf("continuation is not serializable: %v", err)
	}

	before := session.g.snapshot()
	stale := session.Resume(ChoiceResponse{RequestID: step.Choice.RequestID, ActionID: step.Choice.ActionID, StateRevision: step.Choice.StateRevision + 1, SelectedInstanceIDs: []string{targetID}})
	if stale.Status != StatusRejected || stale.ErrorCode != "stale_or_mismatched_response" || !reflect.DeepEqual(before, session.g.snapshot()) {
		t.Fatalf("stale response mutated state: %#v", stale)
	}
	bad := session.Resume(ChoiceResponse{RequestID: step.Choice.RequestID, ActionID: step.Choice.ActionID, StateRevision: step.Choice.StateRevision, SelectedInstanceIDs: []string{sourceID}})
	if bad.Status != StatusRejected || bad.ErrorCode != "invalid_candidate" || !reflect.DeepEqual(before, session.g.snapshot()) {
		t.Fatalf("invalid response mutated state: %#v", bad)
	}
	good := session.Resume(ChoiceResponse{RequestID: step.Choice.RequestID, ActionID: step.Choice.ActionID, StateRevision: step.Choice.StateRevision, SelectedInstanceIDs: []string{targetID}})
	if good.Status != StatusCompleted || session.g.instances[targetID].zone != "graveyard" {
		t.Fatalf("continuation did not complete: %#v", good)
	}

	state = testState()
	state.Players["own"] = withInstance(state.Players["own"], "hand", ir.TestInstance{InstanceID: sourceID, Alias: "source", CardID: 12345678, DeclaredType: "spell"})
	session, err = NewSession(pack, state, 7)
	if err != nil {
		t.Fatal(err)
	}
	illegal := session.Begin(strings.Repeat("b", 32), ir.SourceAction{Kind: "play", Actor: "own", Source: sourceID})
	if illegal.Status != StatusIllegal || illegal.IllegalCode != "target_required" || !session.g.unchanged || session.g.instances[sourceID].zone != "hand" || session.g.rng.Consumed() != 0 {
		t.Fatalf("preflight rejection changed state: %#v", illegal)
	}
}

func TestPreflightExcludesPlayedSpellFromHandCandidates(t *testing.T) {
	sourceID := strings.Repeat("9", 32)
	pack := &ir.CardPack{Cards: []ir.Card{
		{ID: 45678901, CardType: "spell", PlayEffects: []ir.Effect{
			ir.SelectionEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("e", 32)}, Kind: "require", Policy: "required", Binding: "target", Source: ir.ZoneRef{Kind: "zone", Side: "own", Zone: "hand", Member: "card"}},
		}},
	}}
	state := testState()
	state.Players["own"] = withInstance(state.Players["own"], "hand", ir.TestInstance{InstanceID: sourceID, Alias: "source", CardID: 45678901, DeclaredType: "spell"})
	session, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	step := session.Begin(strings.Repeat("f", 32), ir.SourceAction{Kind: "play", Actor: "own", Source: sourceID})
	if step.Status != StatusIllegal || step.IllegalCode != "target_required" || !session.g.unchanged || session.g.instances[sourceID].zone != "hand" {
		t.Fatalf("played spell remained a preflight hand candidate: %#v", step)
	}
}

func TestSessionModeRequestValidatesOption(t *testing.T) {
	sourceID := strings.Repeat("5", 32)
	leader := ir.LeaderRef{Kind: "leader", Side: "own", ValueType: "leader"}
	pack := &ir.CardPack{Cards: []ir.Card{
		{ID: 34567890, CardType: "spell", PlayEffects: []ir.Effect{
			ir.ModeEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("6", 32)}, Kind: "mode", Options: []ir.ModeOption{
				{ID: 1, Body: []ir.Effect{ir.TargetEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("7", 32)}, Kind: "heal", Target: leader, Amount: 1}}},
				{ID: 2, Body: []ir.Effect{ir.TargetEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("8", 32)}, Kind: "heal", Target: leader, Amount: 2}}},
			}},
		}},
	}}
	state := testState()
	p := state.Players["own"]
	p.Leader = ir.Leader{Life: 10, MaxLife: 20}
	state.Players["own"] = withInstance(p, "hand", ir.TestInstance{InstanceID: sourceID, Alias: "source", CardID: 34567890, DeclaredType: "spell"})
	session, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	step := session.Begin(strings.Repeat("c", 32), ir.SourceAction{Kind: "play", Actor: "own", Source: sourceID})
	if step.Status != StatusSuspended || step.Choice.Kind != "mode" {
		t.Fatalf("unexpected mode request: %#v", step)
	}
	bad := session.Resume(ChoiceResponse{RequestID: step.Choice.RequestID, ActionID: step.Choice.ActionID, StateRevision: step.Choice.StateRevision, SelectedOptionID: 3})
	if bad.Status != StatusRejected || session.g.own.leaderLife != 10 {
		t.Fatalf("invalid mode response was accepted: %#v", bad)
	}
	good := session.Resume(ChoiceResponse{RequestID: step.Choice.RequestID, ActionID: step.Choice.ActionID, StateRevision: step.Choice.StateRevision, SelectedOptionID: 2})
	if good.Status != StatusCompleted || session.g.own.leaderLife != 12 {
		t.Fatalf("mode continuation failed: %#v", good)
	}
}

func TestTriggerQueueDrainsFIFO(t *testing.T) {
	g := &game{instances: map[string]*instance{}, legal: true, rng: ruleset.NewRNG(1)}
	g.own.leaderLife, g.own.leaderMax = 10, 20
	leader := ir.LeaderRef{Kind: "leader", Side: "own", ValueType: "leader"}
	first := triggerInvocation{body: []ir.Effect{ir.AdjustEffect{Kind: "adjust_resource", Owner: "own", Resource: "maxpp", Delta: 1}}, bindings: frame{}}
	second := triggerInvocation{body: []ir.Effect{ir.IfEffect{Kind: "if", Condition: ir.CompareCondition{Kind: "compare", Op: "eq", Left: ir.Scalar{Kind: "scalar", Side: "own", Field: "maxpp"}, Right: 1}, Then: []ir.Effect{ir.TargetEffect{Kind: "heal", Target: leader, Amount: 1}}, Else: []ir.Effect{ir.TargetEffect{Kind: "heal", Target: leader, Amount: 2}}}}, bindings: frame{}}
	g.triggers = []triggerInvocation{first, second}
	session := &Session{g: g, actionID: strings.Repeat("d", 32)}
	if step := session.run(); step.Status != StatusCompleted {
		t.Fatalf("trigger queue did not complete: %#v", step)
	}
	if len(g.events) != 1 || g.events[0].Actual != 1 || g.own.leaderLife != 11 {
		t.Fatalf("trigger queue did not preserve FIFO: events=%#v life=%d", g.events, g.own.leaderLife)
	}
}

func testState() ir.State {
	zones := func() map[string][]ir.TestInstance {
		return map[string][]ir.TestInstance{"deck": {}, "hand": {}, "field": {}, "graveyard": {}, "banished": {}, "destroyed": {}}
	}
	return ir.State{Turn: ir.Turn{Active: "own", Number: 1}, Phase: "main", Players: map[string]ir.PlayerState{
		"own":  {Leader: ir.Leader{Life: 20, MaxLife: 20}, Zones: zones()},
		"oppo": {Leader: ir.Leader{Life: 20, MaxLife: 20}, Zones: zones()},
	}, Aliases: map[string]string{}}
}

func TestEndTurnPreparesNextPlayerDeterministically(t *testing.T) {
	spellID, followerID, amuletID := strings.Repeat("1", 32), strings.Repeat("2", 32), strings.Repeat("3", 32)
	pack := &ir.CardPack{Cards: []ir.Card{
		{ID: 12345678, CardType: "spell"},
		{ID: 23456789, CardType: "follower", Stats: &ir.Stats{Attack: 1, Life: 1}},
		{ID: 34567890, CardType: "amulet", Abilities: []ir.Ability{{ID: strings.Repeat("4", 32), Trigger: ir.CostTrigger{Kind: "engage", Cost: 0}}}},
	}}
	state := testState()
	own, oppo := state.Players["own"], state.Players["oppo"]
	own.PP, own.MaxPP, own.Combo = 1, 2, 5
	oppo.PP, oppo.MaxPP = 0, 3
	own = withInstance(own, "hand", ir.TestInstance{InstanceID: spellID, Alias: "spell", CardID: 12345678, DeclaredType: "spell"})
	oppo = withInstance(oppo, "deck", ir.TestInstance{InstanceID: followerID, Alias: "next", CardID: 23456789, DeclaredType: "follower"})
	oppo = withInstance(oppo, "field", ir.TestInstance{InstanceID: amuletID, Alias: "countdown", CardID: 34567890, DeclaredType: "amulet", Overrides: ir.InstanceOverrides{Countdown: intPtr(1), Engaged: boolPtr(true)}})
	state.Players["own"], state.Players["oppo"] = own, oppo
	session, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	session.g.oppo.attackedThisTurn = true
	result := session.Begin(strings.Repeat("5", 32), ir.SourceAction{Kind: "end_turn", Actor: "own"})
	if result.Status != StatusCompleted {
		t.Fatalf("end turn did not complete: %#v", result)
	}
	if session.g.turn.Active != "oppo" || session.g.turn.Number != 1 || session.g.oppo.maxpp != 4 || session.g.oppo.pp != 4 || session.g.oppo.combo != 0 || session.g.oppo.attackedThisTurn || len(session.g.oppo.hand) != 1 || len(session.g.oppo.field) != 0 {
		t.Fatalf("next turn preparation diverged: turn=%#v pp=%d/%d hand=%d field=%d", session.g.turn, session.g.oppo.pp, session.g.oppo.maxpp, len(session.g.oppo.hand), len(session.g.oppo.field))
	}
	if len(session.g.events) != 4 || session.g.events[0].Kind != "turn_ended" || session.g.events[1].Kind != "destroyed" || session.g.events[2].Kind != "card_drawn" || session.g.events[2].Side != "oppo" || session.g.events[3].Kind != "turn_started" {
		t.Fatalf("turn event order diverged: %#v", session.g.events)
	}
}

func TestAdvanceDrivesDeclaredTurnEvent(t *testing.T) {
	abilityID := strings.Repeat("6", 32)
	pack := &ir.CardPack{Cards: []ir.Card{{ID: 45678901, CardType: "follower", Stats: &ir.Stats{Attack: 1, Life: 1}, Abilities: []ir.Ability{{
		ID: abilityID, Trigger: ir.EventTrigger{Kind: "event", Event: "turn_started", Side: "own"}, Body: []ir.Effect{ir.AdjustEffect{Kind: "adjust_resource", Resource: "combo", Delta: 1}},
	}}}}}
	state := testState()
	state.Players["own"] = withInstance(state.Players["own"], "field", ir.TestInstance{InstanceID: strings.Repeat("7", 32), Alias: "listener", CardID: 45678901, DeclaredType: "follower"})
	session, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	result := session.Advance(ir.AdvanceAction{Kind: "advance", Timing: "turn_start", Side: "own"})
	if result.Status != StatusCompleted || session.g.own.combo != 1 || len(session.g.events) != 1 || session.g.events[0].Kind != "turn_started" {
		t.Fatalf("advance did not drive turn event: result=%#v combo=%d events=%#v", result, session.g.own.combo, session.g.events)
	}
}

func TestAttackResolvesSimultaneousFollowerDamage(t *testing.T) {
	attackerID, defenderID := strings.Repeat("8", 32), strings.Repeat("9", 32)
	pack := &ir.CardPack{Cards: []ir.Card{
		{ID: 11111111, CardType: "follower", Stats: &ir.Stats{Attack: 3, Life: 2}},
		{ID: 22222222, CardType: "follower", Stats: &ir.Stats{Attack: 2, Life: 3}},
	}}
	state := testState()
	state.Players["own"] = withInstance(state.Players["own"], "field", ir.TestInstance{InstanceID: attackerID, Alias: "attacker", CardID: 11111111, DeclaredType: "follower"})
	state.Players["oppo"] = withInstance(state.Players["oppo"], "field", ir.TestInstance{InstanceID: defenderID, Alias: "defender", CardID: 22222222, DeclaredType: "follower"})
	session, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	result := session.Begin(strings.Repeat("a", 32), ir.AttackAction{Kind: "attack_entity", Actor: "own", Attacker: attackerID, Defender: defenderID})
	if result.Status != StatusCompleted {
		t.Fatalf("attack did not complete: %#v", result)
	}
	if session.g.instances[attackerID].zone != "graveyard" || session.g.instances[defenderID].zone != "graveyard" {
		t.Fatalf("simultaneous damage did not resolve deaths: attacker=%s defender=%s", session.g.instances[attackerID].zone, session.g.instances[defenderID].zone)
	}
	if len(session.g.events) != 5 || session.g.events[0].Kind != "attacked" || session.g.events[1].Kind != "damaged" || session.g.events[2].Kind != "damaged" || session.g.events[3].Kind != "destroyed" || session.g.events[4].Kind != "destroyed" {
		t.Fatalf("unexpected attack events: %#v", session.g.events)
	}
	if !session.g.own.attackedThisTurn || session.g.instances[attackerID].attacksUsed != 0 {
		t.Fatalf("attack history or field-exit reset diverged: player=%t used=%d", session.g.own.attackedThisTurn, session.g.instances[attackerID].attacksUsed)
	}
}

func TestAttackCanDamageLeaderOncePerTurn(t *testing.T) {
	attackerID := strings.Repeat("a", 32)
	pack := &ir.CardPack{Cards: []ir.Card{{ID: 33333333, CardType: "follower", Stats: &ir.Stats{Attack: 4, Life: 4}}}}
	state := testState()
	state.Players["own"] = withInstance(state.Players["own"], "field", ir.TestInstance{InstanceID: attackerID, Alias: "attacker", CardID: 33333333, DeclaredType: "follower"})
	oppo := state.Players["oppo"]
	oppo.Leader.Life = 5
	state.Players["oppo"] = oppo
	session, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	first := session.Begin(strings.Repeat("b", 32), ir.AttackAction{Kind: "attack_leader", Actor: "own", Attacker: attackerID, Defender: "oppo"})
	if first.Status != StatusCompleted || session.g.oppo.leaderLife != 1 || session.g.instances[attackerID].attacksUsed != 1 || !session.g.own.attackedThisTurn {
		t.Fatalf("leader attack diverged: %#v", first)
	}
	second := session.Begin(strings.Repeat("c", 32), ir.AttackAction{Kind: "attack_leader", Actor: "own", Attacker: attackerID, Defender: "oppo"})
	if second.Status != StatusIllegal || second.IllegalCode != "already_attacked" || session.g.oppo.leaderLife != 1 {
		t.Fatalf("repeat attack was accepted: %#v", second)
	}
}

func TestAttackAbilityResolvesBeforeCombatDamage(t *testing.T) {
	attackerID := strings.Repeat("a", 32)
	abilityID := strings.Repeat("b", 32)
	leader := ir.LeaderRef{Kind: "leader", Side: "own", ValueType: "leader"}
	pack := &ir.CardPack{Cards: []ir.Card{{ID: 33333334, CardType: "follower", Stats: &ir.Stats{Attack: 4, Life: 4}, Abilities: []ir.Ability{{
		ID: abilityID, Trigger: ir.SimpleTrigger{Kind: "attack"}, Body: []ir.Effect{ir.TargetEffect{Kind: "heal", Target: leader, Amount: 2}},
	}}}}}
	state := testState()
	own := state.Players["own"]
	own.Leader.Life = 10
	state.Players["own"] = withInstance(own, "field", ir.TestInstance{InstanceID: attackerID, Alias: "attacker", CardID: 33333334, DeclaredType: "follower"})
	session, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	result := session.Begin(strings.Repeat("c", 32), ir.AttackAction{Kind: "attack_leader", Actor: "own", Attacker: attackerID, Defender: "oppo"})
	if result.Status != StatusCompleted || session.g.own.leaderLife != 12 || session.g.oppo.leaderLife != 16 {
		t.Fatalf("attack ability did not resolve before combat: result=%#v own=%d oppo=%d events=%#v", result, session.g.own.leaderLife, session.g.oppo.leaderLife, session.g.events)
	}
	if len(session.g.events) != 3 || session.g.events[0].Kind != "attacked" || session.g.events[1].Kind != "healed" || session.g.events[2].Kind != "damaged" {
		t.Fatalf("unexpected attack phase events: %#v", session.g.events)
	}
}

func TestAttackAbilityCanSuspendBeforeCombat(t *testing.T) {
	attackerID, defenderID := strings.Repeat("d", 32), strings.Repeat("e", 32)
	abilityID, selectionID := strings.Repeat("f", 32), strings.Repeat("1", 32)
	pack := &ir.CardPack{Cards: []ir.Card{
		{ID: 33333335, CardType: "follower", Stats: &ir.Stats{Attack: 4, Life: 4}, Abilities: []ir.Ability{{
			ID: abilityID, Trigger: ir.SimpleTrigger{Kind: "attack"}, Body: []ir.Effect{
				ir.SelectionEffect{NodeBase: ir.NodeBase{ID: selectionID}, Kind: "require", Policy: "required", Binding: "target", Source: ir.ZoneRef{Kind: "zone", Side: "oppo", Zone: "field", Member: "follower"}},
				ir.TargetEffect{Kind: "destroy", Target: ir.BindingRef{Kind: "binding", Name: "target"}},
			},
		}}},
		{ID: 33333336, CardType: "follower", Stats: &ir.Stats{Attack: 1, Life: 3}},
	}}
	state := testState()
	state.Players["own"] = withInstance(state.Players["own"], "field", ir.TestInstance{InstanceID: attackerID, Alias: "attacker", CardID: 33333335, DeclaredType: "follower"})
	state.Players["oppo"] = withInstance(state.Players["oppo"], "field", ir.TestInstance{InstanceID: defenderID, Alias: "defender", CardID: 33333336, DeclaredType: "follower"})
	session, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	step := session.Begin(strings.Repeat("2", 32), ir.AttackAction{Kind: "attack_entity", Actor: "own", Attacker: attackerID, Defender: defenderID})
	if step.Status != StatusSuspended || session.g.instances[defenderID].life != 3 || session.g.attack == nil {
		t.Fatalf("attack did not suspend before combat: step=%#v defender=%#v attack=%#v", step, session.g.instances[defenderID], session.g.attack)
	}
	data, err := session.EncodeContinuation()
	if err != nil {
		t.Fatal(err)
	}
	restored, err := RestoreSession(pack, mustDecodeContinuation(t, data))
	if err != nil {
		t.Fatal(err)
	}
	result := restored.Resume(ChoiceResponse{RequestID: step.Choice.RequestID, ActionID: step.Choice.ActionID, StateRevision: step.Choice.StateRevision, SelectedInstanceIDs: []string{defenderID}})
	if result.Status != StatusCompleted || restored.g.instances[defenderID].zone != "graveyard" || restored.g.instances[attackerID].life != 4 || restored.g.oppo.leaderLife != 20 || restored.g.attack != nil {
		t.Fatalf("attack continuation did not skip destroyed target combat: result=%#v attacker=%#v defender=%#v oppoLife=%d", result, restored.g.instances[attackerID], restored.g.instances[defenderID], restored.g.oppo.leaderLife)
	}
}

func mustDecodeContinuation(t *testing.T, data []byte) *Continuation {
	t.Helper()
	continuation, err := DecodeContinuation(data)
	if err != nil {
		t.Fatal(err)
	}
	return continuation
}

func TestFollowersCannotAttackUntilTheirNextTurn(t *testing.T) {
	playedID := strings.Repeat("1", 32)
	pack := &ir.CardPack{Cards: []ir.Card{{ID: 66666666, CardType: "follower", Cost: 1, Stats: &ir.Stats{Attack: 2, Life: 2}}}}
	state := testState()
	own := state.Players["own"]
	own.PP, own.MaxPP = 1, 1
	own = withInstance(own, "hand", ir.TestInstance{InstanceID: playedID, Alias: "played", CardID: 66666666, DeclaredType: "follower"})
	state.Players["own"] = own
	session, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	if result := session.Begin(strings.Repeat("2", 32), ir.SourceAction{Kind: "play", Actor: "own", Source: playedID}); result.Status != StatusCompleted {
		t.Fatalf("play failed: %#v", result)
	}
	if !session.g.instances[playedID].summoningSick {
		t.Fatal("played follower was ready immediately")
	}
	before := session.g.snapshot()
	attack := session.Begin(strings.Repeat("3", 32), ir.AttackAction{Kind: "attack_leader", Actor: "own", Attacker: playedID, Defender: "oppo"})
	if attack.Status != StatusIllegal || attack.IllegalCode != "summoning_sick" || !reflect.DeepEqual(before, session.g.snapshot()) {
		t.Fatalf("summoning-sick attack changed state: %#v", attack)
	}
	if result := session.Begin(strings.Repeat("4", 32), ir.SourceAction{Kind: "end_turn", Actor: "own"}); result.Status != StatusCompleted {
		t.Fatalf("own turn did not end: %#v", result)
	}
	if result := session.Begin(strings.Repeat("5", 32), ir.SourceAction{Kind: "end_turn", Actor: "oppo"}); result.Status != StatusCompleted {
		t.Fatalf("opponent turn did not end: %#v", result)
	}
	if session.g.instances[playedID].summoningSick {
		t.Fatal("follower remained sick on its controller's next turn")
	}
	attack = session.Begin(strings.Repeat("6", 32), ir.AttackAction{Kind: "attack_leader", Actor: "own", Attacker: playedID, Defender: "oppo"})
	if attack.Status != StatusCompleted || session.g.oppo.leaderLife != 18 {
		t.Fatalf("ready follower could not attack: %#v", attack)
	}
}

func TestEffectSummonedFollowerIsSummoningSick(t *testing.T) {
	spellID := strings.Repeat("7", 32)
	pack := &ir.CardPack{Cards: []ir.Card{
		{ID: 77777777, CardType: "spell", PlayEffects: []ir.Effect{ir.CardEffect{Kind: "summon", Owner: "own", CardID: 88888888, Count: 1, Output: "summoned"}}},
		{ID: 88888888, CardType: "follower", Stats: &ir.Stats{Attack: 1, Life: 1}},
	}}
	state := testState()
	state.Players["own"] = withInstance(state.Players["own"], "hand", ir.TestInstance{InstanceID: spellID, Alias: "spell", CardID: 77777777, DeclaredType: "spell"})
	session, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	if result := session.Begin(strings.Repeat("8", 32), ir.SourceAction{Kind: "play", Actor: "own", Source: spellID}); result.Status != StatusCompleted {
		t.Fatalf("summon spell failed: %#v", result)
	}
	summoned := session.g.instances["summoned-1"]
	if summoned == nil || !summoned.summoningSick {
		t.Fatalf("summoned follower combat state diverged: %#v", summoned)
	}
	result := session.Begin(strings.Repeat("9", 32), ir.AttackAction{Kind: "attack_leader", Actor: "own", Attacker: summoned.id, Defender: "oppo"})
	if result.Status != StatusIllegal || result.IllegalCode != "summoning_sick" {
		t.Fatalf("summoned follower attacked immediately: %#v", result)
	}
}

func TestStormAndRushOverrideSummoningSicknessWithTargetLimits(t *testing.T) {
	stormID, rushID := strings.Repeat("3", 32), strings.Repeat("4", 32)
	pack := &ir.CardPack{Cards: []ir.Card{
		{ID: 66666667, CardType: "follower", Stats: &ir.Stats{Attack: 1, Life: 1}, Intrinsic: []string{"storm"}},
		{ID: 66666668, CardType: "follower", Stats: &ir.Stats{Attack: 1, Life: 1}, Intrinsic: []string{"rush"}},
	}}
	state := testState()
	state.Players["own"] = withInstance(state.Players["own"], "field", ir.TestInstance{InstanceID: stormID, CardID: 66666667, DeclaredType: "follower"})
	state.Players["own"] = withInstance(state.Players["own"], "field", ir.TestInstance{InstanceID: rushID, CardID: 66666668, DeclaredType: "follower"})
	session, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	if result := session.Begin(strings.Repeat("5", 32), ir.AttackAction{Kind: "attack_leader", Actor: "own", Attacker: rushID, Defender: "oppo"}); result.Status != StatusIllegal || result.IllegalCode != "rush_cannot_attack_leader" {
		t.Fatalf("rush attacked leader: %#v", result)
	}
	if result := session.Begin(strings.Repeat("6", 32), ir.AttackAction{Kind: "attack_leader", Actor: "own", Attacker: stormID, Defender: "oppo"}); result.Status != StatusCompleted {
		t.Fatalf("storm could not attack leader: %#v", result)
	}
}

func TestWardAndIntimidateRestrictFollowerAttackTargets(t *testing.T) {
	attackerID, wardID, guardedID := strings.Repeat("7", 32), strings.Repeat("8", 32), strings.Repeat("9", 32)
	pack := &ir.CardPack{Cards: []ir.Card{
		{ID: 66666669, CardType: "follower", Stats: &ir.Stats{Attack: 1, Life: 3}},
		{ID: 66666670, CardType: "follower", Stats: &ir.Stats{Attack: 1, Life: 3}, Intrinsic: []string{"ward"}},
		{ID: 66666671, CardType: "follower", Stats: &ir.Stats{Attack: 1, Life: 3}, Intrinsic: []string{"intimidate"}},
	}}
	state := testState()
	state.Players["own"] = withInstance(state.Players["own"], "field", ir.TestInstance{InstanceID: attackerID, CardID: 66666669, DeclaredType: "follower"})
	state.Players["oppo"] = withInstance(state.Players["oppo"], "field", ir.TestInstance{InstanceID: wardID, CardID: 66666670, DeclaredType: "follower"})
	state.Players["oppo"] = withInstance(state.Players["oppo"], "field", ir.TestInstance{InstanceID: guardedID, CardID: 66666671, DeclaredType: "follower"})
	session, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range []ir.AttackAction{{Kind: "attack_leader", Actor: "own", Attacker: attackerID, Defender: "oppo"}} {
		result := session.Begin(strings.Repeat("a", 32), action)
		if result.Status != StatusIllegal || result.IllegalCode != "ward_required" {
			t.Fatalf("ward restriction failed: action=%#v result=%#v", action, result)
		}
	}
	result := session.Begin(strings.Repeat("a", 32), ir.AttackAction{Kind: "attack_entity", Actor: "own", Attacker: attackerID, Defender: guardedID})
	if result.Status != StatusIllegal || result.IllegalCode != "intimidate_target" {
		t.Fatalf("intimidate target was not rejected: %#v", result)
	}
	result = session.Begin(strings.Repeat("b", 32), ir.AttackAction{Kind: "attack_entity", Actor: "own", Attacker: attackerID, Defender: wardID})
	if result.Status != StatusCompleted {
		t.Fatalf("ward target was not attackable: %#v", result)
	}
	state = testState()
	state.Players["own"] = withInstance(state.Players["own"], "field", ir.TestInstance{InstanceID: attackerID, CardID: 66666669, DeclaredType: "follower"})
	state.Players["oppo"] = withInstance(state.Players["oppo"], "field", ir.TestInstance{InstanceID: guardedID, CardID: 66666671, DeclaredType: "follower"})
	session, _ = NewSession(pack, state, 1)
	result = session.Begin(strings.Repeat("c", 32), ir.AttackAction{Kind: "attack_entity", Actor: "own", Attacker: attackerID, Defender: guardedID})
	if result.Status != StatusIllegal || result.IllegalCode != "intimidate_target" {
		t.Fatalf("intimidate target was attackable: %#v", result)
	}
}

func TestBaneDestroysAtZeroDamageAndDrainUsesActualDamage(t *testing.T) {
	attackerID, defenderID := strings.Repeat("d", 32), strings.Repeat("e", 32)
	pack := &ir.CardPack{Cards: []ir.Card{
		{ID: 66666672, CardType: "follower", Stats: &ir.Stats{Attack: 0, Life: 3}, Intrinsic: []string{"bane", "drain"}},
		{ID: 66666673, CardType: "follower", Stats: &ir.Stats{Attack: 1, Life: 2}},
	}}
	state := testState()
	own := state.Players["own"]
	own.Leader.Life = 10
	state.Players["own"] = withInstance(own, "field", ir.TestInstance{InstanceID: attackerID, CardID: 66666672, DeclaredType: "follower"})
	state.Players["oppo"] = withInstance(state.Players["oppo"], "field", ir.TestInstance{InstanceID: defenderID, CardID: 66666673, DeclaredType: "follower"})
	session, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	result := session.Begin(strings.Repeat("f", 32), ir.AttackAction{Kind: "attack_entity", Actor: "own", Attacker: attackerID, Defender: defenderID})
	if result.Status != StatusCompleted || session.g.instances[defenderID].zone != "graveyard" || session.g.oppo.leaderLife != 20 || session.g.own.leaderLife != 10 {
		t.Fatalf("bane or zero drain diverged: result=%#v attacker=%#v defender=%#v leaders=%d/%d", result, session.g.instances[attackerID], session.g.instances[defenderID], session.g.own.leaderLife, session.g.oppo.leaderLife)
	}
}

func TestDrainUsesActualLeaderDamageAndCapsAtTargetLife(t *testing.T) {
	attackerID := strings.Repeat("6", 32)
	pack := &ir.CardPack{Cards: []ir.Card{{ID: 66666676, CardType: "follower", Stats: &ir.Stats{Attack: 5, Life: 2}, Intrinsic: []string{"drain"}}}}
	state := testState()
	own := state.Players["own"]
	own.Leader.Life = 10
	state.Players["own"] = withInstance(own, "field", ir.TestInstance{InstanceID: attackerID, CardID: 66666676, DeclaredType: "follower"})
	oppo := state.Players["oppo"]
	oppo.Leader.Life = 2
	state.Players["oppo"] = oppo
	session, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	result := session.Begin(strings.Repeat("7", 32), ir.AttackAction{Kind: "attack_leader", Actor: "own", Attacker: attackerID, Defender: "oppo"})
	if result.Status != StatusCompleted || session.g.own.leaderLife != 12 || !session.g.gameOver {
		t.Fatalf("drain did not use actual leader damage: result=%#v own=%d gameOver=%t events=%#v", result, session.g.own.leaderLife, session.g.gameOver, session.g.events)
	}
}

func TestBarrierAndSuperEvolutionProtectDamageAndEffectDestruction(t *testing.T) {
	barrierID, superID := strings.Repeat("8", 32), strings.Repeat("9", 32)
	pack := &ir.CardPack{Cards: []ir.Card{
		{ID: 66666677, CardType: "follower", Stats: &ir.Stats{Attack: 1, Life: 3}, Intrinsic: []string{"barrier"}},
		{ID: 66666678, CardType: "follower", Stats: &ir.Stats{Attack: 1, Life: 3}},
	}}
	state := testState()
	state.Players["own"] = withInstance(state.Players["own"], "field", ir.TestInstance{InstanceID: barrierID, CardID: 66666677, DeclaredType: "follower"})
	state.Players["own"] = withInstance(state.Players["own"], "field", ir.TestInstance{InstanceID: superID, CardID: 66666678, DeclaredType: "follower", Overrides: ir.InstanceOverrides{SuperEvolved: boolPtr(true)}})
	session, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	session.g.damageInstance(session.g.instances[barrierID], 2)
	if session.g.instances[barrierID].life != 3 || session.g.instances[barrierID].abilities["barrier"] {
		t.Fatalf("barrier did not absorb exactly once: %#v", session.g.instances[barrierID])
	}
	session.g.damageInstance(session.g.instances[barrierID], 2)
	if session.g.instances[barrierID].life != 1 {
		t.Fatalf("second barrier damage diverged: %#v", session.g.instances[barrierID])
	}
	session.g.damageInstance(session.g.instances[superID], 2)
	if session.g.instances[superID].life != 3 {
		t.Fatalf("super-evolved follower took damage on own turn: %#v", session.g.instances[superID])
	}
	session.g.destroyByEffect([]*instance{session.g.instances[superID]})
	if session.g.instances[superID].zone != "field" {
		t.Fatalf("super-evolved follower was destroyed by effect on own turn: %#v", session.g.instances[superID])
	}
}

func TestEffectDamageCanTargetLeader(t *testing.T) {
	session, err := NewSession(&ir.CardPack{}, testState(), 1)
	if err != nil {
		t.Fatal(err)
	}
	session.g.execTargetEffect(ir.TargetEffect{Kind: "damage", Target: ir.LeaderRef{Kind: "leader", Side: "oppo"}, Amount: 3}, nil, frame{})
	if session.g.oppo.leaderLife != 17 || len(session.g.events) != 1 || session.g.events[0].Kind != "damaged" || session.g.events[0].Actual != 3 {
		t.Fatalf("leader damage was not applied: life=%d events=%#v", session.g.oppo.leaderLife, session.g.events)
	}
}

func TestClashAbilitiesResolveAttackerThenDefenderBeforeCombat(t *testing.T) {
	attackerID, defenderID := strings.Repeat("1", 32), strings.Repeat("2", 32)
	attackerAbility, defenderAbility := strings.Repeat("3", 32), strings.Repeat("4", 32)
	pack := &ir.CardPack{Cards: []ir.Card{
		{ID: 66666674, CardType: "follower", Stats: &ir.Stats{Attack: 1, Life: 3}, Abilities: []ir.Ability{{
			ID: attackerAbility, Trigger: ir.SimpleTrigger{Kind: "clash"}, Body: []ir.Effect{ir.TargetEffect{
				Kind: "buff_stats", Target: ir.SelfRef{Kind: "self", ValueType: "follower"}, AttackDelta: 1,
			}},
		}}},
		{ID: 66666675, CardType: "follower", Stats: &ir.Stats{Attack: 1, Life: 2}, Abilities: []ir.Ability{{
			ID: defenderAbility, Trigger: ir.SimpleTrigger{Kind: "clash"}, Body: []ir.Effect{ir.TargetEffect{
				Kind: "damage", Target: ir.ZoneRef{Kind: "zone", Side: "oppo", Zone: "field", Member: "follower"}, Amount: 1,
			}},
		}}},
	}}
	state := testState()
	state.Players["own"] = withInstance(state.Players["own"], "field", ir.TestInstance{InstanceID: attackerID, CardID: 66666674, DeclaredType: "follower"})
	state.Players["oppo"] = withInstance(state.Players["oppo"], "field", ir.TestInstance{InstanceID: defenderID, CardID: 66666675, DeclaredType: "follower"})
	session, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	result := session.Begin(strings.Repeat("5", 32), ir.AttackAction{Kind: "attack_entity", Actor: "own", Attacker: attackerID, Defender: defenderID})
	if result.Status != StatusCompleted || session.g.instances[defenderID].zone != "graveyard" || session.g.instances[attackerID].life != 1 {
		t.Fatalf("clash sequence diverged: result=%#v attacker=%#v defender=%#v", result, session.g.instances[attackerID], session.g.instances[defenderID])
	}
}

func TestLethalAttackEndsGameAndRejectsLaterActions(t *testing.T) {
	attackerID := strings.Repeat("d", 32)
	pack := &ir.CardPack{Cards: []ir.Card{{ID: 44444444, CardType: "follower", Stats: &ir.Stats{Attack: 5, Life: 3}}}}
	state := testState()
	state.Players["own"] = withInstance(state.Players["own"], "field", ir.TestInstance{InstanceID: attackerID, Alias: "attacker", CardID: 44444444, DeclaredType: "follower"})
	oppo := state.Players["oppo"]
	oppo.Leader.Life = 5
	state.Players["oppo"] = oppo
	session, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	result := session.Begin(strings.Repeat("e", 32), ir.AttackAction{Kind: "attack_leader", Actor: "own", Attacker: attackerID, Defender: "oppo"})
	if result.Status != StatusCompleted || !session.g.gameOver || session.g.winner != "own" || session.g.oppo.leaderLife != 0 {
		t.Fatalf("lethal attack did not end game: result=%#v gameOver=%t winner=%s life=%d", result, session.g.gameOver, session.g.winner, session.g.oppo.leaderLife)
	}
	if len(session.g.events) != 3 || session.g.events[2].Kind != "game_ended" || session.g.events[2].Side != "own" {
		t.Fatalf("unexpected lethal events: %#v", session.g.events)
	}
	before := session.g.snapshot()
	rejected := session.Begin(strings.Repeat("f", 32), ir.SourceAction{Kind: "end_turn", Actor: "own"})
	if rejected.Status != StatusRejected || rejected.ErrorCode != "game_over" || !reflect.DeepEqual(before, session.g.snapshot()) {
		t.Fatalf("post-game action changed state: result=%#v", rejected)
	}
	view, err := session.View("oppo")
	if err != nil || !view.GameOver || view.Winner != "oppo" || len(session.LegalActions()) != 0 {
		t.Fatalf("terminal view diverged: view=%#v actions=%#v err=%v", view, session.LegalActions(), err)
	}
	restored, err := restoreGame(session.g.cards, snapshotContinuationGame(session.g))
	if err != nil || !restored.gameOver || restored.winner != "own" || restored.oppo.leaderLife != 0 {
		t.Fatalf("terminal state did not restore: game=%#v err=%v", restored, err)
	}
}

func TestOpponentCanSubmitAfterTurnChange(t *testing.T) {
	instanceID := strings.Repeat("2", 32)
	pack := &ir.CardPack{Cards: []ir.Card{{ID: 55555555, CardType: "follower", Cost: 1, Stats: &ir.Stats{Attack: 1, Life: 1}}}}
	state := testState()
	oppo := state.Players["oppo"]
	oppo.PP, oppo.MaxPP = 0, 0
	oppo = withInstance(oppo, "hand", ir.TestInstance{InstanceID: instanceID, Alias: "oppo-card", CardID: 55555555, DeclaredType: "follower"})
	state.Players["oppo"] = oppo
	session, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	if result := session.Begin(strings.Repeat("3", 32), ir.SourceAction{Kind: "end_turn", Actor: "own"}); result.Status != StatusCompleted || session.g.turn.Active != "oppo" {
		t.Fatalf("turn did not change: %#v", result)
	}
	result := session.Begin(strings.Repeat("4", 32), ir.SourceAction{Kind: "play", Actor: "oppo", Source: instanceID})
	if result.Status != StatusCompleted || !contains(session.g.oppo.field, session.g.instances[instanceID]) || !session.g.instances[instanceID].summoningSick || len(session.g.oppo.hand) != 0 {
		t.Fatalf("opponent action did not use opponent state: result=%#v field=%#v hand=%#v", result, session.g.oppo.field, session.g.oppo.hand)
	}
}

func intPtr(value int) *int    { return &value }
func boolPtr(value bool) *bool { return &value }

func withInstance(player ir.PlayerState, zone string, instance ir.TestInstance) ir.PlayerState {
	player.Zones[zone] = append(player.Zones[zone], instance)
	return player
}
