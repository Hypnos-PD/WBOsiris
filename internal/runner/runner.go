package runner

import (
	"fmt"

	"wbo/internal/ir"
	"wbo/internal/ruleset"
)

const (
	fieldLimit = 5
	handLimit  = 9
)

type Result struct {
	File, Scenario string
	Failures       []string
}

func (r Result) Passed() bool { return len(r.Failures) == 0 }

type instance struct {
	id, alias, zone                                                        string
	card                                                                   *ir.Card
	attack, life, earthsigil, countdown, damageReduction, attackLimitValue int
	attacksUsed                                                            int
	engaged, summoningSick                                                 bool
	evolved, superEvolved                                                  bool
	abilities                                                              map[string]bool
	materials                                                              []*instance
}
type player struct {
	pp, maxpp, leaderLife, leaderMax, ep, sep, combo, shadows int
	deck, hand, field, graveyard, banished, destroyed         []*instance
	attackedThisTurn, evolvedThisTurn                         bool
	extraPPEarly, extraPPLate                                 bool
	extraPPActive                                             bool
}
type attackState struct {
	stage                          string
	actor, attacker, defender      string
	attackerAttack, defenderAttack int
}
type damageContext struct {
	source     *instance
	target     *instance
	amount     int
	damageType string
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
	firstPlayer                     string
	phase                           string
	turnTransition                  string
	gameOver                        bool
	winner                          string
	attack                          *attackState
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
	g.turn, g.phase, g.firstPlayer = s.Turn, s.Phase, s.FirstPlayer
	for _, side := range []string{"own", "oppo"} {
		src := s.Players[side]
		p := &g.own
		if side == "oppo" {
			p = &g.oppo
		}
		p.leaderLife, p.leaderMax = src.Leader.Life, src.Leader.MaxLife
		p.pp, p.maxpp, p.ep, p.sep, p.combo, p.shadows = src.PP, src.MaxPP, src.EP, src.SEP, src.Combo, src.Shadows
		p.extraPPEarly, p.extraPPLate = src.ExtraPPEarly, src.ExtraPPLate
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
				if o.DamageReduction != nil {
					i.damageReduction = *o.DamageReduction
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
		} else if s.Kind == "damage_reduction" {
			i.damageReduction = s.Initial
		} else if s.Kind == "attack_limit" {
			i.attackLimitValue = s.Initial
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
	if g.gameOver {
		return "game_over"
	}
	if g.phase != "main" {
		return "wrong_timing"
	}
	if x, ok := a.(ir.AttackAction); ok {
		return g.preflightAttack(x)
	}
	if x, ok := a.(ir.FusionAction); ok {
		if x.Actor != g.turn.Active {
			return "wrong_timing"
		}
		source := g.instances[x.Source]
		if source == nil || source.zone != "hand" || !contains(g.player(x.Actor).hand, source) {
			return "invalid_fusion_source"
		}
		if len(g.fusionCandidates(source)) == 0 {
			return "fusion_material_required"
		}
		return ""
	}
	x, ok := a.(ir.SourceAction)
	if !ok {
		return "unsupported_action"
	}
	if x.Actor != "own" && x.Actor != "oppo" {
		return "wrong_actor"
	}
	if x.Actor != g.turn.Active {
		return "wrong_timing"
	}
	if x.Kind == "end_turn" {
		if g.turnTransition != "" {
			return "command_in_progress"
		}
		return ""
	}
	if x.Kind == "use_extra_pp" {
		actor := g.player(x.Actor)
		if g.firstPlayer == "" || x.Actor == g.firstPlayer || actor.extraPPActive || g.turn.Number <= 5 && !actor.extraPPEarly || g.turn.Number >= 6 && !actor.extraPPLate {
			return "extra_pp_unavailable"
		}
		return ""
	}
	i := g.instances[x.Source]
	if i == nil {
		return "unknown_alias"
	}
	switch x.Kind {
	case "play":
		actor := g.player(x.Actor)
		if i.zone != "hand" || !contains(actor.hand, i) || actor.pp < i.card.Cost {
			return "cost"
		}
		for _, restriction := range i.card.Restrictions {
			if restriction.Kind == "unplayable" {
				return "unplayable"
			}
		}
		if i.card.CardType != "spell" && len(actor.field) >= fieldLimit {
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
		actor := g.player(x.Actor)
		if i.zone != "field" || !contains(actor.field, i) || i.engaged || a == nil || actor.pp < a.Trigger.(ir.CostTrigger).Cost {
			return "cost"
		}
		sandbox := g.clone()
		sandbox.budget = budget
		sandboxSource := sandbox.instances[i.id]
		sandbox.player(x.Actor).pp -= a.Trigger.(ir.CostTrigger).Cost
		sandboxSource.engaged = true
		return sandbox.preflightRequirements(a.Body, sandboxSource, frame{}, true)
	case "evolve":
		actor := g.player(x.Actor)
		unlockTurn := 5
		if g.firstPlayer != "" && x.Actor != g.firstPlayer {
			unlockTurn = 4
		}
		if i.zone != "field" || !contains(actor.field, i) || i.card.CardType != "follower" || i.evolved || actor.ep < 1 || actor.evolvedThisTurn || g.firstPlayer != "" && g.turn.Number < unlockTurn {
			return "cost"
		}
		if plan := findActionPlan(i.card, "evolve"); plan != nil {
			sandbox := g.clone()
			sandbox.budget = budget
			sandboxSource := sandbox.instances[i.id]
			sandbox.player(x.Actor).ep--
			sandboxSource.evolved = true
			if code := sandbox.preflightPlan(sandboxSource, plan); code != "" {
				return code
			}
		}
	case "superevolve":
		actor := g.player(x.Actor)
		unlockTurn := 7
		if g.firstPlayer != "" && x.Actor != g.firstPlayer {
			unlockTurn = 6
		}
		if i.zone != "field" || !contains(actor.field, i) || i.card.CardType != "follower" || i.evolved || i.superEvolved || actor.sep < 1 || actor.evolvedThisTurn || g.firstPlayer != "" && g.turn.Number < unlockTurn {
			return "cost"
		}
		sandbox := g.clone()
		sandbox.budget = budget
		sandboxSource := sandbox.instances[i.id]
		sandbox.player(x.Actor).sep--
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
	if _, ok := a.(ir.FusionAction); ok {
		return nil, ""
	}
	x, ok := a.(ir.SourceAction)
	if !ok {
		return nil, "unsupported_action"
	}
	if x.Kind == "end_turn" {
		if g.turnTransition != "" {
			return nil, "command_in_progress"
		}
		actor := g.player(x.Actor)
		if actor.extraPPActive {
			actor.pp--
			actor.extraPPActive = false
		}
		g.turnTransition = "ending"
		event := ir.RuntimeEvent{Kind: "turn_ended", Side: g.turn.Active}
		if g.emit(event) {
			g.queueEventTriggers(event, nil, "")
		}
		return nil, ""
	}
	if x.Kind == "use_extra_pp" {
		actor := g.player(x.Actor)
		actor.pp++
		actor.extraPPActive = true
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
	case "evolve":
		return g.commitEvolve(i), ""
	default:
		return nil, "unsupported_action"
	}
}

func (g *game) preflightAttack(a ir.AttackAction) string {
	if a.Actor != "own" && a.Actor != "oppo" {
		return "wrong_actor"
	}
	if a.Actor != g.turn.Active {
		return "wrong_timing"
	}
	actor, opponent := g.player(a.Actor), g.player(oppositeSide(a.Actor))
	attacker := g.instances[a.Attacker]
	if attacker == nil || attacker.zone != "field" || !contains(actor.field, attacker) || attacker.card.CardType != "follower" {
		return "invalid_attacker"
	}
	if attacker.abilities["cannot_attack"] {
		return "attack_restricted"
	}
	if attacker.summoningSick {
		if !attacker.abilities["storm"] && !attacker.abilities["rush"] && !attacker.evolved {
			return "summoning_sick"
		}
	}
	if attacker.attacksUsed >= attackLimit(attacker) {
		return "already_attacked"
	}
	if a.Kind == "attack_leader" {
		if attacker.abilities["cannot_attack_leader"] {
			return "attack_leader_restricted"
		}
		if attacker.summoningSick && (attacker.abilities["rush"] || attacker.evolved) && !attacker.abilities["storm"] {
			return "rush_cannot_attack_leader"
		}
		if hasAttackableWard(opponent) {
			return "ward_required"
		}
		if a.Defender != oppositeSide(a.Actor) {
			return "invalid_defender"
		}
		return ""
	}
	if a.Kind != "attack_entity" {
		return "unsupported_action"
	}
	if attacker.abilities["cannot_attack_follower"] {
		return "attack_follower_restricted"
	}
	defender := g.instances[a.Defender]
	if defender == nil || defender.zone != "field" || !contains(opponent.field, defender) || defender.card.CardType != "follower" {
		return "invalid_defender"
	}
	if defender.abilities["stealth"] {
		return "stealth_target"
	}
	if defender.abilities["intimidate"] {
		return "intimidate_target"
	}
	if hasAttackableWard(opponent) && !defender.abilities["ward"] {
		return "ward_required"
	}
	return ""
}

func hasAttackableWard(p *player) bool {
	for _, i := range p.field {
		if i.card.CardType == "follower" && i.abilities["ward"] && !i.abilities["intimidate"] {
			return true
		}
	}
	return false
}

func (g *game) commitAttack(a ir.AttackAction) string {
	attacker := g.instances[a.Attacker]
	var defender *instance
	if a.Kind == "attack_entity" {
		defender = g.instances[a.Defender]
	}
	attacker.attacksUsed++
	delete(attacker.abilities, "stealth")
	g.player(a.Actor).attackedThisTurn = true
	g.attack = &attackState{stage: "attack", actor: a.Actor, attacker: attacker.id}
	attackerTarget := ir.EventTarget{Kind: "instance", InstanceID: attacker.id}
	opponentSide := oppositeSide(a.Actor)
	defenderTarget := ir.EventTarget{Kind: "leader", Side: opponentSide}
	if defender != nil {
		g.attack.defender = defender.id
		defenderTarget = ir.EventTarget{Kind: "instance", InstanceID: defender.id}
	}
	event := ir.RuntimeEvent{Kind: "attacked", Attacker: &attackerTarget, Defender: &defenderTarget}
	if !g.emit(event) {
		return ""
	}
	g.queueEventTriggers(event, attacker, "attack")
	g.queueSimpleTriggers(attacker, "attack")
	return ""
}

func (g *game) advanceAttack() {
	state := g.attack
	if state == nil {
		return
	}
	if state.stage != "attack" && state.stage != "clash_attacker" && state.stage != "clash_defender" && state.stage != "combat" {
		g.attack = nil
		return
	}
	attacker := g.instances[state.attacker]
	if attacker == nil || attacker.zone != "field" {
		g.attack = nil
		return
	}
	opponentSide := oppositeSide(state.actor)
	if state.stage == "attack" {
		if state.defender == "" {
			state.stage = "combat"
		} else {
			defender := g.instances[state.defender]
			if defender == nil || defender.zone != "field" || !contains(g.player(opponentSide).field, defender) {
				if attacker.superEvolved && defender != nil && defender.zone != "field" {
					g.damageLeader(g.player(opponentSide), opponentSide, 1)
				}
				g.attack = nil
				return
			}
			state.stage = "clash_attacker"
			g.queueSimpleTriggers(attacker, "clash")
			return
		}
	}
	if state.stage == "clash_attacker" {
		defender := g.instances[state.defender]
		if defender == nil || defender.zone != "field" || !contains(g.player(opponentSide).field, defender) {
			if attacker.superEvolved && defender != nil && defender.zone != "field" {
				g.damageLeader(g.player(opponentSide), opponentSide, 1)
			}
			g.attack = nil
			return
		}
		state.stage = "clash_defender"
		g.queueSimpleTriggers(defender, "clash")
		return
	}
	if state.stage == "clash_defender" {
		state.stage = "combat"
	}
	if state.defender == "" {
		actual := g.damageLeader(g.player(opponentSide), opponentSide, attacker.attack)
		if attacker.abilities["drain"] && actual > 0 {
			g.healLeader(g.owner(attacker), state.actor, actual)
		}
		g.attack = nil
		return
	}
	defender := g.instances[state.defender]
	if defender == nil || defender.zone != "field" || !contains(g.player(opponentSide).field, defender) {
		g.attack = nil
		return
	}
	state.attackerAttack, state.defenderAttack = attacker.attack, defender.attack
	defenderDamage := g.damageInstanceFrom(attacker, defender, state.attackerAttack, "combat")
	g.damageInstanceFrom(defender, attacker, state.defenderAttack, "combat")
	if attacker.abilities["drain"] && defenderDamage > 0 {
		g.healLeader(g.owner(attacker), g.sideOf(attacker), defenderDamage)
	}
	if attacker.abilities["bane"] {
		g.destroyByEffect([]*instance{defender})
	}
	if defender.abilities["bane"] {
		g.destroyByEffect([]*instance{attacker})
	}
	g.resolveDeathBatch(nil)
	if attacker.superEvolved && defender.zone != "field" {
		g.damageLeader(g.player(opponentSide), opponentSide, 1)
	}
	g.attack = nil
}

func (g *game) queueSimpleTriggers(source *instance, kind string) {
	if source == nil {
		return
	}
	for _, ability := range source.card.Abilities {
		if ir.TriggerKind(ability.Trigger) == kind {
			g.queueTrigger(triggerInvocation{body: ability.Body, blockID: abilityBlockID(source.card.ID, ability.ID), self: source, bindings: frame{}})
		}
	}
}

func attackLimit(i *instance) int {
	if i != nil && i.attackLimitValue > 0 {
		return i.attackLimitValue
	}
	return 1
}

func (g *game) damageLeader(target *player, side string, amount int) int {
	if amount < 0 {
		amount = 0
	}
	actual := min(amount, target.leaderLife)
	t := ir.EventTarget{Kind: "leader", Side: side}
	event := ir.RuntimeEvent{Kind: "damaged", Side: side, Actual: actual, Target: &t}
	if g.emit(event) {
		target.leaderLife -= actual
		g.queueEventTriggers(event, nil, "")
		if target.leaderLife == 0 {
			g.finishGame(oppositeSide(side))
		}
	}
	return actual
}

func (g *game) damageLeaders(amount int) {
	if amount < 0 {
		amount = 0
	}
	for _, target := range []struct {
		player *player
		side   string
	}{
		{&g.own, "own"},
		{&g.oppo, "oppo"},
	} {
		actual := min(amount, max(target.player.leaderLife, 0))
		eventTarget := &ir.EventTarget{Kind: "leader", Side: target.side}
		event := ir.RuntimeEvent{Kind: "damaged", Side: target.side, Actual: actual, Target: eventTarget}
		if g.emit(event) {
			target.player.leaderLife -= actual
			g.queueEventTriggers(event, nil, "")
		}
	}
	if g.own.leaderLife <= 0 && g.oppo.leaderLife <= 0 {
		g.finishGame(oppositeSide(g.turn.Active))
	} else if g.oppo.leaderLife <= 0 {
		g.finishGame("own")
	} else if g.own.leaderLife <= 0 {
		g.finishGame("oppo")
	}
}

func (g *game) healLeader(target *player, side string, amount int) {
	if amount <= 0 {
		return
	}
	life := target.leaderLife + amount
	if target.leaderMax > 0 {
		life = min(life, target.leaderMax)
	}
	actual := life - target.leaderLife
	t := ir.EventTarget{Kind: "leader", Side: side}
	event := ir.RuntimeEvent{Kind: "healed", Side: side, Actual: actual, Target: &t}
	if actual > 0 && g.emit(event) {
		target.leaderLife = life
		g.queueEventTriggers(event, nil, "")
	}
}

func (g *game) damageInstance(target *instance, amount int) int {
	return g.damageInstanceFrom(nil, target, amount, "effect")
}

func (g *game) damageInstanceFrom(source, target *instance, amount int, damageType string) int {
	if target == nil || target.zone != "field" || amount <= 0 {
		return 0
	}
	actual := g.modifyDamage(damageContext{source: source, target: target, amount: amount, damageType: damageType})
	target.life -= actual
	t := &ir.EventTarget{Kind: "instance", InstanceID: target.id}
	g.emitDamage(actual, t, target)
	if actual > 0 && damageType == "effect" && source != nil && g.sideOf(source) != g.sideOf(target) {
		delete(target.abilities, "stealth")
	}
	return actual
}

func (g *game) modifyDamage(context damageContext) int {
	amount := max(context.amount, 0)
	if context.target.abilities["barrier"] {
		delete(context.target.abilities, "barrier")
		return 0
	}
	if context.target.superEvolved && g.sideOf(context.target) == g.turn.Active {
		return 0
	}
	amount = max(amount-context.target.damageReduction, 0)
	return min(amount, max(context.target.life, 0))
}

func (g *game) destroyByEffect(targets []*instance) {
	allowed := make([]*instance, 0, len(targets))
	for _, target := range targets {
		if target == nil || target.zone != "field" {
			continue
		}
		if target.superEvolved && g.sideOf(target) == g.turn.Active {
			continue
		}
		allowed = append(allowed, target)
	}
	g.resolveDeathBatch(allowed)
}

func (g *game) finishGame(winner string) {
	if g.gameOver {
		return
	}
	g.gameOver, g.winner = true, winner
	g.emit(ir.RuntimeEvent{Kind: "game_ended", Side: winner})
}

func (g *game) emitDamage(amount int, target *ir.EventTarget, subject *instance) {
	if amount < 0 {
		amount = 0
	}
	event := ir.RuntimeEvent{Kind: "damaged", Side: func() string {
		if subject != nil {
			return g.sideOf(subject)
		}
		return target.Side
	}(), Actual: amount, Target: target}
	if g.emit(event) {
		g.queueEventTriggers(event, subject, "")
	}
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
	actor := g.owner(i)
	g.spendPP(actor, i.card.Cost)
	g.remove(&actor.hand, i)
	if i.card.CardType == "spell" {
		g.addToZone(actor, i, "graveyard")
	} else {
		g.addToZone(actor, i, "field")
		i.summoningSick = i.card.CardType == "follower"
		actor.combo++
		g.mergeEarthSigil(i)
		if emit && i.card.CardType == "follower" {
			g.triggerSummoned(i)
		}
	}
}
func (g *game) commitEngage(i *instance) []execFrame {
	a := findAbility(i.card, "engage")
	g.spendPP(g.owner(i), a.Trigger.(ir.CostTrigger).Cost)
	i.engaged = true
	g.triggerEngaged(i)
	return []execFrame{{body: a.Body, blockID: abilityBlockID(i.card.ID, a.ID), self: i, bindings: frame{}}}
}

func (g *game) spendPP(actor *player, amount int) {
	before := actor.pp
	actor.pp -= amount
	if amount <= 0 || !actor.extraPPActive {
		return
	}
	if amount <= before-1 {
		return
	}
	if g.turn.Number <= 5 {
		actor.extraPPEarly = false
	} else {
		actor.extraPPLate = false
	}
	actor.extraPPActive = false
}
func (g *game) commitSuperEvolve(i *instance) []execFrame {
	owner := g.owner(i)
	owner.sep--
	owner.evolvedThisTurn = true
	i.evolved, i.superEvolved = true, true
	i.attack += 3
	i.life += 3
	target := &ir.EventTarget{Kind: "instance", InstanceID: i.id}
	evolved := ir.RuntimeEvent{Kind: "evolved", Side: g.sideOf(i), InstanceID: i.id, CardID: i.card.ID, Subject: target}
	if g.emit(evolved) {
		g.queueEventTriggers(evolved, i, "")
	}
	superEvolved := ir.RuntimeEvent{Kind: "super_evolved", Side: g.sideOf(i), InstanceID: i.id, CardID: i.card.ID, Subject: target}
	if g.emit(superEvolved) {
		g.queueEventTriggers(superEvolved, i, "")
	}
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

func (g *game) commitEvolve(i *instance) []execFrame {
	owner := g.owner(i)
	owner.ep--
	owner.evolvedThisTurn = true
	i.evolved = true
	i.attack += 2
	i.life += 2
	target := &ir.EventTarget{Kind: "instance", InstanceID: i.id}
	event := ir.RuntimeEvent{Kind: "evolved", Side: g.sideOf(i), InstanceID: i.id, CardID: i.card.ID, Subject: target}
	if g.emit(event) {
		g.queueEventTriggers(event, i, "")
	}
	return g.commitActionPlan(i, "evolve")
}

func (g *game) commitActionPlan(i *instance, action string) []execFrame {
	plan := findActionPlan(i.card, action)
	if plan == nil {
		return nil
	}
	abilities := map[string]ir.Ability{}
	for _, a := range i.card.Abilities {
		abilities[a.ID] = a
	}
	var f frame
	var frames []execFrame
	for _, step := range plan.Steps {
		if step.Frame == "new" || f == nil {
			f = frame{}
		}
		a := abilities[step.AbilityID]
		frames = append(frames, execFrame{body: a.Body, blockID: abilityBlockID(i.card.ID, a.ID), self: i, bindings: f})
	}
	return frames
}

func findActionPlan(card *ir.Card, action string) *ir.ActionPlan {
	if card == nil {
		return nil
	}
	for n := range card.ActionPlans {
		if card.ActionPlans[n].Action == action {
			return &card.ActionPlans[n]
		}
	}
	return nil
}

func (g *game) preflightPlan(i *instance, plan *ir.ActionPlan) string {
	abilities := map[string]ir.Ability{}
	for _, a := range i.card.Abilities {
		abilities[a.ID] = a
	}
	for _, step := range plan.Steps {
		if ability := abilities[step.AbilityID]; ability.ID != "" {
			if code := g.preflightRequirements(ability.Body, i, frame{}, true); code != "" {
				return code
			}
		}
	}
	return ""
}

func (g *game) advanceTurn() {
	if g.turnTransition == "ending" {
		if g.turn.Active == "oppo" {
			g.turn.Number++
		}
		g.turn.Active = oppositeSide(g.turn.Active)
		g.turnTransition = "starting"
	}
	startingTriggers := g.turnTransition == "starting_triggers"
	if startingTriggers {
		g.turnTransition = ""
	} else {
		active := g.player(g.turn.Active)
		if active.maxpp < 10 {
			active.maxpp++
		}
		active.pp = active.maxpp
		active.combo = 0
		active.attackedThisTurn = false
		active.evolvedThisTurn = false
		var expired []*instance
		for _, i := range active.field {
			i.engaged = false
			i.attacksUsed = 0
			i.summoningSick = false
			if i.card.CardType == "amulet" && i.countdown > 0 {
				i.countdown--
				if i.countdown == 0 {
					expired = append(expired, i)
				}
			}
		}
		g.resolveDeathBatch(expired)
		if len(g.triggers) > 0 {
			g.turnTransition = "starting_triggers"
			return
		}
	}
	g.draw(ir.DrawEffect{Kind: "draw", Owner: g.turn.Active, Count: 1}, nil, frame{})
	if g.gameOver {
		return
	}
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
