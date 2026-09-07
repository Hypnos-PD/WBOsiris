package runner

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"wbo/internal/ir"
	"wbo/internal/ruleset"
)

const continuationVersion = "0.29.0"

type ContinuationBindings struct {
	ID     string                      `json:"id"`
	Values map[string][]ir.EventTarget `json:"values"`
}

type ContinuationPending struct {
	Request         ChoiceRequest        `json:"request"`
	Binding         string               `json:"binding,omitempty"`
	BindingFrameID  string               `json:"bindingFrameId"`
	SelfInstanceID  string               `json:"selfInstanceId,omitempty"`
	OptionBlockIDs  []ContinuationOption `json:"optionBlockIds"`
	FusionSourceID  string               `json:"fusionSourceId,omitempty"`
	FusionAbilityID string               `json:"fusionAbilityId,omitempty"`
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
	FirstPlayer      string               `json:"firstPlayer,omitempty"`
	Phase            string               `json:"phase"`
	TurnTransition   string               `json:"turnTransition"`
	EndingSide       string               `json:"endingSide,omitempty"`
	GameOver         bool                 `json:"gameOver"`
	Winner           string               `json:"winner,omitempty"`
	Attack           *ContinuationAttack  `json:"attack,omitempty"`
}

type ContinuationAttack struct {
	DefenderDestroyed bool   `json:"defenderDestroyed"`
	Stage             string `json:"stage"`
	Actor             string `json:"actor"`
	Attacker          string `json:"attacker"`
	Defender          string `json:"defender,omitempty"`
	AttackerAttack    int    `json:"attackerAttack,omitempty"`
	DefenderAttack    int    `json:"defenderAttack,omitempty"`
}

type ContinuationPlayer struct {
	Crests           []string            `json:"crests"`
	RetiredCrests    []string            `json:"retiredCrests"`
	PP               int                 `json:"pp"`
	MaxPP            int                 `json:"maxpp"`
	LeaderLife       int                 `json:"leaderLife"`
	LeaderMax        int                 `json:"leaderMax"`
	EP               int                 `json:"ep"`
	SEP              int                 `json:"sep"`
	Combo            int                 `json:"combo"`
	Shadows          int                 `json:"shadows"`
	AttackedThisTurn bool                `json:"attackedThisTurn"`
	EvolvedThisTurn  bool                `json:"evolvedThisTurn"`
	ExtraPPEarly     bool                `json:"extraPPEarly"`
	ExtraPPLate      bool                `json:"extraPPLate"`
	ExtraPPActive    bool                `json:"extraPPActive"`
	Deck             []string            `json:"deck"`
	Hand             []string            `json:"hand"`
	Field            []string            `json:"field"`
	Graveyard        []string            `json:"graveyard"`
	Resolving        []string            `json:"resolving"`
	Banished         []string            `json:"banished"`
	Destroyed        []DestructionRecord `json:"destroyed"`
}

type ContinuationEntity struct {
	Crest             bool                     `json:"crest,omitempty"`
	UsedTriggers      map[string]bool          `json:"usedTriggers,omitempty"`
	Grants            []string                 `json:"grants,omitempty"`
	Counters          map[string]int           `json:"counters,omitempty"`
	FusedThisTurn     bool                     `json:"fusedThisTurn"`
	ID                string                   `json:"id"`
	Alias             string                   `json:"alias"`
	Zone              string                   `json:"zone"`
	CardID            int                      `json:"cardId"`
	Cost              int                      `json:"cost"`
	Attack            int                      `json:"attack"`
	Life              int                      `json:"life"`
	Earthsigil        int                      `json:"earthsigil"`
	DamageReduction   int                      `json:"damageReduction"`
	Countdown         int                      `json:"countdown"`
	AttacksUsed       int                      `json:"attacksUsed"`
	AttackLimit       int                      `json:"attackLimit"`
	Engaged           bool                     `json:"engaged"`
	SummoningSick     bool                     `json:"summoningSick"`
	Evolved           bool                     `json:"evolved"`
	SuperEvolved      bool                     `json:"superEvolved"`
	Departed          bool                     `json:"departed,omitempty"`
	Abilities         []string                 `json:"abilities"`
	TemporaryKeywords map[string]KeywordExpiry `json:"temporaryKeywords,omitempty"`
	TemporaryStats    map[string]ir.Stats      `json:"temporaryStats,omitempty"`
	Materials         []string                 `json:"materials,omitempty"`
}

type ContinuationEvent struct {
	PrivateTo  string          `json:"privateTo,omitempty"`
	Kind       string          `json:"kind"`
	Side       string          `json:"side,omitempty"`
	InstanceID string          `json:"instanceId,omitempty"`
	From       string          `json:"from,omitempty"`
	To         string          `json:"to,omitempty"`
	Reason     string          `json:"reason,omitempty"`
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
	for index, saved := range c.Stack {
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
		var repeatBindings frame
		if strings.HasSuffix(saved.BlockID, ":repeat") {
			var exists bool
			repeatBindings, exists = bindings[saved.RepeatBindingsID]
			if saved.RepeatRemaining <= 0 || !exists || saved.RepeatBindingsID == saved.BindingFrameID {
				return nil, fmt.Errorf("invalid continuation repeat checkpoint")
			}
			if index == 0 {
				return nil, fmt.Errorf("repeat checkpoint has no caller")
			}
			caller := s.stack[index-1]
			if caller.pc < 1 || caller.pc > len(caller.body) {
				return nil, fmt.Errorf("invalid repeat caller position")
			}
			repeat, ok := caller.body[caller.pc-1].(ir.RepeatEffect)
			if !ok || nestedBlockID(repeat.ID, "repeat") != saved.BlockID || caller.self != self ||
				c.Stack[index-1].BindingFrameID != saved.RepeatBindingsID || repeat.TimesExpr == nil && saved.RepeatRemaining > repeat.Times {
				return nil, fmt.Errorf("repeat checkpoint differs from caller")
			}
		} else if saved.RepeatRemaining != 0 || saved.RepeatBindingsID != "" {
			return nil, fmt.Errorf("repeat checkpoint on a non-repeat block")
		}
		s.stack = append(s.stack, execFrame{body: body, blockID: saved.BlockID, pc: saved.PC, self: self, bindings: bindingFrame,
			repeatRemaining: saved.RepeatRemaining, repeatBindings: repeatBindings})
	}
	if c.TriggerBase < 0 || c.TriggerBase > len(s.stack) || c.DrainingTrigger && c.TriggerBase == len(s.stack) {
		return nil, fmt.Errorf("invalid continuation trigger checkpoint")
	}
	if len(g.own.resolving)+len(g.oppo.resolving) != 0 {
		if !c.DrainingTrigger || c.TriggerBase != 0 || len(s.stack) == 0 || s.stack[0].self != g.player(g.turn.Active).resolving[0] {
			return nil, fmt.Errorf("invalid continuation spell checkpoint")
		}
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
	pending := &pendingChoice{request: cloneChoiceRequest(c.Pending.Request), binding: c.Pending.Binding, bindings: pendingBindings, self: pendingSelf, options: map[int][]ir.Effect{}, optionBlockIDs: map[int]string{}}
	if pending.request.Kind == "fusion_material" {
		source := g.instances[c.Pending.FusionSourceID]
		if source == nil || source != pendingSelf {
			return nil, fmt.Errorf("invalid fusion continuation source")
		}
		for n := range source.card.FusionAbilities {
			if source.card.FusionAbilities[n].ID == c.Pending.FusionAbilityID {
				pending.fusionSource = source
				pending.fusion = &source.card.FusionAbilities[n]
				break
			}
		}
		if pending.fusion == nil {
			return nil, fmt.Errorf("invalid fusion continuation ability")
		}
	}
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
	} else if pending.request.Kind == "fusion_material" {
		if pending.fusion == nil || pending.fusionSource == nil || pending.binding != "" || len(pending.options) != 0 {
			return nil, fmt.Errorf("invalid fusion continuation")
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
		if self != nil {
			for ability := range self.triggeredAbilities() {
				if trigger, ok := ability.Trigger.(ir.EventTrigger); ok && ability.blockID == saved.BlockID && trigger.OncePerTurn != "" && !self.usedTriggers[ability.ID] {
					return nil, fmt.Errorf("queued continuation trigger has no recorded usage")
				}
			}
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
		values := map[string][]ir.EventTarget{}
		for name, targets := range f {
			values[name] = append([]ir.EventTarget(nil), targets...)
		}
		c.BindingFrames = append(c.BindingFrames, ContinuationBindings{ID: id, Values: values})
		return id
	}
	for _, f := range s.stack {
		saved := ContinuationFrame{BlockID: f.blockID, PC: f.pc, SelfInstanceID: instanceID(f.self), BindingFrameID: addBindings(f.bindings), RepeatRemaining: f.repeatRemaining}
		if f.repeatRemaining > 0 {
			saved.RepeatBindingsID = addBindings(f.repeatBindings)
		}
		c.Stack = append(c.Stack, saved)
	}
	c.Pending = ContinuationPending{Request: *s.PendingChoice(), Binding: s.pending.binding, BindingFrameID: addBindings(s.pending.bindings), SelfInstanceID: instanceID(s.pending.self)}
	if s.pending.request.Kind == "fusion_material" {
		c.Pending.FusionSourceID = instanceID(s.pending.fusionSource)
		c.Pending.FusionAbilityID = s.pending.fusion.ID
	}
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
		Serial: g.serial, EventSequence: g.eventSequence, DeathBatchSerial: g.deathBatchSerial, Revision: g.revision, Legal: g.legal, Illegal: g.illegal, Unchanged: g.unchanged, Turn: g.turn, FirstPlayer: g.firstPlayer, Phase: g.phase, TurnTransition: g.turnTransition, EndingSide: g.endingSide, GameOver: g.gameOver, Winner: g.winner,
	}
	if g.attack != nil {
		snapshot.Attack = &ContinuationAttack{Stage: g.attack.stage, Actor: g.attack.actor, Attacker: g.attack.attacker, Defender: g.attack.defender, AttackerAttack: g.attack.attackerAttack, DefenderAttack: g.attack.defenderAttack, DefenderDestroyed: g.attack.defenderDestroyed}
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
			Crest:             i.card.CardType == "crest",
			Counters:          maps.Clone(i.counters),
			UsedTriggers:      maps.Clone(i.usedTriggers),
			TemporaryKeywords: maps.Clone(i.temporaryKeywords),
			TemporaryStats:    maps.Clone(i.temporaryStats),
			ID:                i.id, Alias: i.alias, Zone: i.zone, CardID: i.card.ID, Cost: i.cost, Attack: i.attack, Life: i.life,
			Earthsigil: i.earthsigil, Countdown: i.countdown, AttacksUsed: i.attacksUsed, AttackLimit: attackLimit(i),
			Engaged: i.engaged, SummoningSick: i.summoningSick, Evolved: i.evolved,
			SuperEvolved: i.superEvolved, Departed: i.departed, FusedThisTurn: i.fusedThisTurn, DamageReduction: i.damageReduction, Abilities: abilities, Materials: instanceIDs(i.materials), Grants: grantIDs(i),
		})
	}
	for _, event := range g.events {
		snapshot.Events = append(snapshot.Events, ContinuationEvent{PrivateTo: event.PrivateTo, Kind: event.Kind, Side: event.Side, InstanceID: event.InstanceID, From: event.From, To: event.To, Reason: event.Reason, CardID: event.CardID, Count: event.Count, Actual: event.Actual, Target: cloneEventTarget(event.Target), Subject: cloneEventTarget(event.Subject), Attacker: cloneEventTarget(event.Attacker), Defender: cloneEventTarget(event.Defender), Sequence: event.Sequence, BatchID: event.BatchID})
	}
	return snapshot
}

func snapshotContinuationPlayer(p player) ContinuationPlayer {
	return ContinuationPlayer{
		Crests: instanceIDs(p.crests), RetiredCrests: instanceIDs(p.retiredCrests),
		PP: p.pp, MaxPP: p.maxpp, LeaderLife: p.leaderLife, LeaderMax: p.leaderMax,
		EP: p.ep, SEP: p.sep, Combo: p.combo, Shadows: p.shadows, AttackedThisTurn: p.attackedThisTurn,
		Deck: instanceIDs(p.deck), Hand: instanceIDs(p.hand), Field: instanceIDs(p.field), EvolvedThisTurn: p.evolvedThisTurn,
		Graveyard: instanceIDs(p.graveyard), Banished: instanceIDs(p.banished), Destroyed: cloneDestructionHistory(p.destroyed),
		Resolving:    instanceIDs(p.resolving),
		ExtraPPEarly: p.extraPPEarly, ExtraPPLate: p.extraPPLate, ExtraPPActive: p.extraPPActive,
	}
}

func restoreGame(cards map[int]*ir.Card, saved ContinuationGame) (*game, error) {
	grants, err := indexGrants(cards)
	if err != nil {
		return nil, err
	}
	g := &game{cards: cards, instances: map[string]*instance{}, legal: saved.Legal, illegal: saved.Illegal, unchanged: saved.Unchanged, rng: ruleset.NewRNG(0), serial: saved.Serial, eventSequence: saved.EventSequence, deathBatchSerial: saved.DeathBatchSerial, revision: saved.Revision, turn: saved.Turn, firstPlayer: saved.FirstPlayer, phase: saved.Phase, turnTransition: saved.TurnTransition, endingSide: saved.EndingSide, gameOver: saved.GameOver, winner: saved.Winner}
	if saved.Serial < 0 || saved.Serial < generatedInstanceSerial(saved.Instances) {
		return nil, fmt.Errorf("invalid continuation instance serial")
	}
	validTransition := saved.Turn.Active == "own" || saved.Turn.Active == "oppo"
	if saved.TurnTransition != "" && saved.TurnTransition != "ending" && saved.TurnTransition != "starting" && saved.TurnTransition != "starting_triggers" && saved.TurnTransition != "starting_crests" && saved.TurnTransition != "starting_draw" {
		validTransition = false
	}
	if saved.EndingSide != "" && saved.EndingSide != "own" && saved.EndingSide != "oppo" ||
		saved.TurnTransition == "ending" && saved.EndingSide != saved.Turn.Active ||
		saved.EndingSide != "" && saved.TurnTransition != "" && saved.TurnTransition != "ending" {
		validTransition = false
	}
	if !validTransition || saved.Turn.Number < 0 || saved.Phase != "main" {
		return nil, fmt.Errorf("invalid continuation turn state")
	}
	if saved.GameOver != (saved.Winner != "") || saved.Winner != "" && saved.Winner != "own" && saved.Winner != "oppo" {
		return nil, fmt.Errorf("invalid continuation game result")
	}
	for _, entity := range saved.Instances {
		card := cards[entity.CardID]
		if card != nil && entity.Crest {
			card = card.CrestCard()
		}
		if entity.ID == "" || card == nil || g.instances[entity.ID] != nil || !validZone(entity.Zone) {
			return nil, fmt.Errorf("invalid continuation entity %q", entity.ID)
		}
		if entity.Crest != (entity.Zone == "crests" || entity.Zone == "retired_crest") || entity.Crest &&
			(entity.Cost != 0 || entity.Attack != 0 || entity.Life != 0 || entity.Evolved || entity.SuperEvolved || entity.Engaged || entity.FusedThisTurn || entity.Earthsigil != 0 || entity.DamageReduction != 0 || len(entity.Materials) > 0 || len(entity.Abilities) > 0 || len(entity.TemporaryStats) > 0 || entity.Countdown < 0 || entity.Countdown > cards[entity.CardID].Crest.Countdown || entity.Zone == "crests" && cards[entity.CardID].Crest.Countdown > 0 && entity.Countdown == 0) {
			return nil, fmt.Errorf("invalid continuation crest state")
		}
		if entity.Crest && (entity.AttackLimit != 1 || entity.Zone == "retired_crest" && (entity.Countdown != 0 || cards[entity.CardID].Crest.Countdown == 0)) {
			return nil, fmt.Errorf("invalid continuation crest lifetime")
		}
		limit := entity.AttackLimit
		if limit == 0 {
			limit = 1
		}
		if limit < 1 || entity.AttacksUsed < 0 || entity.AttacksUsed > limit ||
			entity.Cost < 0 || entity.Cost > 65535 || entity.Departed && card.CardType != "follower" ||
			entity.AttacksUsed > 0 && (entity.Zone != "field" || card.CardType != "follower") ||
			entity.SummoningSick && (entity.Zone != "field" || card.CardType != "follower" || entity.AttacksUsed != 0) {
			return nil, fmt.Errorf("invalid continuation combat state")
		}
		i := &instance{
			counters:     maps.Clone(entity.Counters),
			usedTriggers: maps.Clone(entity.UsedTriggers),
			id:           entity.ID, alias: entity.Alias, zone: entity.Zone, card: card,
			attack: entity.Attack, life: entity.Life, cost: entity.Cost, earthsigil: entity.Earthsigil, countdown: entity.Countdown,
			attacksUsed: entity.AttacksUsed, attackLimitValue: limit, engaged: entity.Engaged, summoningSick: entity.SummoningSick,
			evolved: entity.Evolved, superEvolved: entity.SuperEvolved, departed: entity.Departed, fusedThisTurn: entity.FusedThisTurn, damageReduction: entity.DamageReduction, abilities: map[string]bool{},
		}
		for _, id := range entity.Grants {
			grant, ok := grants[id]
			if !ok || card.CardType != "follower" {
				return nil, fmt.Errorf("invalid continuation grant")
			}
			i.grants = append(i.grants, grant)
		}
		for _, ability := range entity.Abilities {
			if ability == "" || i.abilities[ability] {
				return nil, fmt.Errorf("invalid continuation ability")
			}
			i.abilities[ability] = true
		}
		for keyword, expiry := range entity.TemporaryKeywords {
			if !ir.ValidKeyword(keyword) || !i.abilities[keyword] || !expiry.OwnTurnEnd && !expiry.OppoTurnEnd {
				return nil, fmt.Errorf("invalid temporary keyword")
			}
		}
		if len(entity.TemporaryKeywords) > 0 {
			i.temporaryKeywords = maps.Clone(entity.TemporaryKeywords)
		}
		for side, delta := range entity.TemporaryStats {
			if side != "own" && side != "oppo" || delta.Attack == 0 && delta.Life == 0 {
				return nil, fmt.Errorf("invalid temporary stats")
			}
		}
		if len(entity.TemporaryStats) > 0 {
			i.temporaryStats = maps.Clone(entity.TemporaryStats)
		}
		if !ir.ValidCounters(i.counters) || len(i.counters) != len(card.Counters) {
			return nil, fmt.Errorf("invalid continuation counters")
		}
		for name := range card.Counters {
			if _, ok := i.counters[name]; !ok {
				return nil, fmt.Errorf("missing continuation counter %q", name)
			}
		}
		g.instances[i.id] = i
		limited := map[string]bool{}
		for _, ability := range card.Abilities {
			if trigger, ok := ability.Trigger.(ir.EventTrigger); ok && trigger.OncePerTurn != "" {
				limited[ability.ID] = true
			}
		}
		for id, used := range i.usedTriggers {
			if !used || !limited[id] {
				return nil, fmt.Errorf("invalid continuation trigger usage")
			}
		}
	}
	for _, entity := range saved.Instances {
		source := g.instances[entity.ID]
		for _, materialID := range entity.Materials {
			material := g.instances[materialID]
			if material == nil || material == source || material.zone != "attached" || contains(source.materials, material) {
				return nil, fmt.Errorf("invalid continuation fusion material")
			}
			source.materials = append(source.materials, material)
		}
	}
	if g.own, err = restorePlayer(saved.Own, g.instances, cards); err != nil {
		return nil, err
	}
	if g.oppo, err = restorePlayer(saved.Oppo, g.instances, cards); err != nil {
		return nil, err
	}
	if len(g.own.resolving)+len(g.oppo.resolving) > 1 || len(g.player(oppositeSide(g.turn.Active)).resolving) != 0 {
		return nil, fmt.Errorf("invalid continuation spell owner")
	}
	for _, spell := range g.player(g.turn.Active).resolving {
		if spell.card.CardType != "spell" || saved.Attack != nil || saved.TurnTransition != "" || saved.GameOver {
			return nil, fmt.Errorf("invalid continuation resolving spell")
		}
	}
	seen := map[string]bool{}
	zoneOwner := map[string]string{}
	for _, side := range []struct {
		name  string
		zones [][]*instance
	}{{"own", [][]*instance{g.own.deck, g.own.hand, g.own.field, g.own.graveyard, g.own.banished, g.own.resolving, g.own.crests, g.own.retiredCrests}}, {"oppo", [][]*instance{g.oppo.deck, g.oppo.hand, g.oppo.field, g.oppo.graveyard, g.oppo.banished, g.oppo.resolving, g.oppo.crests, g.oppo.retiredCrests}}} {
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
	// Walk attachments from zoned roots; shared materials and cycles are invalid.
	var visitMaterials func(*instance, string) error
	visitMaterials = func(source *instance, side string) error {
		for _, material := range source.materials {
			if seen[material.id] {
				return fmt.Errorf("continuation material has multiple owners or a cycle")
			}
			seen[material.id], zoneOwner[material.id] = true, side
			if err := visitMaterials(material, side); err != nil {
				return err
			}
		}
		return nil
	}
	for _, entity := range saved.Instances {
		if entity.Zone != "attached" {
			if err := visitMaterials(g.instances[entity.ID], zoneOwner[entity.ID]); err != nil {
				return nil, err
			}
		}
	}
	historyOnly := map[string]string{}
	for _, side := range []struct {
		name    string
		history []DestructionRecord
	}{{"own", g.own.destroyed}, {"oppo", g.oppo.destroyed}} {
		for _, record := range side.history {
			i := g.instances[record.InstanceID]
			if i.zone != "destroyed" && zoneOwner[i.id] != side.name {
				return nil, fmt.Errorf("invalid continuation destroyed history owner")
			}
			if i.zone == "destroyed" {
				if owner := historyOnly[i.id]; owner != "" && owner != side.name {
					return nil, fmt.Errorf("continuation history entity has multiple owners")
				}
				historyOnly[i.id] = side.name
			}
		}
	}
	for id, i := range g.instances {
		if !seen[id] && (i.zone != "destroyed" || historyOnly[id] == "") {
			return nil, fmt.Errorf("continuation contains an unzoned entity")
		}
		owner := zoneOwner[id]
		if owner == "" {
			owner = historyOnly[id]
		}
		for _, ability := range i.card.Abilities {
			if trigger, ok := ability.Trigger.(ir.EventTrigger); ok && i.usedTriggers[ability.ID] && !triggerTurnMatches(trigger.OncePerTurn, owner, g.turn.Active) {
				return nil, fmt.Errorf("continuation trigger usage outside permitted turn")
			}
		}
	}
	for _, event := range saved.Events {
		if (event.Kind == "crest_gained" || event.Kind == "crest_countdown" || event.Kind == "crest_destroyed") && zoneOwner[event.InstanceID] != event.Side {
			return nil, fmt.Errorf("invalid continuation crest event owner")
		}
		g.events = append(g.events, ir.RuntimeEvent{PrivateTo: event.PrivateTo, Kind: event.Kind, Side: event.Side, InstanceID: event.InstanceID, From: event.From, To: event.To, Reason: event.Reason, CardID: event.CardID, Count: event.Count, Actual: event.Actual, Target: cloneEventTarget(event.Target), Subject: cloneEventTarget(event.Subject), Attacker: cloneEventTarget(event.Attacker), Defender: cloneEventTarget(event.Defender), Sequence: event.Sequence, BatchID: event.BatchID})
	}
	destroyedHistory := map[string]bool{}
	for _, record := range append(append([]DestructionRecord{}, g.own.destroyed...), g.oppo.destroyed...) {
		destroyedHistory[record.InstanceID] = true
	}
	if err := validateDestructionRecords(g); err != nil {
		return nil, err
	}
	if err := validateContinuationEvents(g.events, saved.EventSequence, saved.DeathBatchSerial, g.instances, destroyedHistory); err != nil {
		return nil, err
	}
	g.rng.Restore(ruleset.RNGState{State: saved.RNG.State, Consumed: saved.RNG.Consumed})
	if saved.Attack != nil {
		if saved.Attack.Stage != "attack" && saved.Attack.Stage != "clash_attacker" && saved.Attack.Stage != "clash_defender" && saved.Attack.Stage != "combat" || saved.Attack.Actor != "own" && saved.Attack.Actor != "oppo" || saved.Attack.Attacker == "" || saved.Attack.AttackerAttack < 0 || saved.Attack.DefenderAttack < 0 {
			return nil, fmt.Errorf("invalid continuation attack state")
		}
		attacker := g.instances[saved.Attack.Attacker]
		if attacker == nil || zoneOwner[attacker.id] != saved.Attack.Actor || saved.Attack.Actor != g.turn.Active {
			return nil, fmt.Errorf("invalid continuation attack state")
		}
		if saved.Attack.Defender != "" {
			defender := g.instances[saved.Attack.Defender]
			if defender == nil || zoneOwner[defender.id] != oppositeSide(saved.Attack.Actor) {
				return nil, fmt.Errorf("invalid continuation attack state")
			}
		}
		if saved.Attack.DefenderDestroyed && !destroyedHistory[saved.Attack.Defender] {
			return nil, fmt.Errorf("invalid continuation attack destruction")
		}
		// Participants may leave during an ability; validate against the committed attack.
		foundAttack, defenderDestroyed := false, false
		for n := len(g.events) - 1; n >= 0; n-- {
			event := g.events[n]
			if event.Kind == "destroyed" && event.Subject != nil && event.Subject.InstanceID == saved.Attack.Defender {
				defenderDestroyed = true
			}
			if event.Kind != "attacked" {
				continue
			}
			if event.Attacker == nil || event.Attacker.InstanceID != saved.Attack.Attacker || event.Defender == nil ||
				saved.Attack.Defender != "" && (event.Defender.Kind != "instance" || event.Defender.InstanceID != saved.Attack.Defender) ||
				saved.Attack.Defender == "" && (event.Defender.Kind != "leader" || event.Defender.Side != oppositeSide(saved.Attack.Actor)) {
				return nil, fmt.Errorf("invalid continuation attack event")
			}
			foundAttack = true
			break
		}
		if !foundAttack || defenderDestroyed != saved.Attack.DefenderDestroyed {
			return nil, fmt.Errorf("invalid continuation attack destruction")
		}
		g.attack = &attackState{stage: saved.Attack.Stage, actor: saved.Attack.Actor, attacker: saved.Attack.Attacker, defender: saved.Attack.Defender, attackerAttack: saved.Attack.AttackerAttack, defenderAttack: saved.Attack.DefenderAttack, defenderDestroyed: saved.Attack.DefenderDestroyed}
	}
	g.rebuildTriggerIndex()
	return g, nil
}

func validateContinuationEvents(events []ir.RuntimeEvent, sequence, deathBatchSerial uint64, instances map[string]*instance, destroyedHistory map[string]bool) error {
	// 恢复时逐项核对，不能让篡改后的批次号进入后续结算。
	var lastBatch uint64
	previousBatch := uint64(0)
	for n, event := range events {
		if event.Kind == "crest_gained" || event.Kind == "crest_countdown" || event.Kind == "crest_destroyed" {
			i := instances[event.InstanceID]
			if i == nil || i.card.CardType != "crest" || i.card.ID != event.CardID || event.Side != "own" && event.Side != "oppo" || event.PrivateTo != "" || event.Count < 0 || event.Kind == "crest_destroyed" && i.zone != "retired_crest" {
				return fmt.Errorf("invalid continuation crest event")
			}
		}
		if event.PrivateTo != "" && (event.PrivateTo != "own" && event.PrivateTo != "oppo" || event.PrivateTo != event.Side) {
			return fmt.Errorf("invalid continuation event visibility")
		}
		if event.Kind == "card_fused" || event.Kind == "card_transformed" {
			if event.Side != "own" && event.Side != "oppo" || event.Subject == nil || event.Subject.Kind != "instance" || instances[event.Subject.InstanceID] == nil || event.Subject.CardID < 10000000 {
				return fmt.Errorf("invalid continuation card event subject")
			}
			if event.Kind == "card_fused" && (event.PrivateTo != event.Side || event.Count < 1) {
				return fmt.Errorf("invalid continuation fusion event")
			}
			if event.Kind == "card_transformed" && (event.Target == nil || event.Target.Kind != "instance" || event.Target.InstanceID != event.Subject.InstanceID || event.Target.CardID < 10000000) {
				return fmt.Errorf("invalid continuation transform event")
			}
			if !validZone(event.From) || (event.From == "hand" || event.From == "deck" || event.From == "attached") && event.PrivateTo != event.Side {
				return fmt.Errorf("invalid continuation hidden card event")
			}
		}
		if event.Kind == "card_discarded" && (event.Side != "own" && event.Side != "oppo" || event.PrivateTo != "" || event.From != "hand" || event.To != "graveyard" || event.Subject == nil || event.Subject.Kind != "instance" || instances[event.Subject.InstanceID] == nil || event.Subject.CardID < 10000000) {
			return fmt.Errorf("invalid continuation discard event")
		}
		if event.Kind == "follower_left" && (event.Side != "own" && event.Side != "oppo" || event.PrivateTo != "" || event.From != "field" || event.To != "hand" && event.To != "deck" && event.To != "graveyard" && event.To != "banished" || event.Subject == nil || event.Subject.Kind != "instance" || instances[event.Subject.InstanceID] == nil || event.Subject.CardID < 10000000) {
			return fmt.Errorf("invalid continuation departure event")
		}
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
		for _, prefix := range []string{"added-", "summoned-", "crest-"} {
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

func restorePlayer(saved ContinuationPlayer, instances map[string]*instance, cards map[int]*ir.Card) (player, error) {
	p := player{pp: saved.PP, maxpp: saved.MaxPP, leaderLife: saved.LeaderLife, leaderMax: saved.LeaderMax, ep: saved.EP, sep: saved.SEP, combo: saved.Combo, shadows: saved.Shadows, attackedThisTurn: saved.AttackedThisTurn, evolvedThisTurn: saved.EvolvedThisTurn, extraPPEarly: saved.ExtraPPEarly, extraPPLate: saved.ExtraPPLate, extraPPActive: saved.ExtraPPActive}
	var err error
	if p.crests, err = restoreInstanceList(saved.Crests, instances, "crests"); err != nil {
		return player{}, err
	}
	if p.retiredCrests, err = restoreInstanceList(saved.RetiredCrests, instances, "retired_crest"); err != nil {
		return player{}, err
	}
	if len(p.crests) > crestLimit {
		return player{}, fmt.Errorf("too many continuation crests")
	}
	seenCrests := map[int]bool{}
	for _, crest := range p.crests {
		if seenCrests[crest.card.ID] {
			return player{}, fmt.Errorf("duplicate continuation crest")
		}
		seenCrests[crest.card.ID] = true
	}
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
	if p.resolving, err = restoreInstanceList(saved.Resolving, instances, "resolving"); err != nil {
		return player{}, err
	}
	if p.banished, err = restoreInstanceList(saved.Banished, instances, "banished"); err != nil {
		return player{}, err
	}
	if p.destroyed, err = restoreHistory(saved.Destroyed, instances, cards); err != nil {
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

func restoreHistory(records []DestructionRecord, instances map[string]*instance, cards map[int]*ir.Card) ([]DestructionRecord, error) {
	items := make([]DestructionRecord, 0, len(records))
	var previous uint64
	for _, record := range records {
		card := cards[record.CardID]
		if instances[record.InstanceID] == nil || card == nil || card.CardType != "follower" && card.CardType != "amulet" ||
			record.Cost < 0 || record.Cost > 65535 ||
			(record.Evolved || record.SuperEvolved || record.Departed) && card.CardType != "follower" ||
			previous > 0 && record.EventSequence <= previous {
			return nil, fmt.Errorf("invalid continuation destroyed history")
		}
		previous = record.EventSequence
		for n, keyword := range record.Keywords {
			if !ir.ValidKeyword(keyword) || n > 0 && keyword <= record.Keywords[n-1] {
				return nil, fmt.Errorf("invalid continuation history keywords")
			}
		}
		record.Keywords = append([]string(nil), record.Keywords...)
		items = append(items, record)
	}
	return items, nil
}

func validateDestructionRecords(g *game) error {
	recorded := map[uint64]bool{}
	for _, side := range []string{"own", "oppo"} {
		for _, record := range g.player(side).destroyed {
			if record.EventSequence == 0 {
				if record.TurnSide != "" || record.TurnNumber != 0 {
					return fmt.Errorf("invalid initial history turn")
				}
				continue
			}
			if record.TurnSide != "own" && record.TurnSide != "oppo" || record.TurnNumber < 0 || record.TurnNumber > g.turn.Number ||
				record.TurnNumber == g.turn.Number && record.TurnSide == "oppo" && g.turn.Active == "own" {
				return fmt.Errorf("invalid destruction record turn")
			}
			if record.EventSequence > uint64(len(g.events)) || recorded[record.EventSequence] {
				return fmt.Errorf("invalid continuation destruction record sequence")
			}
			event := g.events[record.EventSequence-1]
			if event.Kind != "destroyed" || event.Side != side || event.Subject == nil ||
				event.Subject.InstanceID != record.InstanceID || event.Subject.CardID != record.CardID {
				return fmt.Errorf("continuation destruction record does not match its event")
			}
			recorded[record.EventSequence] = true
		}
	}
	for _, event := range g.events {
		if event.Kind == "destroyed" && !recorded[event.Sequence] {
			return fmt.Errorf("continuation death event has no destruction record")
		}
	}
	return nil
}

func restoreBindings(saved []ContinuationBindings, instances map[string]*instance) (map[string]frame, error) {
	result := map[string]frame{}
	for _, bindingFrame := range saved {
		if bindingFrame.ID == "" || result[bindingFrame.ID] != nil || bindingFrame.Values == nil {
			return nil, fmt.Errorf("invalid continuation binding frame")
		}
		f := frame{}
		for name, values := range bindingFrame.Values {
			if name == "" {
				return nil, fmt.Errorf("invalid continuation binding name")
			}
			for _, value := range values {
				if value.Kind == "instance" && (instances[value.InstanceID] == nil || value.Side != "" || value.CardID != 0) ||
					value.Kind == "leader" && (value.Side != "own" && value.Side != "oppo" || value.InstanceID != "" || value.CardID != 0) ||
					value.Kind != "instance" && value.Kind != "leader" {
					return nil, fmt.Errorf("invalid bound target")
				}
			}
			f[name] = append([]ir.EventTarget(nil), values...)
		}
		result[bindingFrame.ID] = f
	}
	return result, nil
}

func validateChoiceRequest(request ChoiceRequest, instances map[string]*instance) error {
	if !validRuntimeID(request.RequestID) || !validRuntimeID(request.ActionID) || request.NodeID == "" || request.PublicTo != "own" && request.PublicTo != "oppo" || request.MinSelections < 0 || request.MaxSelections < request.MinSelections || request.MaxSelections > len(request.Candidates) {
		return fmt.Errorf("invalid continuation choice request")
	}
	seenEntities, seenOptions, seenLeaders := map[string]bool{}, map[int]bool{}, map[string]bool{}
	for _, candidate := range request.Candidates {
		switch candidate.Kind {
		case "entity":
			if request.Kind != "target" && request.Kind != "fusion_material" {
				return fmt.Errorf("entity candidate on non-target request")
			}
			if candidate.LeaderSide != "" || candidate.InstanceID == "" || instances[candidate.InstanceID] == nil || candidate.OptionID != 0 || seenEntities[candidate.InstanceID] || len(candidate.Labels) != 0 {
				return fmt.Errorf("invalid continuation entity candidate")
			}
			seenEntities[candidate.InstanceID] = true
		case "leader":
			if request.Kind != "target" || candidate.LeaderSide != "own" && candidate.LeaderSide != "oppo" || candidate.InstanceID != "" || candidate.OptionID != 0 || len(candidate.Labels) != 0 || seenLeaders[candidate.LeaderSide] {
				return fmt.Errorf("invalid continuation leader candidate")
			}
			seenLeaders[candidate.LeaderSide] = true
		case "option":
			if request.Kind != "mode" {
				return fmt.Errorf("option candidate on non-mode request")
			}
			if candidate.LeaderSide != "" || candidate.InstanceID != "" || candidate.OptionID == 0 || seenOptions[candidate.OptionID] || !ir.ValidChoiceLabels(candidate.Labels) {
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
	if pending.request.Kind == "fusion_material" {
		if pending.fusion == nil || pending.fusionSource == nil || pending.fusionSource.fusedThisTurn || pending.request.NodeID != pending.fusion.ID || pending.request.PublicTo != s.g.sideOf(pending.fusionSource) {
			return fmt.Errorf("continuation fusion request is detached")
		}
		candidates := s.g.fusionCandidates(pending.fusionSource, pending.fusion)
		if len(candidates) != len(pending.request.Candidates) {
			return fmt.Errorf("continuation fusion candidates changed")
		}
		for n, candidate := range candidates {
			if pending.request.Candidates[n].Kind != "entity" || pending.request.Candidates[n].InstanceID != candidate.id {
				return fmt.Errorf("continuation fusion candidates changed")
			}
		}
		return nil
	}
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
		count := min(node.SelectionCount(), len(pending.request.Candidates))
		if count < 1 || node.Kind == "require" && count != node.SelectionCount() || pending.request.Kind != "target" || node.Kind != "choose" && node.Kind != "require" || pending.binding != node.Binding || pending.request.MinSelections != count || pending.request.MaxSelections != count {
			return fmt.Errorf("continuation target request does not match its IR node")
		}
		candidates := s.g.selectionValues(node, pending.self, pending.bindings)
		if len(candidates) != len(pending.request.Candidates) {
			return fmt.Errorf("continuation target candidates changed")
		}
		for n, candidate := range candidates {
			want := candidateForValue(candidate)
			if got := pending.request.Candidates[n]; got.Kind != want.Kind || got.InstanceID != want.InstanceID || got.LeaderSide != want.LeaderSide {
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
			if candidate.Kind != "option" || candidate.OptionID != option.ID || pending.optionBlockIDs[option.ID] != blockID || !maps.Equal(candidate.Labels, option.Labels) {
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
		if card.Crest != nil {
			for _, ability := range card.Crest.Abilities {
				if ability.ID == "" {
					return nil, fmt.Errorf("crest %d has an unstable ability ID", id)
				}
				if err := addBlock(blocks, abilityBlockID(id, ability.ID), ability.Body); err != nil {
					return nil, err
				}
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
		case ir.GrantEffect:
			if nodeID == "" {
				return fmt.Errorf("grant has an unstable node ID")
			}
			if err := addBlock(blocks, nestedBlockID(nodeID, "granted"), e.Ability.Body); err != nil {
				return err
			}
		case ir.RepeatEffect:
			if nodeID == "" {
				return fmt.Errorf("repeat block has an unstable node ID")
			}
			if err := addBlock(blocks, nestedBlockID(nodeID, "repeat"), e.Body); err != nil {
				return err
			}
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
	case "deck", "hand", "field", "graveyard", "banished", "destroyed", "resolving", "attached", "crests", "retired_crest":
		return true
	}
	return false
}
