package runner

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"maps"
	"reflect"

	"wbo/internal/ir"
	"wbo/internal/ruleset"
)

type Status string

const (
	StatusCompleted Status = "completed"
	StatusSuspended Status = "suspended"
	StatusIllegal   Status = "illegal"
	StatusRejected  Status = "rejected"
	StatusFault     Status = "fault"
)

type ChoiceCandidate struct {
	Kind       string            `json:"kind"`
	InstanceID string            `json:"instanceId,omitempty"`
	OptionID   int               `json:"optionId,omitempty"`
	Labels     map[string]string `json:"labels,omitempty"`
}

type ChoiceRequest struct {
	RequestID     string            `json:"requestId"`
	ActionID      string            `json:"actionId"`
	NodeID        string            `json:"nodeId"`
	Kind          string            `json:"kind"`
	MinSelections int               `json:"minSelections"`
	MaxSelections int               `json:"maxSelections"`
	Candidates    []ChoiceCandidate `json:"candidates"`
	StateRevision uint64            `json:"stateRevision"`
	PublicTo      string            `json:"publicTo"`
}

type ChoiceResponse struct {
	RequestID           string   `json:"requestId"`
	ActionID            string   `json:"actionId"`
	StateRevision       uint64   `json:"stateRevision"`
	SelectedInstanceIDs []string `json:"selectedInstanceIds,omitempty"`
	SelectedOptionID    int      `json:"selectedOptionId,omitempty"`
}

type ContinuationFrame struct {
	BlockID          string `json:"blockId"`
	PC               int    `json:"pc"`
	SelfInstanceID   string `json:"selfInstanceId,omitempty"`
	BindingFrameID   string `json:"bindingFrameId"`
	RepeatRemaining  int    `json:"repeatRemaining,omitempty"`
	RepeatBindingsID string `json:"repeatBindingsId,omitempty"`
}

type Continuation struct {
	Version         string                 `json:"version"`
	CardPackHash    string                 `json:"cardPackHash"`
	RulesetID       string                 `json:"rulesetId"`
	RulesetHash     string                 `json:"rulesetHash"`
	ActionID        string                 `json:"actionId"`
	RequestID       string                 `json:"requestId"`
	StateRevision   uint64                 `json:"stateRevision"`
	RequestOrdinal  uint32                 `json:"requestOrdinal"`
	Stack           []ContinuationFrame    `json:"stack"`
	BindingFrames   []ContinuationBindings `json:"bindingFrames"`
	Pending         ContinuationPending    `json:"pending"`
	Triggers        []ContinuationTrigger  `json:"triggers"`
	DrainingTrigger bool                   `json:"drainingTrigger"`
	TriggerBase     int                    `json:"triggerBase"`
	Game            ContinuationGame       `json:"game"`
	Budget          ExecutionBudgetState   `json:"budget"`
}

type StepResult struct {
	Status      Status         `json:"status"`
	Choice      *ChoiceRequest `json:"choice,omitempty"`
	IllegalCode string         `json:"illegalCode,omitempty"`
	ErrorCode   string         `json:"errorCode,omitempty"`
}

type execFrame struct {
	body            []ir.Effect
	blockID         string
	pc              int
	self            *instance
	bindings        frame
	repeatRemaining int
	repeatBindings  frame
}

type pendingChoice struct {
	request        ChoiceRequest
	binding        string
	bindings       frame
	options        map[int][]ir.Effect
	optionBlockIDs map[int]string
	self           *instance
	fusion         *ir.FusionAbility
	fusionSource   *instance
}

type triggerInvocation struct {
	body     []ir.Effect
	blockID  string
	self     *instance
	bindings frame
}

type Session struct {
	g               *game
	actionID        string
	requestOrdinal  uint32
	stack           []execFrame
	pending         *pendingChoice
	drainingTrigger bool
	triggerBase     int
	blocks          map[string][]ir.Effect
	cardPackHash    string
	budgetPolicy    ruleset.ExecutionBudget
	budget          budgetTracker
	fault           string
}

func NewSession(cards *ir.CardPack, state ir.State, seed uint64) (*Session, error) {
	if cards == nil {
		return nil, fmt.Errorf("card pack is required")
	}
	index := map[int]*ir.Card{}
	for n := range cards.Cards {
		card := &cards.Cards[n]
		if index[card.ID] != nil {
			return nil, fmt.Errorf("duplicate card %d", card.ID)
		}
		index[card.ID] = card
	}
	return newSessionWithPack(index, state, seed, cards)
}

func newSession(index map[int]*ir.Card, state ir.State, seed uint64) (*Session, error) {
	return newSessionWithPack(index, state, seed, nil)
}

func newSessionWithPack(index map[int]*ir.Card, state ir.State, seed uint64, pack *ir.CardPack) (*Session, error) {
	g := &game{cards: index, instances: map[string]*instance{}, legal: true, rng: ruleset.NewRNG(seed)}
	if err := g.loadState(state); err != nil {
		return nil, err
	}
	blocks, err := indexBlocks(index)
	if err != nil {
		return nil, err
	}
	hash, err := runtimeCardPackHash(pack, index)
	if err != nil {
		return nil, err
	}
	s := &Session{g: g, blocks: blocks, cardPackHash: hash, budgetPolicy: ruleset.Default().ExecutionBudget}
	s.budget.reset(s.budgetPolicy)
	g.budget = &s.budget
	return s, nil
}

func (s *Session) Begin(actionID string, action ir.Action) StepResult {
	s.ensureBudget()
	if s.fault != "" {
		return StepResult{Status: StatusFault, ErrorCode: s.fault}
	}
	if s.g.gameOver {
		return StepResult{Status: StatusRejected, ErrorCode: "game_over"}
	}
	if s.actionID != "" || s.pending != nil || len(s.stack) != 0 {
		return StepResult{Status: StatusRejected, ErrorCode: "command_in_progress"}
	}
	if !validRuntimeID(actionID) {
		return StepResult{Status: StatusRejected, ErrorCode: "invalid_action_id"}
	}
	before := s.g.snapshot()
	var preflightBudget budgetTracker
	preflightBudget.reset(s.budgetPolicy)
	if code := s.g.preflight(action, &preflightBudget); preflightBudget.exceeded {
		s.budget = preflightBudget
		return s.budgetFault()
	} else if code != "" {
		s.g.legal, s.g.illegal = false, code
		s.g.unchanged = reflect.DeepEqual(before, s.g.snapshot())
		return StepResult{Status: StatusIllegal, IllegalCode: code}
	}
	s.budget.reset(s.budgetPolicy)
	frames, code := s.g.commitAction(action)
	if code != "" {
		s.g.legal, s.g.illegal = false, code
		s.g.unchanged = reflect.DeepEqual(before, s.g.snapshot())
		return StepResult{Status: StatusIllegal, IllegalCode: code}
	}
	s.g.legal, s.g.illegal, s.g.unchanged = true, "", false
	s.actionID = actionID
	s.requestOrdinal = 0
	s.g.revision++
	if fusion, ok := action.(ir.FusionAction); ok {
		return s.beginFusion(fusion)
	}
	for n := len(frames) - 1; n >= 0; n-- {
		s.pushFrame(frames[n])
	}
	if len(frames) > 0 {
		// Finish the action's abilities before resolving events they generated.
		s.drainingTrigger, s.triggerBase = true, 0
	}
	return s.run()
}

func (s *Session) beginFusion(action ir.FusionAction) StepResult {
	source := s.g.instances[action.Source]
	if ability, candidates := s.g.availableFusion(source); ability != nil {
		items := make([]ChoiceCandidate, 0, len(candidates))
		for _, candidate := range candidates {
			items = append(items, ChoiceCandidate{Kind: "entity", InstanceID: candidate.id})
		}
		request := s.newRequest(ability.ID, "fusion_material", ability.MaterialFilter.Minimum, len(items), items, action.Actor)
		s.pending = &pendingChoice{request: request, fusion: ability, fusionSource: source, self: source}
		return StepResult{Status: StatusSuspended, Choice: s.PendingChoice()}
	}
	s.actionID = ""
	return StepResult{Status: StatusIllegal, IllegalCode: "fusion_material_required"}
}

// Advance 仅供规则测试驱动已声明的回合事件，不属于正式玩家动作。
func (s *Session) Advance(action ir.AdvanceAction) StepResult {
	s.ensureBudget()
	if s.fault != "" {
		return StepResult{Status: StatusFault, ErrorCode: s.fault}
	}
	if s.g.gameOver {
		return StepResult{Status: StatusRejected, ErrorCode: "game_over"}
	}
	if s.actionID != "" || s.pending != nil || len(s.stack) != 0 {
		return StepResult{Status: StatusRejected, ErrorCode: "command_in_progress"}
	}
	if action.Timing != "turn_start" && action.Timing != "turn_end" || action.Side != "own" && action.Side != "oppo" {
		return StepResult{Status: StatusRejected, ErrorCode: "invalid_advance"}
	}
	s.actionID = deriveRuntimeID("advance", action.Timing, action.Side, fmt.Sprint(s.g.revision))
	s.g.legal, s.g.illegal, s.g.unchanged = true, "", false
	s.g.revision++
	kind := "turn_started"
	if action.Timing == "turn_end" {
		kind = "turn_ended"
		s.g.endingSide = action.Side
	}
	event := ir.RuntimeEvent{Kind: kind, Side: action.Side}
	if s.g.emit(event) {
		s.g.queueEventTriggers(event, nil, "")
	}
	return s.run()
}

func (s *Session) Resume(response ChoiceResponse) StepResult {
	if s.fault != "" {
		return StepResult{Status: StatusFault, ErrorCode: s.fault}
	}
	if s.pending == nil {
		return StepResult{Status: StatusRejected, ErrorCode: "no_pending_request"}
	}
	p := s.pending
	if response.RequestID != p.request.RequestID || response.ActionID != p.request.ActionID || response.StateRevision != p.request.StateRevision {
		return StepResult{Status: StatusRejected, Choice: s.PendingChoice(), ErrorCode: "stale_or_mismatched_response"}
	}
	if p.request.Kind == "target" {
		if len(response.SelectedInstanceIDs) < p.request.MinSelections || len(response.SelectedInstanceIDs) > p.request.MaxSelections || response.SelectedOptionID != 0 {
			return StepResult{Status: StatusRejected, Choice: s.PendingChoice(), ErrorCode: "invalid_selection_count"}
		}
		seen := map[string]bool{}
		for _, id := range response.SelectedInstanceIDs {
			if seen[id] || !candidateInstance(p.request.Candidates, id) {
				return StepResult{Status: StatusRejected, Choice: s.PendingChoice(), ErrorCode: "invalid_candidate"}
			}
			seen[id] = true
		}
		selected := []*instance{}
		for _, candidate := range p.request.Candidates {
			if seen[candidate.InstanceID] {
				selected = append(selected, s.g.instances[candidate.InstanceID])
			}
		}
		p.bindings[p.binding] = selected
	} else if p.request.Kind == "fusion_material" {
		if len(response.SelectedInstanceIDs) < p.request.MinSelections || len(response.SelectedInstanceIDs) > p.request.MaxSelections || response.SelectedOptionID != 0 {
			return StepResult{Status: StatusRejected, Choice: s.PendingChoice(), ErrorCode: "invalid_selection_count"}
		}
		seen := map[string]bool{}
		materials := make([]*instance, 0, len(response.SelectedInstanceIDs))
		for _, id := range response.SelectedInstanceIDs {
			if seen[id] || !candidateInstance(p.request.Candidates, id) {
				return StepResult{Status: StatusRejected, Choice: s.PendingChoice(), ErrorCode: "invalid_candidate"}
			}
			seen[id] = true
			materials = append(materials, s.g.instances[id])
		}
		if !s.g.commitFusion(p.fusionSource, p.fusion, materials) {
			if s.budget.exceeded {
				return s.budgetFault()
			}
			return StepResult{Status: StatusRejected, Choice: s.PendingChoice(), ErrorCode: "invalid_candidate"}
		}
		s.pushFrame(execFrame{body: p.fusion.Body, blockID: fusionBlockID(p.fusionSource.card.ID, p.fusion.ID), self: p.fusionSource, bindings: frame{}})
	} else if p.request.Kind == "mode" {
		body, ok := p.options[response.SelectedOptionID]
		if !ok || len(response.SelectedInstanceIDs) != 0 {
			return StepResult{Status: StatusRejected, Choice: s.PendingChoice(), ErrorCode: "invalid_option"}
		}
		s.pushFrame(execFrame{body: body, blockID: p.optionBlockIDs[response.SelectedOptionID], self: p.self, bindings: p.bindings})
	} else {
		return StepResult{Status: StatusRejected, Choice: s.PendingChoice(), ErrorCode: "unsupported_request"}
	}
	s.pending = nil
	s.g.revision++
	return s.run()
}

func (s *Session) PendingChoice() *ChoiceRequest {
	if s.pending == nil {
		return nil
	}
	request := cloneChoiceRequest(s.pending.request)
	return &request
}

func cloneChoiceRequest(request ChoiceRequest) ChoiceRequest {
	request.Candidates = append([]ChoiceCandidate(nil), request.Candidates...)
	for n := range request.Candidates {
		request.Candidates[n].Labels = maps.Clone(request.Candidates[n].Labels)
	}
	return request
}

func (s *Session) BudgetState() ExecutionBudgetState {
	return s.budget.state
}

func (s *Session) Continuation() *Continuation {
	if s.pending == nil {
		return nil
	}
	return s.makeContinuation()
}

func (s *Session) run() StepResult {
	s.ensureBudget()
	for {
		if s.fault != "" {
			return StepResult{Status: StatusFault, ErrorCode: s.fault}
		}
		if s.g.gameOver {
			s.g.finishSpell()
			s.stack = nil
			s.g.triggers = nil
			s.g.attack = nil
			s.actionID = ""
			return StepResult{Status: StatusCompleted}
		}
		if s.budget.exceeded {
			return s.budgetFault()
		}
		if len(s.stack) == 0 {
			// A spell enters the graveyard before its queued triggers resolve.
			s.g.finishSpell()
		}
		if s.drainingTrigger && len(s.stack) <= s.triggerBase {
			s.drainingTrigger = false
		}
		if !s.drainingTrigger && !s.insideRepeat() && len(s.g.triggers) > 0 {
			t := s.g.triggers[0]
			s.g.triggers = s.g.triggers[1:]
			s.triggerBase = len(s.stack)
			s.drainingTrigger = true
			s.pushFrame(execFrame{body: t.body, blockID: t.blockID, self: t.self, bindings: t.bindings})
			if s.budget.exceeded {
				return s.budgetFault()
			}
		}
		if len(s.stack) == 0 {
			if !s.drainingTrigger && s.g.attack != nil {
				s.g.advanceAttack()
				continue
			}
			if s.g.endingSide != "" {
				if !s.g.expireKeywords(s.g.endingSide) {
					return s.budgetFault()
				}
				s.g.endingSide = ""
			}
			if s.g.turnTransition == "ending" || s.g.turnTransition == "starting_triggers" {
				s.g.advanceTurn()
				continue
			}
			if s.g.turnTransition == "starting" {
				s.g.turnTransition = ""
			}
			s.actionID = ""
			return StepResult{Status: StatusCompleted}
		}
		top := &s.stack[len(s.stack)-1]
		if top.pc >= len(top.body) {
			if top.repeatRemaining > 1 {
				if !s.budget.chargeInstructions(1) {
					return s.budgetFault()
				}
				top.repeatRemaining--
				top.pc = 0
				top.bindings = copyRepeatBindings(top.repeatBindings)
				continue
			}
			s.stack = s.stack[:len(s.stack)-1]
			continue
		}
		effect := top.body[top.pc]
		top.pc++
		if !s.budget.chargeInstructions(1) {
			return s.budgetFault()
		}
		if request := s.execute(effect, top.self, top.bindings); request != nil {
			if s.budget.exceeded {
				return s.budgetFault()
			}
			s.pending = request
			return StepResult{Status: StatusSuspended, Choice: s.PendingChoice()}
		}
	}
}

func (s *Session) execute(effect ir.Effect, self *instance, bindings frame) *pendingChoice {
	switch e := effect.(type) {
	case ir.RepeatEffect:
		times := e.Times
		if e.TimesExpr != nil {
			times = s.g.numericValue(e.TimesExpr, self, bindings)
		}
		if times > 0 && !s.budget.exceeded {
			s.pushFrame(execFrame{body: e.Body, blockID: nestedBlockID(e.ID, "repeat"), self: self,
				bindings: copyRepeatBindings(bindings), repeatRemaining: times, repeatBindings: bindings})
		}
	case ir.SelectionEffect:
		candidates := s.g.selectionCandidates(e, self, bindings)
		if s.budget.exceeded {
			return nil
		}
		bindings[e.Binding] = nil
		if e.Kind == "random_choose" {
			remaining := append([]*instance(nil), candidates...)
			selected := map[*instance]bool{}
			for n := 0; n < e.SelectionCount() && len(remaining) > 0; n++ {
				index := s.g.rng.Index(len(remaining))
				selected[remaining[index]] = true
				remaining[index] = remaining[len(remaining)-1]
				remaining = remaining[:len(remaining)-1]
			}
			for _, candidate := range candidates {
				if selected[candidate] {
					bindings[e.Binding] = append(bindings[e.Binding], candidate)
				}
			}
			return nil
		}
		if len(candidates) == 0 && e.Kind == "choose" {
			return nil
		}
		return s.targetRequest(e, candidates, bindings, self)
	case ir.IfEffect:
		body := e.Else
		branch := "else"
		if s.g.condition(e.Condition, self) {
			body = e.Then
			branch = "then"
		}
		s.pushFrame(execFrame{body: body, blockID: nestedBlockID(e.ID, branch), self: self, bindings: bindings})
	case ir.ModeEffect:
		return s.modeRequest(e, bindings, self)
	case ir.PayResourceEffect:
		paid := false
		own, _, _ := s.g.relativePlayers(self)
		if e.Resource == "shadows" && own.shadows >= e.Amount {
			own.shadows -= e.Amount
			paid = true
		} else if e.Resource == "earthsigil" {
			paid = s.g.consumeEarthSigil(self, e.Amount)
		}
		if paid {
			s.pushFrame(execFrame{body: e.OnPaid, blockID: nestedBlockID(e.ID, "onPaid"), self: self, bindings: bindings})
		}
	case ir.DrawEffect:
		s.g.draw(e, self, bindings)
	case ir.CardEffect:
		s.g.execCardEffect(e, self, bindings)
	case ir.TargetEffect:
		s.g.execTargetEffect(e, self, bindings)
	case ir.AdjustEffect:
		if e.Kind == "adjust_counter" {
			if self == nil {
				return nil
			}
			value, exists := self.counters[e.Field]
			if !exists {
				return nil
			}
			if e.Delta < 0 || e.Delta > ir.MaxCounterValue-value {
				s.fault = "counter_overflow"
				return nil
			}
			self.counters[e.Field] = value + e.Delta
		} else {
			s.g.execAdjust(e, self, bindings)
		}
	}
	return nil
}

func (s *Session) targetRequest(e ir.SelectionEffect, candidates []*instance, bindings frame, self *instance) *pendingChoice {
	if !s.budget.chargeCandidates(uint64(len(candidates))) {
		return nil
	}
	items := make([]ChoiceCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		items = append(items, ChoiceCandidate{Kind: "entity", InstanceID: candidate.id})
	}
	count := min(e.SelectionCount(), len(candidates))
	request := s.newRequest(e.ID, "target", count, count, items, s.g.sideOf(self))
	return &pendingChoice{request: request, binding: e.Binding, bindings: bindings, self: self}
}

func (s *Session) modeRequest(e ir.ModeEffect, bindings frame, self *instance) *pendingChoice {
	if !s.budget.chargeCandidates(uint64(len(e.Options))) {
		return nil
	}
	items := make([]ChoiceCandidate, 0, len(e.Options))
	options := map[int][]ir.Effect{}
	optionBlockIDs := map[int]string{}
	for _, option := range e.Options {
		items = append(items, ChoiceCandidate{Kind: "option", OptionID: option.ID, Labels: maps.Clone(option.Labels)})
		options[option.ID] = option.Body
		optionBlockIDs[option.ID] = nestedBlockID(e.ID, fmt.Sprintf("option:%d", option.ID))
	}
	request := s.newRequest(e.ID, "mode", 1, 1, items, s.g.sideOf(self))
	return &pendingChoice{request: request, bindings: bindings, options: options, optionBlockIDs: optionBlockIDs, self: self}
}

func (s *Session) pushFrame(f execFrame) {
	if !s.budget.observeStack(len(s.stack) + 1) {
		return
	}
	s.stack = append(s.stack, f)
}

func (s *Session) budgetFault() StepResult {
	s.fault = executionBudgetExceeded
	s.pending = nil
	return StepResult{Status: StatusFault, ErrorCode: s.fault}
}

func (s *Session) ensureBudget() {
	if s.budgetPolicy.Instructions == 0 {
		s.budgetPolicy = ruleset.Default().ExecutionBudget
		s.budget.reset(s.budgetPolicy)
	}
	if s.g != nil && s.g.budget == nil {
		s.g.budget = &s.budget
	}
}

func (s *Session) newRequest(nodeID, kind string, min, max int, candidates []ChoiceCandidate, publicTo string) ChoiceRequest {
	s.requestOrdinal++
	return ChoiceRequest{
		RequestID: deriveRuntimeID(s.actionID, nodeID, fmt.Sprint(s.requestOrdinal)),
		ActionID:  s.actionID, NodeID: nodeID, Kind: kind,
		MinSelections: min, MaxSelections: max, Candidates: candidates,
		StateRevision: s.g.revision, PublicTo: publicTo,
	}
}

func candidateInstance(candidates []ChoiceCandidate, id string) bool {
	for _, candidate := range candidates {
		if candidate.Kind == "entity" && candidate.InstanceID == id {
			return true
		}
	}
	return false
}

func validRuntimeID(id string) bool {
	if len(id) != 32 {
		return false
	}
	_, err := hex.DecodeString(id)
	return err == nil
}

func deriveRuntimeID(parts ...string) string {
	h := sha256.New()
	for _, part := range parts {
		h.Write([]byte(part))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil)[:16])
}

type instanceSnapshot struct {
	Counters                            map[string]int
	FusedThisTurn                       bool
	ID, Zone                            string
	CardID, Cost                        int
	Attack, Life, Earthsigil, Countdown int
	AttacksUsed                         int
	Engaged, SummoningSick              bool
	Evolved, SuperEvolved               bool
	Departed                            bool
	Abilities                           map[string]bool
	TemporaryKeywords                   map[string]KeywordExpiry
}

type gameSnapshot struct {
	Own, Oppo                       playerSnapshot
	Instances                       []instanceSnapshot
	Events                          []ir.RuntimeEvent
	RNG                             ruleset.RNGState
	Serial                          int
	EventSequence, DeathBatchSerial uint64
	Revision                        uint64
	Triggers                        int
	Turn                            ir.Turn
	FirstPlayer                     string
	Phase                           string
	TurnTransition                  string
	EndingSide                      string
	GameOver                        bool
	Winner                          string
	Attack                          *attackSnapshot
}

type attackSnapshot struct {
	Stage                          string
	Actor, Attacker, Defender      string
	AttackerAttack, DefenderAttack int
}

type playerSnapshot struct {
	PP, MaxPP, LeaderLife, LeaderMax, EP, SEP, Combo, Shadows int
	Deck, Hand, Field, Graveyard, Banished, Destroyed         []string
	Resolving                                                 []string
	AttackedThisTurn, EvolvedThisTurn                         bool
	ExtraPPEarly, ExtraPPLate, ExtraPPActive                  bool
}

func (g *game) snapshot() gameSnapshot {
	snapshot := gameSnapshot{Own: snapshotPlayer(g.own), Oppo: snapshotPlayer(g.oppo), Events: append([]ir.RuntimeEvent(nil), g.events...), RNG: g.rng.Snapshot(), Serial: g.serial, EventSequence: g.eventSequence, DeathBatchSerial: g.deathBatchSerial, Revision: g.revision, Triggers: len(g.triggers), Turn: g.turn, FirstPlayer: g.firstPlayer, Phase: g.phase, TurnTransition: g.turnTransition, EndingSide: g.endingSide, GameOver: g.gameOver, Winner: g.winner}
	if g.attack != nil {
		snapshot.Attack = &attackSnapshot{Stage: g.attack.stage, Actor: g.attack.actor, Attacker: g.attack.attacker, Defender: g.attack.defender, AttackerAttack: g.attack.attackerAttack, DefenderAttack: g.attack.defenderAttack}
	}
	for _, side := range []*player{&g.own, &g.oppo} {
		for _, zone := range [][]*instance{side.deck, side.hand, side.field, side.graveyard, side.banished, side.resolving} {
			for _, i := range zone {
				abilities := map[string]bool{}
				for name, value := range i.abilities {
					abilities[name] = value
				}
				snapshot.Instances = append(snapshot.Instances, instanceSnapshot{
					Counters:          maps.Clone(i.counters),
					TemporaryKeywords: maps.Clone(i.temporaryKeywords),
					ID:                i.id, Zone: i.zone, CardID: i.card.ID, Cost: i.cost, Attack: i.attack, Life: i.life,
					Earthsigil: i.earthsigil, Countdown: i.countdown, AttacksUsed: i.attacksUsed,
					Engaged: i.engaged, SummoningSick: i.summoningSick, Evolved: i.evolved,
					SuperEvolved: i.superEvolved, Departed: i.departed, FusedThisTurn: i.fusedThisTurn, Abilities: abilities,
				})
			}
		}
	}
	return snapshot
}

func snapshotPlayer(p player) playerSnapshot {
	return playerSnapshot{
		PP: p.pp, MaxPP: p.maxpp, LeaderLife: p.leaderLife, LeaderMax: p.leaderMax,
		EP: p.ep, SEP: p.sep, Combo: p.combo, Shadows: p.shadows,
		Deck: ids(p.deck), Hand: ids(p.hand), Field: ids(p.field), Graveyard: ids(p.graveyard),
		Resolving: ids(p.resolving),
		Banished:  ids(p.banished), Destroyed: ids(p.destroyed), AttackedThisTurn: p.attackedThisTurn, EvolvedThisTurn: p.evolvedThisTurn,
		ExtraPPEarly: p.extraPPEarly, ExtraPPLate: p.extraPPLate, ExtraPPActive: p.extraPPActive,
	}
}

func (g *game) clone() *game {
	clone := &game{
		cards: g.cards, instances: map[string]*instance{}, legal: g.legal, illegal: g.illegal,
		unchanged: g.unchanged, rng: g.rng.Clone(), events: append([]ir.RuntimeEvent(nil), g.events...),
		serial: g.serial, eventSequence: g.eventSequence, deathBatchSerial: g.deathBatchSerial, revision: g.revision,
		turn: g.turn, firstPlayer: g.firstPlayer, phase: g.phase, turnTransition: g.turnTransition, endingSide: g.endingSide, gameOver: g.gameOver, winner: g.winner,
	}
	if g.attack != nil {
		attack := *g.attack
		clone.attack = &attack
	}
	for id, original := range g.instances {
		copy := *original
		copy.counters = maps.Clone(original.counters)
		copy.temporaryKeywords = maps.Clone(original.temporaryKeywords)
		copy.abilities = map[string]bool{}
		for name, value := range original.abilities {
			copy.abilities[name] = value
		}
		clone.instances[id] = &copy
	}
	clone.own = clonePlayer(g.own, clone.instances)
	clone.oppo = clonePlayer(g.oppo, clone.instances)
	clone.rebuildTriggerIndex()
	for _, trigger := range g.triggers {
		bindings := frame{}
		for name, values := range trigger.bindings {
			for _, value := range values {
				bindings[name] = append(bindings[name], clone.instances[value.id])
			}
		}
		var self *instance
		if trigger.self != nil {
			self = clone.instances[trigger.self.id]
		}
		clone.triggers = append(clone.triggers, triggerInvocation{body: trigger.body, blockID: trigger.blockID, self: self, bindings: bindings})
	}
	return clone
}

func clonePlayer(original player, instances map[string]*instance) player {
	return player{
		pp: original.pp, maxpp: original.maxpp, leaderLife: original.leaderLife, leaderMax: original.leaderMax,
		ep: original.ep, sep: original.sep, combo: original.combo, shadows: original.shadows,
		deck: cloneInstances(original.deck, instances), hand: cloneInstances(original.hand, instances),
		field: cloneInstances(original.field, instances), graveyard: cloneInstances(original.graveyard, instances),
		banished: cloneInstances(original.banished, instances), destroyed: cloneInstances(original.destroyed, instances),
		resolving:        cloneInstances(original.resolving, instances),
		attackedThisTurn: original.attackedThisTurn, evolvedThisTurn: original.evolvedThisTurn,
		extraPPEarly: original.extraPPEarly, extraPPLate: original.extraPPLate, extraPPActive: original.extraPPActive,
	}
}

func cloneInstances(original []*instance, instances map[string]*instance) []*instance {
	cloned := make([]*instance, 0, len(original))
	for _, item := range original {
		cloned = append(cloned, instances[item.id])
	}
	return cloned
}

func ids(items []*instance) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.id)
	}
	return out
}
