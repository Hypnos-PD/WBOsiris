package runner

import (
	"fmt"

	"wbo/internal/ir"
	"wbo/internal/ruleset"
)

const fieldLimit = 5

type Result struct {
	File, Scenario string
	Failures       []string
}

func (r Result) Passed() bool { return len(r.Failures) == 0 }

type instance struct {
	id, alias, zone                          string
	card                                     *ir.Card
	attack, life, earthsigil, countdown      int
	engaged, attacked, evolved, superEvolved bool
	abilities                                map[string]bool
}
type player struct {
	pp, maxpp, leaderLife, leaderMax, ep, sep, combo, shadows int
	deck, hand, field, graveyard, banished, destroyed         []*instance
}
type frame map[string][]*instance
type game struct {
	cards                           map[int]*ir.Card
	own, oppo                       player
	instances                       map[string]*instance
	legal                           bool
	illegal                         string
	unchanged                       bool
	rng                             *ruleset.RNG
	events                          []ir.RuntimeEvent
	serial                          int
	eventSequence, deathBatchSerial uint64
	revision                        uint64
	triggers                        []triggerInvocation
	triggerIndex                    triggerIndex
	budget                          *budgetTracker
	turn                            ir.Turn
	phase                           string
	turnTransition                  string
}

func Run(cards *ir.CardPack, tests *ir.TestPack) []Result {
	return RunWithRuleset(cards, tests, ruleset.DefaultID)
}
func RunWithRuleset(cards *ir.CardPack, tests *ir.TestPack, rulesetID string) []Result {
	if rulesetID != ruleset.DefaultID || !matchesDefaultRuleset(tests.Ruleset) {
		return []Result{{Scenario: "规则集", Failures: []string{"unsupported ruleset " + rulesetID}}}
	}
	index := map[int]*ir.Card{}
	for n := range cards.Cards {
		c := &cards.Cards[n]
		index[c.ID] = c
	}
	paths := map[string]string{}
	for _, s := range tests.Sources {
		paths[s.SourceID] = s.Path
	}
	results := make([]Result, 0, len(tests.Scenarios))
	for n := range tests.Scenarios {
		results = append(results, runScenario(paths[tests.Scenarios[n].Origin.Primary.SourceID], &tests.Scenarios[n], index))
	}
	return results
}

func matchesDefaultRuleset(got ir.RulesetDependency) bool {
	want := ruleset.DefaultDependency()
	return got.ID == want.ID && got.ContentHash == want.ContentHash &&
		got.RNG.Algorithm == want.RNG.Algorithm && got.RNG.Version == want.RNG.Version &&
		got.OrderingPolicy.Collections == want.OrderingPolicy.Collections &&
		got.OrderingPolicy.ReplacementAbilities == want.OrderingPolicy.ReplacementAbilities &&
		got.OrderingPolicy.SimultaneousTriggers == want.OrderingPolicy.SimultaneousTriggers &&
		got.OrderingPolicy.DeathBatchLastwords == want.OrderingPolicy.DeathBatchLastwords &&
		got.ExecutionBudget.Instructions == want.ExecutionBudget.Instructions &&
		got.ExecutionBudget.QueryVisits == want.ExecutionBudget.QueryVisits &&
		got.ExecutionBudget.StackDepth == want.ExecutionBudget.StackDepth &&
		got.ExecutionBudget.Candidates == want.ExecutionBudget.Candidates &&
		got.ExecutionBudget.Events == want.ExecutionBudget.Events &&
		got.ExecutionBudget.Triggers == want.ExecutionBudget.Triggers &&
		got.ExecutionBudget.CreatedInstances == want.ExecutionBudget.CreatedInstances &&
		got.ExecutionBudget.ContinuationBytes == want.ExecutionBudget.ContinuationBytes
}
func runScenario(path string, s *ir.Scenario, cards map[int]*ir.Card) Result {
	r := Result{File: path, Scenario: s.Name}
	seed, err := parseSeed(s.Seed)
	if err != nil {
		r.Failures = append(r.Failures, "invalid scenario seed")
		return r
	}
	session, err := newSession(cards, s.InitialState, seed)
	if err != nil {
		r.Failures = append(r.Failures, err.Error())
		return r
	}
	if len(s.Actions) == 0 {
		r.Failures = append(r.Failures, "missing action")
		return r
	}
	step := beginScenarioAction(session, s.ID, s.Actions[0])
	next := 1
	for step.Status == StatusSuspended {
		if next >= len(s.Actions) {
			r.Failures = append(r.Failures, "missing response for request "+step.Choice.RequestID)
			break
		}
		response := ChoiceResponse{RequestID: step.Choice.RequestID, ActionID: step.Choice.ActionID, StateRevision: step.Choice.StateRevision}
		switch x := s.Actions[next].(type) {
		case ir.SelectAction:
			response.SelectedInstanceIDs = []string{x.Target}
		case ir.ModeAction:
			response.SelectedOptionID = x.OptionID
		default:
			r.Failures = append(r.Failures, "action response kind does not match request")
		}
		if len(r.Failures) != 0 {
			break
		}
		step = session.Resume(response)
		if step.Status == StatusRejected {
			r.Failures = append(r.Failures, "response rejected: "+step.ErrorCode)
			break
		}
		next++
	}
	if step.Status == StatusFault {
		r.Failures = append(r.Failures, "runtime fault: "+step.ErrorCode)
	}
	if step.Status != StatusSuspended && next < len(s.Actions) {
		r.Failures = append(r.Failures, "extra response without a pending request")
	}
	g := session.g
	for _, a := range s.Assertions {
		if failure := g.assert(a); failure != "" {
			line := ir.AssertionOrigin(a).Primary.StartLine
			r.Failures = append(r.Failures, fmt.Sprintf("line %d: %s", line, failure))
		}
	}
	return r
}

func beginScenarioAction(session *Session, scenarioID string, action ir.Action) StepResult {
	if advance, ok := action.(ir.AdvanceAction); ok {
		return session.Advance(advance)
	}
	return session.Begin(deriveRuntimeID(scenarioID, "command", "0"), action)
}
func parseSeed(s string) (uint64, error) {
	var n uint64
	_, err := fmt.Sscanf(s, "0x%016x", &n)
	return n, err
}
func (g *game) loadState(s ir.State) error {
	if s.Turn.Active != "own" && s.Turn.Active != "oppo" || s.Turn.Number < 0 || s.Phase != "main" {
		return fmt.Errorf("invalid initial turn state")
	}
	g.turn, g.phase = s.Turn, s.Phase
	for _, side := range []string{"own", "oppo"} {
		src := s.Players[side]
		p := &g.own
		if side == "oppo" {
			p = &g.oppo
		}
		p.leaderLife, p.leaderMax = src.Leader.Life, src.Leader.MaxLife
		p.pp, p.maxpp, p.ep, p.sep, p.combo, p.shadows = src.PP, src.MaxPP, src.EP, src.SEP, src.Combo, src.Shadows
		for _, zone := range []string{"deck", "hand", "field", "graveyard", "banished", "destroyed"} {
			for _, decl := range src.Zones[zone] {
				c := g.cards[decl.CardID]
				if c == nil {
					return fmt.Errorf("unknown card %d", decl.CardID)
				}
				if c.CardType != decl.DeclaredType {
					return fmt.Errorf("card type mismatch for %d", decl.CardID)
				}
				i := g.newInstance(c, decl.InstanceID, decl.Alias, zone)
				o := decl.Overrides
				if o.Stats != nil {
					i.attack, i.life = o.Stats.Attack, o.Stats.Life
				}
				if o.Earthsigil != nil {
					i.earthsigil = *o.Earthsigil
				}
				if o.Countdown != nil {
					i.countdown = *o.Countdown
				}
				if o.Engaged != nil {
					i.engaged = *o.Engaged
				}
				if o.Evolved != nil {
					i.evolved = *o.Evolved
				}
				if o.SuperEvolved != nil {
					i.superEvolved = *o.SuperEvolved
				}
				for _, k := range o.Keywords {
					i.abilities[k] = true
				}
				g.addToZone(p, i, zone)
			}
		}
	}
	return nil
}
func (g *game) newInstance(c *ir.Card, id, alias, zone string) *instance {
	i := &instance{id: id, alias: alias, card: c, zone: zone, abilities: map[string]bool{}}
	if c.Stats != nil {
		i.attack, i.life = c.Stats.Attack, c.Stats.Life
	}
	for _, k := range c.Intrinsic {
		i.abilities[k] = true
	}
	for _, s := range c.IntrinsicState {
		if s.Kind == "earthsigil" {
			i.earthsigil = s.Initial
		} else if s.Kind == "countdown" {
			i.countdown = s.Initial
		}
	}
	g.instances[id] = i
	return i
}

func (g *game) reserveCreatedInstance() bool {
	return g.budget == nil || g.budget.chargeCreatedInstances(1)
}

func (g *game) emit(event ir.RuntimeEvent) bool {
	if g.budget != nil && !g.budget.chargeEvents(1) {
		return false
	}
	g.eventSequence++
	event.Sequence = g.eventSequence
	g.events = append(g.events, event)
	return true
}

func (g *game) queueTrigger(trigger triggerInvocation) bool {
	if g.budget != nil && !g.budget.chargeTriggers(1) {
		return false
	}
	g.triggers = append(g.triggers, trigger)
	return true
}

func (g *game) chargeQueryVisits(amount int) bool {
	if g.budget != nil && amount > 0 {
		return g.budget.chargeQueryVisits(uint64(amount))
	}
	return true
}
func (g *game) addToZone(p *player, i *instance, z string) {
	i.zone = z
	switch z {
	case "deck":
		p.deck = append(p.deck, i)
	case "hand":
		p.hand = append(p.hand, i)
	case "field":
		p.field = append(p.field, i)
		g.triggerIndex.add(i)
	case "graveyard":
		p.graveyard = append(p.graveyard, i)
	case "banished":
		p.banished = append(p.banished, i)
	case "destroyed":
		p.destroyed = append(p.destroyed, i)
	}
}
func (g *game) preflight(a ir.Action, budget *budgetTracker) string {
	if g.turn.Active != "own" || g.phase != "main" {
		return "wrong_timing"
	}
	if x, ok := a.(ir.AttackAction); ok {
		return g.preflightAttack(x)
	}
	x, ok := a.(ir.SourceAction)
	if !ok {
		return "unsupported_action"
	}
	if x.Actor != "own" {
		return "wrong_actor"
	}
	if x.Kind == "end_turn" {
		if g.turnTransition != "" {
			return "command_in_progress"
		}
		return ""
	}
	i := g.instances[x.Source]
	if i == nil {
		return "unknown_alias"
	}
	switch x.Kind {
	case "play":
		if i.zone != "hand" || !contains(g.own.hand, i) || g.own.pp < i.card.Cost {
			return "cost"
		}
		for _, restriction := range i.card.Restrictions {
			if restriction.Kind == "unplayable" {
				return "unplayable"
			}
		}
		if i.card.CardType != "spell" && len(g.own.field) >= fieldLimit {
			return "field_full"
		}
		sandbox := g.clone()
		sandbox.budget = budget
		sandboxSource := sandbox.instances[i.id]
		sandbox.applyPlaySetup(sandboxSource, false)
		if code := sandbox.preflightRequirements(sandboxSource.card.PlayEffects, sandboxSource, frame{}, true); code != "" {
			return code
		}
		for _, ability := range sandboxSource.card.Abilities {
			if ir.TriggerKind(ability.Trigger) == "fanfare" {
				if code := sandbox.preflightRequirements(ability.Body, sandboxSource, frame{}, true); code != "" {
					return code
				}
			}
		}
	case "engage":
		a := findAbility(i.card, "engage")
		if i.zone != "field" || !contains(g.own.field, i) || i.engaged || a == nil || g.own.pp < a.Trigger.(ir.CostTrigger).Cost {
			return "cost"
		}
		sandbox := g.clone()
		sandbox.budget = budget
		sandboxSource := sandbox.instances[i.id]
		sandbox.own.pp -= a.Trigger.(ir.CostTrigger).Cost
		sandboxSource.engaged = true
		return sandbox.preflightRequirements(a.Body, sandboxSource, frame{}, true)
	case "superevolve":
		if i.zone != "field" || !contains(g.own.field, i) || i.card.CardType != "follower" || i.superEvolved || g.own.sep < 1 {
			return "cost"
		}
		sandbox := g.clone()
		sandbox.budget = budget
		sandboxSource := sandbox.instances[i.id]
		sandbox.own.sep--
		sandboxSource.evolved, sandboxSource.superEvolved = true, true
		abilities := map[string]ir.Ability{}
		for _, ability := range sandboxSource.card.Abilities {
			abilities[ability.ID] = ability
		}
		for _, plan := range sandboxSource.card.ActionPlans {
			if plan.Action != "superevolve" {
				continue
			}
			for _, planStep := range plan.Steps {
				ability := abilities[planStep.AbilityID]
				if code := sandbox.preflightRequirements(ability.Body, sandboxSource, frame{}, true); code != "" {
					return code
				}
			}
		}
	default:
		return "unsupported_action"
	}
	return ""
}

func (g *game) commitAction(a ir.Action) ([]execFrame, string) {
	if x, ok := a.(ir.AttackAction); ok {
		return nil, g.commitAttack(x)
	}
	x, ok := a.(ir.SourceAction)
	if !ok {
		return nil, "unsupported_action"
	}
	if x.Kind == "end_turn" {
		if g.turnTransition != "" {
			return nil, "command_in_progress"
		}
		g.turnTransition = "ending"
		event := ir.RuntimeEvent{Kind: "turn_ended", Side: g.turn.Active}
		if g.emit(event) {
			g.queueEventTriggers(event, nil, "")
		}
		return nil, ""
	}
	i := g.instances[x.Source]
	if i == nil {
		return nil, "unknown_alias"
	}
	switch x.Kind {
	case "play":
		return g.commitPlay(i), ""
	case "engage":
		return g.commitEngage(i), ""
	case "superevolve":
		return g.commitSuperEvolve(i), ""
	default:
		return nil, "unsupported_action"
	}
}

func (g *game) preflightAttack(a ir.AttackAction) string {
	if a.Actor != "own" {
		return "wrong_actor"
	}
	attacker := g.instances[a.Attacker]
	if attacker == nil || attacker.zone != "field" || !contains(g.own.field, attacker) || attacker.card.CardType != "follower" {
		return "invalid_attacker"
	}
	if attacker.attacked {
		return "already_attacked"
	}
	for _, keyword := range []string{"rush", "storm", "bane", "drain", "intimidate", "ward"} {
		if attacker.abilities[keyword] {
			return "unsupported_keyword"
		}
	}
	if a.Kind == "attack_leader" {
		if a.Defender != "oppo" {
			return "invalid_defender"
		}
		return ""
	}
	if a.Kind != "attack_entity" {
		return "unsupported_action"
	}
	defender := g.instances[a.Defender]
	if defender == nil || defender.zone != "field" || !contains(g.oppo.field, defender) || defender.card.CardType != "follower" {
		return "invalid_defender"
	}
	for _, keyword := range []string{"ward", "intimidate"} {
		if defender.abilities[keyword] {
			return "unsupported_keyword"
		}
	}
	return ""
}

func (g *game) commitAttack(a ir.AttackAction) string {
	attacker := g.instances[a.Attacker]
	var defender *instance
	if a.Kind == "attack_entity" {
		defender = g.instances[a.Defender]
	}
	attacker.attacked = true
	attackerTarget := ir.EventTarget{Kind: "instance", InstanceID: attacker.id}
	defenderTarget := ir.EventTarget{Kind: "leader", Side: "oppo"}
	if defender != nil {
		defenderTarget = ir.EventTarget{Kind: "instance", InstanceID: defender.id}
	}
	event := ir.RuntimeEvent{Kind: "attacked", Attacker: &attackerTarget, Defender: &defenderTarget}
	if !g.emit(event) {
		return ""
	}
	g.queueEventTriggers(event, attacker, "attack")
	if defender == nil {
		g.damageLeader(&g.oppo, "oppo", attacker.attack)
		return ""
	}
	defender.life -= attacker.attack
	attacker.life -= defender.attack
	g.emitDamage(attacker.attack, &defenderTarget)
	g.emitDamage(defender.attack, &attackerTarget)
	g.resolveDeathBatch(nil)
	return ""
}

func (g *game) damageLeader(target *player, side string, amount int) {
	if amount < 0 {
		amount = 0
	}
	actual := min(amount, target.leaderLife)
	t := ir.EventTarget{Kind: "leader", Side: side}
	if g.emit(ir.RuntimeEvent{Kind: "damaged", Actual: actual, Target: &t}) {
		target.leaderLife -= actual
	}
}

func (g *game) emitDamage(amount int, target *ir.EventTarget) {
	if amount < 0 {
		amount = 0
	}
	g.emit(ir.RuntimeEvent{Kind: "damaged", Actual: amount, Target: target})
}

func (g *game) commitPlay(i *instance) []execFrame {
	g.applyPlaySetup(i, true)
	frames := []execFrame{{body: i.card.PlayEffects, blockID: cardPlayBlockID(i.card.ID), self: i, bindings: frame{}}}
	for _, a := range i.card.Abilities {
		if ir.TriggerKind(a.Trigger) == "fanfare" && (i.zone == "field" || i.card.CardType == "spell") {
			frames = append(frames, execFrame{body: a.Body, blockID: abilityBlockID(i.card.ID, a.ID), self: i, bindings: frame{}})
		}
	}
	return frames
}

func (g *game) applyPlaySetup(i *instance, emit bool) {
	g.own.pp -= i.card.Cost
	g.remove(&g.own.hand, i)
	if i.card.CardType == "spell" {
		g.addToZone(&g.own, i, "graveyard")
	} else {
		g.addToZone(&g.own, i, "field")
		g.own.combo++
		g.mergeEarthSigil(i)
		if emit && i.card.CardType == "follower" {
			g.triggerSummoned(i)
		}
	}
}
func (g *game) commitEngage(i *instance) []execFrame {
	a := findAbility(i.card, "engage")
	g.own.pp -= a.Trigger.(ir.CostTrigger).Cost
	i.engaged = true
	g.triggerEngaged(i)
	return []execFrame{{body: a.Body, blockID: abilityBlockID(i.card.ID, a.ID), self: i, bindings: frame{}}}
}
func (g *game) commitSuperEvolve(i *instance) []execFrame {
	g.own.sep--
	i.evolved, i.superEvolved = true, true
	abilities := map[string]ir.Ability{}
	for _, a := range i.card.Abilities {
		abilities[a.ID] = a
	}
	for _, p := range i.card.ActionPlans {
		if p.Action != "superevolve" {
			continue
		}
		var f frame
		var frames []execFrame
		for _, step := range p.Steps {
			if step.Frame == "new" || f == nil {
				f = frame{}
			}
			a := abilities[step.AbilityID]
			frames = append(frames, execFrame{body: a.Body, blockID: abilityBlockID(i.card.ID, a.ID), self: i, bindings: f})
		}
		return frames
	}
	return nil
}

func (g *game) advanceTurn() {
	if g.turnTransition == "ending" {
		if g.turn.Active == "oppo" {
			g.turn.Number++
		}
		g.turn.Active = oppositeSide(g.turn.Active)
		g.turnTransition = "starting"
	}
	active := g.player(g.turn.Active)
	if active.maxpp < 10 {
		active.maxpp++
	}
	active.pp = active.maxpp
	active.combo = 0
	var expired []*instance
	for _, i := range active.field {
		i.engaged = false
		i.attacked = false
		if i.card.CardType == "amulet" && i.countdown > 0 {
			i.countdown--
			if i.countdown == 0 {
				expired = append(expired, i)
			}
		}
	}
	g.resolveDeathBatch(expired)
	g.draw(ir.DrawEffect{Kind: "draw", Owner: g.turn.Active, Count: 1}, nil, frame{})
	event := ir.RuntimeEvent{Kind: "turn_started", Side: g.turn.Active}
	if g.emit(event) {
		g.queueEventTriggers(event, nil, "")
	}
}

func findAbility(card *ir.Card, kind string) *ir.Ability {
	for n := range card.Abilities {
		if ir.TriggerKind(card.Abilities[n].Trigger) == kind {
			return &card.Abilities[n]
		}
	}
	return nil
}

func (g *game) preflightRequirements(body []ir.Effect, self *instance, bindings frame, querySafe bool) string {
	return g.preflightRequirementsAtDepth(body, self, bindings, querySafe, 1)
}

func (g *game) preflightRequirementsAtDepth(body []ir.Effect, self *instance, bindings frame, querySafe bool, depth int) string {
	if g.budget != nil && !g.budget.observeStack(depth) {
		return executionBudgetExceeded
	}
	for _, effect := range body {
		switch e := effect.(type) {
		case ir.SelectionEffect:
			if e.Kind == "require" {
				if !querySafe {
					return "unsupported_preflight"
				}
				candidates := g.fromRef(e.Source, self, bindings)
				if g.budget != nil && (g.budget.exceeded || !g.budget.chargeCandidates(uint64(len(candidates)))) {
					return executionBudgetExceeded
				}
				if len(candidates) == 0 {
					return "target_required"
				}
				querySafe = false
			} else if e.Kind != "random_choose" {
				querySafe = false
			}
		case ir.IfEffect:
			branch := e.Else
			if g.condition(e.Condition, self) {
				branch = e.Then
			}
			if code := g.preflightRequirementsAtDepth(branch, self, bindings, querySafe, depth+1); code != "" {
				return code
			}
		case ir.ModeEffect:
			for _, option := range e.Options {
				if containsRequire(option.Body) {
					return "unsupported_preflight"
				}
			}
			querySafe = false
		case ir.PayResourceEffect:
			if containsRequire(e.OnPaid) {
				return "unsupported_preflight"
			}
			querySafe = false
		default:
			querySafe = false
		}
	}
	return ""
}

func containsRequire(body []ir.Effect) bool {
	for _, effect := range body {
		if ir.EffectKind(effect) == "require" {
			return true
		}
		switch e := effect.(type) {
		case ir.IfEffect:
			if containsRequire(e.Then) || containsRequire(e.Else) {
				return true
			}
		case ir.ModeEffect:
			for _, option := range e.Options {
				if containsRequire(option.Body) {
					return true
				}
			}
		case ir.PayResourceEffect:
			if containsRequire(e.OnPaid) {
				return true
			}
		}
	}
	return false
}
