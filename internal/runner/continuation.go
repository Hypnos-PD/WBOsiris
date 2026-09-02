package runner

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"wbo/internal/ir"
	"wbo/internal/ruleset"
)

const continuationVersion = "0.6.0"

type ContinuationBindings struct {
	ID     string              `json:"id"`
	Values map[string][]string `json:"values"`
}

type ContinuationPending struct {
	Request        ChoiceRequest        `json:"request"`
	Binding        string               `json:"binding,omitempty"`
	BindingFrameID string               `json:"bindingFrameId"`
	SelfInstanceID string               `json:"selfInstanceId,omitempty"`
	OptionBlockIDs []ContinuationOption `json:"optionBlockIds"`
}

type ContinuationOption struct {
	OptionID int    `json:"optionId"`
	BlockID  string `json:"blockId"`
}

type ContinuationTrigger struct {
	BlockID        string `json:"blockId"`
	SelfInstanceID string `json:"selfInstanceId,omitempty"`
	BindingFrameID string `json:"bindingFrameId"`
}

type ContinuationGame struct {
	Own              ContinuationPlayer   `json:"own"`
	Oppo             ContinuationPlayer   `json:"oppo"`
	Instances        []ContinuationEntity `json:"instances"`
	Events           []ContinuationEvent  `json:"events"`
	RNG              ContinuationRNG      `json:"rng"`
	Serial           int                  `json:"serial"`
	EventSequence    uint64               `json:"eventSequence"`
	DeathBatchSerial uint64               `json:"deathBatchSerial"`
	Revision         uint64               `json:"revision"`
	Legal            bool                 `json:"legal"`
	Illegal          string               `json:"illegal,omitempty"`
	Unchanged        bool                 `json:"unchanged"`
	Turn             ir.Turn              `json:"turn"`
	Phase            string               `json:"phase"`
	TurnTransition   string               `json:"turnTransition"`
	GameOver         bool                 `json:"gameOver"`
	Winner           string               `json:"winner,omitempty"`
	Attack           *ContinuationAttack  `json:"attack,omitempty"`
}

type ContinuationAttack struct {
	Stage          string `json:"stage"`
	Actor          string `json:"actor"`
	Attacker       string `json:"attacker"`
	Defender       string `json:"defender,omitempty"`
	AttackerAttack int    `json:"attackerAttack,omitempty"`
	DefenderAttack int    `json:"defenderAttack,omitempty"`
}

type ContinuationPlayer struct {
	PP               int      `json:"pp"`
	MaxPP            int      `json:"maxpp"`
	LeaderLife       int      `json:"leaderLife"`
	LeaderMax        int      `json:"leaderMax"`
	EP               int      `json:"ep"`
	SEP              int      `json:"sep"`
	Combo            int      `json:"combo"`
	Shadows          int      `json:"shadows"`
	AttackedThisTurn bool     `json:"attackedThisTurn"`
	EvolvedThisTurn  bool     `json:"evolvedThisTurn"`
	Deck             []string `json:"deck"`
	Hand             []string `json:"hand"`
	Field            []string `json:"field"`
	Graveyard        []string `json:"graveyard"`
	Banished         []string `json:"banished"`
	Destroyed        []string `json:"destroyed"`
}

type ContinuationEntity struct {
	ID              string   `json:"id"`
	Alias           string   `json:"alias"`
	Zone            string   `json:"zone"`
	CardID          int      `json:"cardId"`
	Attack          int      `json:"attack"`
	Life            int      `json:"life"`
	Earthsigil      int      `json:"earthsigil"`
	DamageReduction int      `json:"damageReduction"`
	Countdown       int      `json:"countdown"`
	AttacksUsed     int      `json:"attacksUsed"`
	AttackLimit     int      `json:"attackLimit"`
	Engaged         bool     `json:"engaged"`
	SummoningSick   bool     `json:"summoningSick"`
	Evolved         bool     `json:"evolved"`
	SuperEvolved    bool     `json:"superEvolved"`
	Abilities       []string `json:"abilities"`
	Materials       []string `json:"materials,omitempty"`
}

type ContinuationEvent struct {
	Kind       string          `json:"kind"`
	Side       string          `json:"side,omitempty"`
	InstanceID string          `json:"instanceId,omitempty"`
	CardID     int             `json:"cardId,omitempty"`
	Count      int             `json:"count,omitempty"`
	Actual     int             `json:"actual,omitempty"`
	Target     *ir.EventTarget `json:"target,omitempty"`
	Subject    *ir.EventTarget `json:"subject,omitempty"`
	Attacker   *ir.EventTarget `json:"attacker,omitempty"`
	Defender   *ir.EventTarget `json:"defender,omitempty"`
	Sequence   uint64          `json:"sequence"`
	BatchID    uint64          `json:"batchId,omitempty"`
}

type ContinuationRNG struct {
	State    uint64 `json:"state"`
	Consumed uint64 `json:"consumed"`
}

func (s *Session) EncodeContinuation() ([]byte, error) {
	c := s.Continuation()
	if c == nil {
		return nil, fmt.Errorf("session is not suspended")
	}
	data, err := json.Marshal(c)
	if err != nil {
		return nil, err
	}
	limit := s.budgetPolicy.ContinuationBytes
	if uint64(len(data)) > limit {
		return nil, fmt.Errorf("continuation exceeds %d bytes", limit)
	}
	return data, nil
}

func DecodeContinuation(data []byte) (*Continuation, error) {
	limit := ruleset.Default().ExecutionBudget.ContinuationBytes
	if uint64(len(data)) > limit {
		return nil, fmt.Errorf("continuation exceeds %d bytes", limit)
	}
	if !json.Valid(data) {
		return nil, fmt.Errorf("invalid continuation JSON")
	}
	if err := rejectDuplicateContinuationKeys(data); err != nil {
		return nil, err
	}
	var c Continuation
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(&c); err != nil {
		return nil, fmt.Errorf("decode continuation: %w", err)
	}
	if err := d.Decode(&struct{}{}); err != io.EOF {
		return nil, fmt.Errorf("continuation has trailing data")
	}
	return &c, nil
}

func RestoreSession(cards *ir.CardPack, c *Continuation) (*Session, error) {
	if cards == nil || c == nil {
		return nil, fmt.Errorf("cards and continuation are required")
	}
	index := map[int]*ir.Card{}
	for n := range cards.Cards {
		card := &cards.Cards[n]
		if index[card.ID] != nil {
			return nil, fmt.Errorf("duplicate card %d", card.ID)
		}
		index[card.ID] = card
	}
	blocks, err := indexBlocks(index)
	if err != nil {
		return nil, err
	}
	packHash, err := runtimeCardPackHash(cards, index)
	if err != nil {
		return nil, err
	}
	dependency := ruleset.DefaultDependency()
	if c.Version != continuationVersion {
		return nil, fmt.Errorf("unsupported continuation version %q", c.Version)
	}
	if c.CardPackHash != packHash {
		return nil, fmt.Errorf("continuation card pack mismatch")
	}
	if c.RulesetID != dependency.ID || c.RulesetHash != dependency.ContentHash {
		return nil, fmt.Errorf("continuation ruleset mismatch")
	}
	if !validRuntimeID(c.ActionID) || !validRuntimeID(c.RequestID) || c.RequestOrdinal == 0 {
		return nil, fmt.Errorf("invalid continuation identifiers")
	}
	g, err := restoreGame(index, c.Game)
	if err != nil {
		return nil, err
	}
	if c.StateRevision != g.revision || c.Pending.Request.StateRevision != g.revision || c.Pending.Request.ActionID != c.ActionID || c.Pending.Request.RequestID != c.RequestID {
		return nil, fmt.Errorf("continuation state or request mismatch")
	}
	if deriveRuntimeID(c.ActionID, c.Pending.Request.NodeID, fmt.Sprint(c.RequestOrdinal)) != c.RequestID {
		return nil, fmt.Errorf("continuation request ID mismatch")
	}

	bindings, err := restoreBindings(c.BindingFrames, g.instances)
	if err != nil {
		return nil, err
	}
	policy := ruleset.Default().ExecutionBudget
	s := &Session{g: g, actionID: c.ActionID, requestOrdinal: c.RequestOrdinal, blocks: blocks, cardPackHash: packHash, drainingTrigger: c.DrainingTrigger, triggerBase: c.TriggerBase, budgetPolicy: policy}
	s.budget.reset(policy)
	if !s.budget.validState(c.Budget) {
		return nil, fmt.Errorf("continuation execution budget is invalid")
	}
	s.budget.state = c.Budget
	for _, saved := range c.Stack {
		body, ok := blocks[saved.BlockID]
		if !ok || saved.PC < 0 || saved.PC > len(body) {
			return nil, fmt.Errorf("invalid continuation stack block %q", saved.BlockID)
		}
		bindingFrame, ok := bindings[saved.BindingFrameID]
		if !ok {
			return nil, fmt.Errorf("unknown binding frame %q", saved.BindingFrameID)
		}
		self, err := continuationInstance(g.instances, saved.SelfInstanceID)
		if err != nil {
			return nil, err
		}
		s.stack = append(s.stack, execFrame{body: body, blockID: saved.BlockID, pc: saved.PC, self: self, bindings: bindingFrame})
	}
	if c.TriggerBase < 0 || c.TriggerBase > len(s.stack) || c.DrainingTrigger && c.TriggerBase == len(s.stack) {
		return nil, fmt.Errorf("invalid continuation trigger checkpoint")
	}
	if uint64(len(s.stack)) > s.budget.state.MaxStackDepth {
		return nil, fmt.Errorf("continuation stack exceeds recorded budget state")
	}

	pendingBindings, ok := bindings[c.Pending.BindingFrameID]
	if !ok {
		return nil, fmt.Errorf("unknown pending binding frame %q", c.Pending.BindingFrameID)
	}
	pendingSelf, err := continuationInstance(g.instances, c.Pending.SelfInstanceID)
	if err != nil {
		return nil, err
	}
	pending := &pendingChoice{request: c.Pending.Request, binding: c.Pending.Binding, bindings: pendingBindings, self: pendingSelf, options: map[int][]ir.Effect{}, optionBlockIDs: map[int]string{}}
	if err := validateChoiceRequest(pending.request, g.instances); err != nil {
		return nil, err
	}
	for _, option := range c.Pending.OptionBlockIDs {
		body, ok := blocks[option.BlockID]
		_, duplicate := pending.options[option.OptionID]
		if !ok || option.OptionID == 0 || duplicate {
			return nil, fmt.Errorf("invalid pending option block")
		}
		pending.options[option.OptionID] = body
		pending.optionBlockIDs[option.OptionID] = option.BlockID
	}
	if pending.request.Kind == "target" {
		if pending.binding == "" || len(pending.options) != 0 {
			return nil, fmt.Errorf("invalid target continuation")
		}
	} else if pending.request.Kind == "mode" {
		if pending.binding != "" || len(pending.options) != len(pending.request.Candidates) {
			return nil, fmt.Errorf("invalid mode continuation")
		}
	} else {
		return nil, fmt.Errorf("unsupported pending request kind %q", pending.request.Kind)
	}
	if err := validatePendingNode(s, pending); err != nil {
		return nil, err
	}
	s.pending = pending

	for _, saved := range c.Triggers {
		body, ok := blocks[saved.BlockID]
		if !ok {
			return nil, fmt.Errorf("invalid trigger block %q", saved.BlockID)
		}
		bindingFrame, ok := bindings[saved.BindingFrameID]
		if !ok {
			return nil, fmt.Errorf("unknown trigger binding frame %q", saved.BindingFrameID)
		}
		self, err := continuationInstance(g.instances, saved.SelfInstanceID)
		if err != nil {
			return nil, err
		}
		s.g.triggers = append(s.g.triggers, triggerInvocation{body: body, blockID: saved.BlockID, self: self, bindings: bindingFrame})
	}
	g.budget = &s.budget
	return s, nil
}

func (s *Session) makeContinuation() *Continuation {
	dependency := ruleset.DefaultDependency()
	c := &Continuation{
		Version: continuationVersion, CardPackHash: s.cardPackHash,
		RulesetID: dependency.ID, RulesetHash: dependency.ContentHash,
		ActionID: s.actionID, RequestID: s.pending.request.RequestID,
		StateRevision: s.g.revision, RequestOrdinal: s.requestOrdinal,
		DrainingTrigger: s.drainingTrigger, TriggerBase: s.triggerBase,
		Game: snapshotContinuationGame(s.g), Budget: s.budget.state,
	}

	frameIDs := map[uintptr]string{}
	addBindings := func(f frame) string {
		key := reflect.ValueOf(f).Pointer()
		if id := frameIDs[key]; id != "" {
			return id
		}
		id := fmt.Sprintf("frame-%d", len(frameIDs)+1)
		frameIDs[key] = id
		values := map[string][]string{}
		for name, instances := range f {
			values[name] = instanceIDs(instances)
		}
		c.BindingFrames = append(c.BindingFrames, ContinuationBindings{ID: id, Values: values})
		return id
	}
	for _, f := range s.stack {
		c.Stack = append(c.Stack, ContinuationFrame{BlockID: f.blockID, PC: f.pc, SelfInstanceID: instanceID(f.self), BindingFrameID: addBindings(f.bindings)})
	}
	c.Pending = ContinuationPending{Request: s.pending.request, Binding: s.pending.binding, BindingFrameID: addBindings(s.pending.bindings), SelfInstanceID: instanceID(s.pending.self)}
	optionIDs := make([]int, 0, len(s.pending.optionBlockIDs))
	for optionID := range s.pending.optionBlockIDs {
		optionIDs = append(optionIDs, optionID)
	}
	sort.Ints(optionIDs)
	for _, optionID := range optionIDs {
		c.Pending.OptionBlockIDs = append(c.Pending.OptionBlockIDs, ContinuationOption{OptionID: optionID, BlockID: s.pending.optionBlockIDs[optionID]})
	}
	for _, trigger := range s.g.triggers {
		c.Triggers = append(c.Triggers, ContinuationTrigger{BlockID: trigger.blockID, SelfInstanceID: instanceID(trigger.self), BindingFrameID: addBindings(trigger.bindings)})
	}
	return c
}

func snapshotContinuationGame(g *game) ContinuationGame {
	snapshot := ContinuationGame{
		Own: snapshotContinuationPlayer(g.own), Oppo: snapshotContinuationPlayer(g.oppo),
		RNG:    ContinuationRNG{State: g.rng.Snapshot().State, Consumed: g.rng.Snapshot().Consumed},
		Serial: g.serial, EventSequence: g.eventSequence, DeathBatchSerial: g.deathBatchSerial, Revision: g.revision, Legal: g.legal, Illegal: g.illegal, Unchanged: g.unchanged, Turn: g.turn, Phase: g.phase, TurnTransition: g.turnTransition, GameOver: g.gameOver, Winner: g.winner,
	}
	if g.attack != nil {
		snapshot.Attack = &ContinuationAttack{Stage: g.attack.stage, Actor: g.attack.actor, Attacker: g.attack.attacker, Defender: g.attack.defender, AttackerAttack: g.attack.attackerAttack, DefenderAttack: g.attack.defenderAttack}
	}
	ids := make([]string, 0, len(g.instances))
	for id := range g.instances {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		i := g.instances[id]
		abilities := make([]string, 0, len(i.abilities))
		for ability, enabled := range i.abilities {
			if enabled {
				abilities = append(abilities, ability)
			}
		}
		sort.Strings(abilities)
		snapshot.Instances = append(snapshot.Instances, ContinuationEntity{
			ID: i.id, Alias: i.alias, Zone: i.zone, CardID: i.card.ID, Attack: i.attack, Life: i.life,
			Earthsigil: i.earthsigil, Countdown: i.countdown, AttacksUsed: i.attacksUsed, AttackLimit: attackLimit(i),
			Engaged: i.engaged, SummoningSick: i.summoningSick, Evolved: i.evolved,
			SuperEvolved: i.superEvolved, DamageReduction: i.damageReduction, Abilities: abilities, Materials: instanceIDs(i.materials),
		})
	}
	for _, event := range g.events {
		snapshot.Events = append(snapshot.Events, ContinuationEvent{Kind: event.Kind, Side: event.Side, InstanceID: event.InstanceID, CardID: event.CardID, Count: event.Count, Actual: event.Actual, Target: cloneEventTarget(event.Target), Subject: cloneEventTarget(event.Subject), Attacker: cloneEventTarget(event.Attacker), Defender: cloneEventTarget(event.Defender), Sequence: event.Sequence, BatchID: event.BatchID})
	}
	return snapshot
}

func snapshotContinuationPlayer(p player) ContinuationPlayer {
	return ContinuationPlayer{
		PP: p.pp, MaxPP: p.maxpp, LeaderLife: p.leaderLife, LeaderMax: p.leaderMax,
		EP: p.ep, SEP: p.sep, Combo: p.combo, Shadows: p.shadows, AttackedThisTurn: p.attackedThisTurn,
		Deck: instanceIDs(p.deck), Hand: instanceIDs(p.hand), Field: instanceIDs(p.field), EvolvedThisTurn: p.evolvedThisTurn,
		Graveyard: instanceIDs(p.graveyard), Banished: instanceIDs(p.banished), Destroyed: instanceIDs(p.destroyed),
	}
}

func restoreGame(cards map[int]*ir.Card, saved ContinuationGame) (*game, error) {
	g := &game{cards: cards, instances: map[string]*instance{}, legal: saved.Legal, illegal: saved.Illegal, unchanged: saved.Unchanged, rng: ruleset.NewRNG(0), serial: saved.Serial, eventSequence: saved.EventSequence, deathBatchSerial: saved.DeathBatchSerial, revision: saved.Revision, turn: saved.Turn, phase: saved.Phase, turnTransition: saved.TurnTransition, gameOver: saved.GameOver, winner: saved.Winner}
	if saved.Serial < 0 || saved.Serial < generatedInstanceSerial(saved.Instances) {
		return nil, fmt.Errorf("invalid continuation instance serial")
	}
	validTransition := saved.TurnTransition == "" && saved.Turn.Active == "own" || saved.TurnTransition == "ending" && saved.Turn.Active == "own" || saved.TurnTransition == "starting" && saved.Turn.Active == "oppo"
	if !validTransition || saved.Turn.Number < 0 || saved.Phase != "main" {
		return nil, fmt.Errorf("invalid continuation turn state")
	}
	if saved.GameOver != (saved.Winner != "") || saved.Winner != "" && saved.Winner != "own" && saved.Winner != "oppo" {
		return nil, fmt.Errorf("invalid continuation game result")
	}
	for _, entity := range saved.Instances {
		card := cards[entity.CardID]
		if entity.ID == "" || card == nil || g.instances[entity.ID] != nil || !validZone(entity.Zone) {
			return nil, fmt.Errorf("invalid continuation entity %q", entity.ID)
		}
		limit := entity.AttackLimit
		if limit == 0 {
			limit = 1
		}
		if limit < 1 || entity.AttacksUsed < 0 || entity.AttacksUsed > limit ||
			entity.AttacksUsed > 0 && (entity.Zone != "field" || card.CardType != "follower") ||
			entity.SummoningSick && (entity.Zone != "field" || card.CardType != "follower" || entity.AttacksUsed != 0) {
			return nil, fmt.Errorf("invalid continuation combat state")
		}
		i := &instance{
			id: entity.ID, alias: entity.Alias, zone: entity.Zone, card: card,
			attack: entity.Attack, life: entity.Life, earthsigil: entity.Earthsigil, countdown: entity.Countdown,
			attacksUsed: entity.AttacksUsed, attackLimitValue: limit, engaged: entity.Engaged, summoningSick: entity.SummoningSick,
			evolved: entity.Evolved, superEvolved: entity.SuperEvolved, damageReduction: entity.DamageReduction, abilities: map[string]bool{},
		}
		for _, ability := range entity.Abilities {
			if ability == "" || i.abilities[ability] {
				return nil, fmt.Errorf("invalid continuation ability")
			}
			i.abilities[ability] = true
		}
		g.instances[i.id] = i
	}
	for _, entity := range saved.Instances {
		source := g.instances[entity.ID]
		for _, materialID := range entity.Materials {
			material := g.instances[materialID]
			if material == nil || material == source || material.zone != "hand" || contains(source.materials, material) {
				return nil, fmt.Errorf("invalid continuation fusion material")
			}
			source.materials = append(source.materials, material)
		}
	}
	var err error
	if g.own, err = restorePlayer(saved.Own, g.instances); err != nil {
		return nil, err
	}
	if g.oppo, err = restorePlayer(saved.Oppo, g.instances); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	zoneOwner := map[string]string{}
	for _, side := range []struct {
		name  string
		zones [][]*instance
	}{{"own", [][]*instance{g.own.deck, g.own.hand, g.own.field, g.own.graveyard, g.own.banished}}, {"oppo", [][]*instance{g.oppo.deck, g.oppo.hand, g.oppo.field, g.oppo.graveyard, g.oppo.banished}}} {
		for _, zone := range side.zones {
			for _, i := range zone {
				if seen[i.id] {
					return nil, fmt.Errorf("continuation entity appears in multiple zones")
				}
				seen[i.id] = true
				zoneOwner[i.id] = side.name
			}
		}
	}
	historyOnly := map[string]bool{}
	for _, side := range []struct {
		name    string
		history []*instance
	}{{"own", g.own.destroyed}, {"oppo", g.oppo.destroyed}} {
		historySeen := map[string]bool{}
		for _, i := range side.history {
			if historySeen[i.id] || i.zone != "destroyed" && zoneOwner[i.id] != side.name {
				return nil, fmt.Errorf("invalid continuation destroyed history owner")
			}
			historySeen[i.id] = true
			if i.zone == "destroyed" {
				if historyOnly[i.id] {
					return nil, fmt.Errorf("continuation history entity has multiple owners")
				}
				historyOnly[i.id] = true
			}
		}
	}
	for id, i := range g.instances {
		if !seen[id] && (i.zone != "destroyed" || !historyOnly[id]) {
			return nil, fmt.Errorf("continuation contains an unzoned entity")
		}
	}
	for _, event := range saved.Events {
		g.events = append(g.events, ir.RuntimeEvent{Kind: event.Kind, Side: event.Side, InstanceID: event.InstanceID, CardID: event.CardID, Count: event.Count, Actual: event.Actual, Target: cloneEventTarget(event.Target), Subject: cloneEventTarget(event.Subject), Attacker: cloneEventTarget(event.Attacker), Defender: cloneEventTarget(event.Defender), Sequence: event.Sequence, BatchID: event.BatchID})
	}
	destroyedHistory := map[string]bool{}
	for _, i := range append(append([]*instance{}, g.own.destroyed...), g.oppo.destroyed...) {
		destroyedHistory[i.id] = true
	}
	if err := validateContinuationEvents(g.events, saved.EventSequence, saved.DeathBatchSerial, g.instances, destroyedHistory); err != nil {
		return nil, err
	}
	g.rng.Restore(ruleset.RNGState{State: saved.RNG.State, Consumed: saved.RNG.Consumed})
	if saved.Attack != nil {
		if saved.Attack.Stage != "attack" && saved.Attack.Stage != "combat" || saved.Attack.Actor != "own" && saved.Attack.Actor != "oppo" || saved.Attack.Attacker == "" || saved.Attack.AttackerAttack < 0 || saved.Attack.DefenderAttack < 0 {
			return nil, fmt.Errorf("invalid continuation attack state")
		}
		attacker := g.instances[saved.Attack.Attacker]
		if attacker == nil || attacker.zone != "field" || attacker.card.CardType != "follower" || !contains(g.player(saved.Attack.Actor).field, attacker) || saved.Attack.Actor != g.turn.Active {
			return nil, fmt.Errorf("invalid continuation attack state")
		}
		if saved.Attack.Defender != "" {
			defender := g.instances[saved.Attack.Defender]
			if defender == nil || defender.zone != "field" || defender.card.CardType != "follower" || !contains(g.player(oppositeSide(saved.Attack.Actor)).field, defender) {
				return nil, fmt.Errorf("invalid continuation attack state")
			}
		}
		g.attack = &attackState{stage: saved.Attack.Stage, actor: saved.Attack.Actor, attacker: saved.Attack.Attacker, defender: saved.Attack.Defender, attackerAttack: saved.Attack.AttackerAttack, defenderAttack: saved.Attack.DefenderAttack}
	}
	g.rebuildTriggerIndex()
	return g, nil
}

func validateContinuationEvents(events []ir.RuntimeEvent, sequence, deathBatchSerial uint64, instances map[string]*instance, destroyedHistory map[string]bool) error {
	// 恢复时逐项核对，不能让篡改后的批次号进入后续结算。
	var lastBatch uint64
	previousBatch := uint64(0)
	for n, event := range events {
		if event.Sequence != uint64(n+1) || event.BatchID > deathBatchSerial || event.Kind == "destroyed" && event.BatchID == 0 || event.Kind != "destroyed" && event.BatchID != 0 {
			return fmt.Errorf("invalid continuation event sequence")
		}
		if event.Kind == "destroyed" {
			if event.Subject == nil || event.Subject.Kind != "instance" || instances[event.Subject.InstanceID] == nil || !destroyedHistory[event.Subject.InstanceID] {
				return fmt.Errorf("invalid continuation death subject")
			}
			if event.BatchID == lastBatch {
				if previousBatch != event.BatchID {
					return fmt.Errorf("invalid continuation death batch")
				}
			} else if event.BatchID != lastBatch+1 {
				return fmt.Errorf("invalid continuation death batch")
			}
			lastBatch = event.BatchID
		}
		previousBatch = event.BatchID
	}
	if uint64(len(events)) != sequence || lastBatch != deathBatchSerial {
		return fmt.Errorf("invalid continuation event sequence")
	}
	return nil
}

func generatedInstanceSerial(instances []ContinuationEntity) int {
	maximum := 0
	for _, entity := range instances {
		for _, prefix := range []string{"added-", "summoned-"} {
			if !strings.HasPrefix(entity.ID, prefix) {
				continue
			}
			n, err := strconv.Atoi(strings.TrimPrefix(entity.ID, prefix))
			if err == nil && n > maximum {
				maximum = n
			}
		}
	}
	return maximum
}

func restorePlayer(saved ContinuationPlayer, instances map[string]*instance) (player, error) {
	p := player{pp: saved.PP, maxpp: saved.MaxPP, leaderLife: saved.LeaderLife, leaderMax: saved.LeaderMax, ep: saved.EP, sep: saved.SEP, combo: saved.Combo, shadows: saved.Shadows, attackedThisTurn: saved.AttackedThisTurn, evolvedThisTurn: saved.EvolvedThisTurn}
	var err error
	if p.deck, err = restoreInstanceList(saved.Deck, instances, "deck"); err != nil {
		return player{}, err
	}
	if p.hand, err = restoreInstanceList(saved.Hand, instances, "hand"); err != nil {
		return player{}, err
	}
	if p.field, err = restoreInstanceList(saved.Field, instances, "field"); err != nil {
		return player{}, err
	}
	if p.graveyard, err = restoreInstanceList(saved.Graveyard, instances, "graveyard"); err != nil {
		return player{}, err
	}
	if p.banished, err = restoreInstanceList(saved.Banished, instances, "banished"); err != nil {
		return player{}, err
	}
	if p.destroyed, err = restoreHistory(saved.Destroyed, instances); err != nil {
		return player{}, err
	}
	return p, nil
}

func restoreInstanceList(ids []string, instances map[string]*instance, zone string) ([]*instance, error) {
	items := make([]*instance, 0, len(ids))
	seen := map[string]bool{}
	for _, id := range ids {
		i := instances[id]
		if i == nil || i.zone != zone || seen[id] {
			return nil, fmt.Errorf("invalid continuation %s zone", zone)
		}
		seen[id] = true
		items = append(items, i)
	}
	return items, nil
}

func restoreHistory(ids []string, instances map[string]*instance) ([]*instance, error) {
	items := make([]*instance, 0, len(ids))
	for _, id := range ids {
		if instances[id] == nil {
			return nil, fmt.Errorf("invalid continuation destroyed history")
		}
		items = append(items, instances[id])
	}
	return items, nil
}

func restoreBindings(saved []ContinuationBindings, instances map[string]*instance) (map[string]frame, error) {
	result := map[string]frame{}
	for _, bindingFrame := range saved {
		if bindingFrame.ID == "" || result[bindingFrame.ID] != nil || bindingFrame.Values == nil {
			return nil, fmt.Errorf("invalid continuation binding frame")
		}
		f := frame{}
		for name, ids := range bindingFrame.Values {
			if name == "" {
				return nil, fmt.Errorf("invalid continuation binding name")
			}
			for _, id := range ids {
				if instances[id] == nil {
					return nil, fmt.Errorf("unknown bound instance %q", id)
				}
				f[name] = append(f[name], instances[id])
			}
		}
		result[bindingFrame.ID] = f
	}
	return result, nil
}

func validateChoiceRequest(request ChoiceRequest, instances map[string]*instance) error {
	if !validRuntimeID(request.RequestID) || !validRuntimeID(request.ActionID) || request.NodeID == "" || request.PublicTo != "own" && request.PublicTo != "oppo" || request.MinSelections < 0 || request.MaxSelections < request.MinSelections || request.MaxSelections > len(request.Candidates) {
		return fmt.Errorf("invalid continuation choice request")
	}
	seenEntities, seenOptions := map[string]bool{}, map[int]bool{}
	for _, candidate := range request.Candidates {
		switch candidate.Kind {
		case "entity":
			if request.Kind != "target" {
				return fmt.Errorf("entity candidate on non-target request")
			}
			if candidate.InstanceID == "" || instances[candidate.InstanceID] == nil || candidate.OptionID != 0 || seenEntities[candidate.InstanceID] {
				return fmt.Errorf("invalid continuation entity candidate")
			}
			seenEntities[candidate.InstanceID] = true
		case "option":
			if request.Kind != "mode" {
				return fmt.Errorf("option candidate on non-mode request")
			}
			if candidate.InstanceID != "" || candidate.OptionID == 0 || seenOptions[candidate.OptionID] {
				return fmt.Errorf("invalid continuation option candidate")
			}
			seenOptions[candidate.OptionID] = true
		default:
			return fmt.Errorf("invalid continuation candidate kind")
		}
	}
	return nil
}

func validatePendingNode(s *Session, pending *pendingChoice) error {
	if len(s.stack) == 0 {
		return fmt.Errorf("continuation request has no execution frame")
	}
	top := s.stack[len(s.stack)-1]
	if top.pc == 0 || top.pc > len(top.body) || top.self != pending.self || reflect.ValueOf(top.bindings).Pointer() != reflect.ValueOf(pending.bindings).Pointer() {
		return fmt.Errorf("continuation request is detached from the execution frame")
	}
	if pending.request.PublicTo != s.g.sideOf(pending.self) {
		return fmt.Errorf("continuation request has the wrong controller")
	}
	effect := top.body[top.pc-1]
	if ir.EffectBase(effect).ID != pending.request.NodeID {
		return fmt.Errorf("continuation request node does not match the execution frame")
	}
	switch node := effect.(type) {
	case ir.SelectionEffect:
		minimum := 0
		if node.Kind == "require" {
			minimum = 1
		}
		if pending.request.Kind != "target" || node.Kind != "choose" && node.Kind != "require" || pending.binding != node.Binding || pending.request.MinSelections != minimum || pending.request.MaxSelections != 1 {
			return fmt.Errorf("continuation target request does not match its IR node")
		}
		candidates := s.g.fromRef(node.Source, pending.self, pending.bindings)
		if len(candidates) != len(pending.request.Candidates) {
			return fmt.Errorf("continuation target candidates changed")
		}
		for n, candidate := range candidates {
			if pending.request.Candidates[n].Kind != "entity" || pending.request.Candidates[n].InstanceID != candidate.id {
				return fmt.Errorf("continuation target candidates changed")
			}
		}
	case ir.ModeEffect:
		if pending.request.Kind != "mode" || pending.request.MinSelections != 1 || pending.request.MaxSelections != 1 || len(node.Options) != len(pending.request.Candidates) || len(node.Options) != len(pending.options) {
			return fmt.Errorf("continuation mode request does not match its IR node")
		}
		for n, option := range node.Options {
			candidate := pending.request.Candidates[n]
			blockID := nestedBlockID(node.ID, fmt.Sprintf("option:%d", option.ID))
			if candidate.Kind != "option" || candidate.OptionID != option.ID || pending.optionBlockIDs[option.ID] != blockID {
				return fmt.Errorf("continuation mode options changed")
			}
		}
	default:
		return fmt.Errorf("continuation request points to a non-choice node")
	}
	return nil
}

func indexBlocks(cards map[int]*ir.Card) (map[string][]ir.Effect, error) {
	blocks := map[string][]ir.Effect{}
	cardIDs := make([]int, 0, len(cards))
	for id := range cards {
		cardIDs = append(cardIDs, id)
	}
	sort.Ints(cardIDs)
	for _, id := range cardIDs {
		card := cards[id]
		if err := addBlock(blocks, cardPlayBlockID(id), card.PlayEffects); err != nil {
			return nil, err
		}
		for _, ability := range card.Abilities {
			if ability.ID == "" {
				return nil, fmt.Errorf("card %d has an unstable ability ID", id)
			}
			if err := addBlock(blocks, abilityBlockID(id, ability.ID), ability.Body); err != nil {
				return nil, err
			}
		}
		for _, ability := range card.FusionAbilities {
			if ability.ID == "" {
				return nil, fmt.Errorf("card %d has an unstable fusion ability ID", id)
			}
			if err := addBlock(blocks, fusionBlockID(id, ability.ID), ability.Body); err != nil {
				return nil, err
			}
		}
	}
	return blocks, nil
}

func addBlock(blocks map[string][]ir.Effect, id string, body []ir.Effect) error {
	if _, exists := blocks[id]; exists {
		return fmt.Errorf("duplicate continuation block %q", id)
	}
	blocks[id] = body
	for _, effect := range body {
		nodeID := ir.EffectBase(effect).ID
		switch e := effect.(type) {
		case ir.IfEffect:
			if nodeID == "" {
				return fmt.Errorf("control block has an unstable node ID")
			}
			if err := addBlock(blocks, nestedBlockID(nodeID, "then"), e.Then); err != nil {
				return err
			}
			if err := addBlock(blocks, nestedBlockID(nodeID, "else"), e.Else); err != nil {
				return err
			}
		case ir.ModeEffect:
			if nodeID == "" {
				return fmt.Errorf("mode block has an unstable node ID")
			}
			for _, option := range e.Options {
				if err := addBlock(blocks, nestedBlockID(nodeID, fmt.Sprintf("option:%d", option.ID)), option.Body); err != nil {
					return err
				}
			}
		case ir.PayResourceEffect:
			if nodeID == "" {
				return fmt.Errorf("payment block has an unstable node ID")
			}
			if err := addBlock(blocks, nestedBlockID(nodeID, "onPaid"), e.OnPaid); err != nil {
				return err
			}
		}
	}
	return nil
}

func cardPlayBlockID(cardID int) string { return fmt.Sprintf("card:%d:play", cardID) }
func abilityBlockID(cardID int, abilityID string) string {
	return fmt.Sprintf("card:%d:ability:%s", cardID, abilityID)
}
func fusionBlockID(cardID int, abilityID string) string {
	return fmt.Sprintf("card:%d:fusion:%s", cardID, abilityID)
}
func nestedBlockID(nodeID, branch string) string { return "node:" + nodeID + ":" + branch }

func runtimeCardPackHash(pack *ir.CardPack, cards map[int]*ir.Card) (string, error) {
	var value any
	if pack != nil {
		copy := *pack
		copy.ContentHash = ""
		value = copy
	} else {
		ids := make([]int, 0, len(cards))
		for id := range cards {
			ids = append(ids, id)
		}
		sort.Ints(ids)
		ordered := make([]ir.Card, 0, len(ids))
		for _, id := range ids {
			ordered = append(ordered, *cards[id])
		}
		value = ordered
	}
	data, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("hash runtime card pack: %w", err)
	}
	hash := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(hash[:]), nil
}

func rejectDuplicateContinuationKeys(data []byte) error {
	d := json.NewDecoder(bytes.NewReader(data))
	var walk func() error
	walk = func() error {
		token, err := d.Token()
		if err != nil {
			return err
		}
		delim, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		switch delim {
		case '{':
			seen := map[string]bool{}
			for d.More() {
				keyToken, err := d.Token()
				if err != nil {
					return err
				}
				key, ok := keyToken.(string)
				if !ok || seen[key] {
					return fmt.Errorf("duplicate continuation object key %q", key)
				}
				seen[key] = true
				if err := walk(); err != nil {
					return err
				}
			}
			_, err = d.Token()
			return err
		case '[':
			for d.More() {
				if err := walk(); err != nil {
					return err
				}
			}
			_, err = d.Token()
			return err
		default:
			return fmt.Errorf("unexpected JSON delimiter")
		}
	}
	if err := walk(); err != nil {
		return err
	}
	return nil
}

func continuationInstance(instances map[string]*instance, id string) (*instance, error) {
	if id == "" {
		return nil, nil
	}
	if instances[id] == nil {
		return nil, fmt.Errorf("unknown continuation instance %q", id)
	}
	return instances[id], nil
}

func instanceID(i *instance) string {
	if i == nil {
		return ""
	}
	return i.id
}
func instanceIDs(items []*instance) []string {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.id)
	}
	return ids
}
func cloneEventTarget(target *ir.EventTarget) *ir.EventTarget {
	if target == nil {
		return nil
	}
	copy := *target
	return &copy
}
func validZone(zone string) bool {
	switch zone {
	case "deck", "hand", "field", "graveyard", "banished", "destroyed":
		return true
	}
	return false
}
