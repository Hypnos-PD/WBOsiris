package runner

import (
	"fmt"
	"maps"
	"sort"

	"wbo/internal/ir"
)

func (s *Session) Mulligan(side string, selected []string) error {
	if s == nil || s.g == nil || side != "own" && side != "oppo" {
		return fmt.Errorf("invalid mulligan player")
	}
	player := s.g.player(side)
	wanted := map[string]bool{}
	for _, id := range selected {
		if wanted[id] {
			return fmt.Errorf("duplicate mulligan card")
		}
		card := s.g.instances[id]
		if card == nil || !contains(player.hand, card) {
			return fmt.Errorf("invalid mulligan card")
		}
		wanted[id] = true
	}
	if len(player.deck) < len(selected) {
		return fmt.Errorf("not enough cards for mulligan")
	}
	kept, returned := make([]*instance, 0, len(player.hand)), make([]*instance, 0, len(selected))
	for _, card := range player.hand {
		if wanted[card.id] {
			returned = append(returned, card)
		} else {
			kept = append(kept, card)
		}
	}
	player.hand = kept
	for range returned {
		card := player.deck[0]
		player.deck = player.deck[1:]
		card.zone = "hand"
		player.hand = append(player.hand, card)
	}
	for _, card := range returned {
		card.zone = "deck"
		player.deck = append(player.deck, card)
	}
	for n := len(player.deck) - 1; n > 0; n-- {
		other := s.g.rng.Index(n + 1)
		player.deck[n], player.deck[other] = player.deck[other], player.deck[n]
	}
	s.g.revision++
	return nil
}

// ValidateMulligan checks a private redraw without changing the session.
func (s *Session) ValidateMulligan(side string, selected []string) error {
	if s == nil || s.g == nil || side != "own" && side != "oppo" {
		return fmt.Errorf("invalid mulligan player")
	}
	player := s.g.player(side)
	wanted := map[string]bool{}
	for _, id := range selected {
		if wanted[id] {
			return fmt.Errorf("duplicate mulligan card")
		}
		card := s.g.instances[id]
		if card == nil || !contains(player.hand, card) {
			return fmt.Errorf("invalid mulligan card")
		}
		wanted[id] = true
	}
	if len(player.deck) < len(selected) {
		return fmt.Errorf("not enough cards for mulligan")
	}
	return nil
}

// StartMatch resolves the first player's initial turn draw after both mulligans.
func (s *Session) StartMatch() StepResult {
	if s == nil || s.g == nil || s.g.gameOver {
		return StepResult{Status: StatusRejected, ErrorCode: "game_over"}
	}
	if s.actionID != "" || s.pending != nil || len(s.stack) != 0 {
		return StepResult{Status: StatusRejected, ErrorCode: "command_in_progress"}
	}
	s.actionID = deriveRuntimeID("match_start", s.g.turn.Active, fmt.Sprint(s.g.revision))
	s.requestOrdinal = 0
	s.g.legal, s.g.illegal, s.g.unchanged = true, "", false
	s.g.draw(ir.DrawEffect{Kind: "draw", Owner: s.g.turn.Active, Count: 1}, nil, frame{})
	event := ir.RuntimeEvent{Kind: "turn_started", Side: s.g.turn.Active}
	if s.g.emit(event) {
		s.g.queueEventTriggers(event, nil, "")
	}
	s.g.revision++
	return s.run()
}

type SimulatorCapabilities struct {
	Play         bool `json:"play"`
	Engage       bool `json:"engage"`
	SuperEvolve  bool `json:"superEvolve"`
	TargetChoice bool `json:"targetChoice"`
	ModeChoice   bool `json:"modeChoice"`
	Evolve       bool `json:"evolve"`
	Attack       bool `json:"attack"`
	EndTurn      bool `json:"endTurn"`
	ExtraPP      bool `json:"extraPP"`
}

type SimulatorCommand struct {
	Kind     string `json:"kind"`
	Actor    string `json:"actor,omitempty"`
	Source   string `json:"source,omitempty"`
	Defender string `json:"defender,omitempty"`
}

type LegalAction struct {
	Kind     string `json:"kind"`
	Actor    string `json:"actor"`
	Source   string `json:"source,omitempty"`
	Defender string `json:"defender,omitempty"`
}

type StateView struct {
	Turn          TurnView       `json:"turn"`
	Phase         string         `json:"phase"`
	Revision      uint64         `json:"revision"`
	Viewer        string         `json:"viewer"`
	Own           PlayerView     `json:"own"`
	Oppo          PlayerView     `json:"oppo"`
	PendingChoice *ChoiceRequest `json:"pendingChoice,omitempty"`
	GameOver      bool           `json:"gameOver"`
	Winner        string         `json:"winner,omitempty"`
}

func (s *Session) Events() []ir.RuntimeEvent {
	if s == nil || s.g == nil {
		return nil
	}
	return append([]ir.RuntimeEvent(nil), s.g.events...)
}

// Redact in place so cumulative replay frame event counts remain valid.
func (s *Session) EventsFor(viewer string) []ir.RuntimeEvent {
	events := s.Events()
	for n, event := range events {
		if event.PrivateTo != "" && event.PrivateTo != viewer {
			events[n] = ir.RuntimeEvent{Kind: event.Kind, Side: event.Side, Count: event.Count, Sequence: event.Sequence}
		}
	}
	return events
}

type TurnView struct {
	Active string `json:"active"`
	Number int    `json:"number"`
}

type PlayerView struct {
	LeaderLife       int          `json:"leaderLife"`
	LeaderMax        int          `json:"leaderMax"`
	PP               int          `json:"pp"`
	MaxPP            int          `json:"maxpp"`
	EP               int          `json:"ep"`
	SEP              int          `json:"sep"`
	Combo            int          `json:"combo"`
	Shadows          int          `json:"shadows"`
	ExtraPPAvailable bool         `json:"extraPPAvailable"`
	ExtraPPUses      int          `json:"extraPPUses"`
	ExtraPPActive    bool         `json:"extraPPActive"`
	AttackedThisTurn bool         `json:"attackedThisTurn"`
	DeckCount        int          `json:"deckCount"`
	HandCount        int          `json:"handCount"`
	Hand             []EntityView `json:"hand,omitempty"`
	Field            []EntityView `json:"field"`
	Graveyard        []EntityView `json:"graveyard"`
	Resolving        []EntityView `json:"resolving,omitempty"`
	Banished         []EntityView `json:"banished"`
	Destroyed        []EntityView `json:"destroyed"`
}

type EntityView struct {
	Counters        map[string]int `json:"counters,omitempty"`
	Fusion          *FusionView    `json:"fusion,omitempty"`
	InstanceID      string         `json:"instanceId"`
	Alias           string         `json:"alias,omitempty"`
	CardID          int            `json:"cardId"`
	Cost            int            `json:"cost"`
	CardType        string         `json:"cardType"`
	Attack          int            `json:"attack"`
	Life            int            `json:"life,omitempty"`
	Countdown       int            `json:"countdown,omitempty"`
	Earthsigil      int            `json:"earthsigil,omitempty"`
	DamageReduction int            `json:"damageReduction,omitempty"`
	Engaged         bool           `json:"engaged,omitempty"`
	AttacksUsed     int            `json:"attacksUsed"`
	AttackLimit     int            `json:"attackLimit"`
	SummoningSick   bool           `json:"summoningSick"`
	Evolved         bool           `json:"evolved,omitempty"`
	SuperEvolved    bool           `json:"superEvolved,omitempty"`
	Keywords        []string       `json:"keywords,omitempty"`
	Traits          []string       `json:"traits,omitempty"`
}

type FusionView struct {
	Enabled       bool                 `json:"enabled"`
	UsedThisTurn  bool                 `json:"usedThisTurn"`
	TotalCost     int                  `json:"totalCost"`
	DistinctKinds int                  `json:"distinctKinds"`
	Materials     []FusionMaterialView `json:"materials"`
}

type FusionMaterialView struct {
	CardID int `json:"cardId"`
	Cost   int `json:"cost"`
}

// SupportedSimulatorCapabilities 返回运行时当前真正支持的交互范围。
func SupportedSimulatorCapabilities() SimulatorCapabilities {
	return SimulatorCapabilities{Play: true, Engage: true, SuperEvolve: true, Evolve: true, TargetChoice: true, ModeChoice: true, Attack: true, EndTurn: true, ExtraPP: true}
}

// Submit 接受模拟器命令，未定义的规则动作会明确拒绝。
func (s *Session) Submit(actionID string, command SimulatorCommand) StepResult {
	return s.SubmitAs(actionID, "own", command)
}

func (s *Session) SubmitAs(actionID, actor string, command SimulatorCommand) StepResult {
	if actor != "own" && actor != "oppo" {
		return StepResult{Status: StatusRejected, ErrorCode: "invalid_actor"}
	}
	switch command.Kind {
	case "play", "engage", "evolve", "superevolve", "fusion", "end_turn", "use_extra_pp":
		if command.Kind == "fusion" {
			return s.Begin(actionID, ir.FusionAction{Kind: "fusion", Actor: actor, Source: command.Source})
		}
		return s.Begin(actionID, ir.SourceAction{Kind: command.Kind, Actor: actor, Source: command.Source})
	case "attack":
		kind, defender := "attack_leader", command.Defender
		if defender != "" {
			kind = "attack_entity"
		}
		return s.Begin(actionID, ir.AttackAction{Kind: kind, Actor: actor, Attacker: command.Source, Defender: func() string {
			if defender == "" {
				return oppositeSide(actor)
			}
			return defender
		}()})
	default:
		return StepResult{Status: StatusRejected, ErrorCode: "unsupported_feature"}
	}
}

// LegalActions 只列出已经通过完整合法性预检的主动作。
func (s *Session) LegalActions() []LegalAction {
	return s.LegalActionsFor("own")
}

func (s *Session) LegalActionsFor(actor string) []LegalAction {
	if s == nil || s.g == nil || s.actionID != "" || s.pending != nil || len(s.stack) != 0 || s.fault != "" || s.g.turn.Active != actor || s.g.phase != "main" || (actor != "own" && actor != "oppo") {
		return []LegalAction{}
	}
	player, opponent := s.g.player(actor), s.g.player(oppositeSide(actor))
	actions := make([]LegalAction, 0)
	add := func(kind string, source *instance) {
		var budget budgetTracker
		budget.reset(s.budgetPolicy)
		action := ir.SourceAction{Kind: kind, Actor: actor, Source: source.id}
		if code := s.g.preflight(action, &budget); code == "" && !budget.exceeded {
			actions = append(actions, LegalAction{Kind: kind, Actor: actor, Source: source.id})
		}
	}
	for _, source := range player.hand {
		add("play", source)
	}
	for _, source := range player.field {
		add("engage", source)
	}
	for _, source := range player.field {
		add("evolve", source)
	}
	for _, source := range player.hand {
		var budget budgetTracker
		budget.reset(s.budgetPolicy)
		if code := s.g.preflight(ir.FusionAction{Kind: "fusion", Actor: actor, Source: source.id}, &budget); code == "" && !budget.exceeded {
			actions = append(actions, LegalAction{Kind: "fusion", Actor: actor, Source: source.id})
		}
	}
	for _, source := range player.field {
		add("superevolve", source)
	}
	var extraBudget budgetTracker
	extraBudget.reset(s.budgetPolicy)
	if code := s.g.preflight(ir.SourceAction{Kind: "use_extra_pp", Actor: actor}, &extraBudget); code == "" && !extraBudget.exceeded {
		actions = append(actions, LegalAction{Kind: "use_extra_pp", Actor: actor})
	}
	for _, source := range player.field {
		attack := ir.AttackAction{Kind: "attack_leader", Actor: actor, Attacker: source.id, Defender: oppositeSide(actor)}
		var budget budgetTracker
		budget.reset(s.budgetPolicy)
		if code := s.g.preflight(attack, &budget); code == "" && !budget.exceeded {
			actions = append(actions, LegalAction{Kind: "attack_leader", Actor: actor, Source: source.id, Defender: oppositeSide(actor)})
		}
		for _, target := range opponent.field {
			attack.Kind, attack.Defender = "attack_entity", target.id
			budget.reset(s.budgetPolicy)
			if code := s.g.preflight(attack, &budget); code == "" && !budget.exceeded {
				actions = append(actions, LegalAction{Kind: "attack_entity", Actor: actor, Source: source.id, Defender: target.id})
			}
		}
	}
	if s.g.preflight(ir.SourceAction{Kind: "end_turn", Actor: actor}, &budgetTracker{}) == "" {
		actions = append(actions, LegalAction{Kind: "end_turn", Actor: actor})
	}
	return actions
}

// View 是给模拟器界面使用的状态投影，不会泄露对手手牌内容。
func (s *Session) View(viewer string) (StateView, error) {
	if s == nil || s.g == nil {
		return StateView{}, fmt.Errorf("session is required")
	}
	if viewer != "own" && viewer != "oppo" {
		return StateView{}, fmt.Errorf("viewer must be own or oppo")
	}
	own, oppo := &s.g.own, &s.g.oppo
	active := s.g.turn.Active
	winner := s.g.winner
	if viewer == "oppo" {
		own, oppo = oppo, own
		if active == "own" {
			active = "oppo"
		} else {
			active = "own"
		}
		if winner == "own" {
			winner = "oppo"
		} else if winner == "oppo" {
			winner = "own"
		}
	}
	return StateView{
		Turn: TurnView{Active: active, Number: s.g.turn.Number}, Phase: s.g.phase,
		Revision: s.g.revision, Viewer: viewer,
		GameOver: s.g.gameOver, Winner: winner,
		Own: playerView(own, true, s.g.turn.Number, viewer, s.g.firstPlayer), Oppo: playerView(oppo, false, s.g.turn.Number, oppositeSide(viewer), s.g.firstPlayer), PendingChoice: s.pendingChoiceFor(viewer),
	}, nil
}

func (s *Session) pendingChoiceFor(viewer string) *ChoiceRequest {
	request := s.PendingChoice()
	if request == nil || request.PublicTo != viewer {
		return nil
	}
	request.PublicTo = "own"
	return request
}

func playerView(p *player, revealHand bool, turn int, side, firstPlayer string) PlayerView {
	view := PlayerView{
		LeaderLife: p.leaderLife, LeaderMax: p.leaderMax, PP: p.pp, MaxPP: p.maxpp,
		EP: p.ep, SEP: p.sep, Combo: p.combo, Shadows: p.shadows, AttackedThisTurn: p.attackedThisTurn,
		DeckCount: len(p.deck), HandCount: len(p.hand), Field: entityViews(p.field, revealHand),
		Graveyard: entityViews(p.graveyard, revealHand), Banished: entityViews(p.banished, revealHand), Destroyed: entityViews(p.destroyed, revealHand),
		Resolving: entityViews(p.resolving, revealHand),
	}
	if firstPlayer != "" && side != firstPlayer {
		view.ExtraPPAvailable = !p.extraPPActive && (p.extraPPEarly || turn >= 6 && p.extraPPLate)
		view.ExtraPPActive = p.extraPPActive
		if p.extraPPEarly || turn >= 6 && p.extraPPLate {
			view.ExtraPPUses = 1
		}
	}
	if revealHand {
		view.Hand = entityViews(p.hand, true)
	}
	return view
}

func entityViews(instances []*instance, revealMaterials bool) []EntityView {
	views := make([]EntityView, 0, len(instances))
	for _, i := range instances {
		keywords := make([]string, 0, len(i.abilities))
		for keyword, enabled := range i.abilities {
			if enabled {
				keywords = append(keywords, keyword)
			}
		}
		for _, ability := range i.card.Abilities {
			kind := ir.TriggerKind(ability.Trigger)
			if kind == "lastwords" {
				keywords = appendUnique(keywords, "lastwords")
			} else if kind == "engage" {
				keywords = appendUnique(keywords, "engage")
			} else if kind != "fanfare" && kind != "evolve" && kind != "superevolve" {
				keywords = appendUnique(keywords, "triggered")
			}
		}
		sort.Strings(keywords)
		traits := append([]string(nil), i.card.Traits...)
		if i.departed {
			traits = appendUnique(traits, "departed")
		}
		views = append(views, EntityView{
			Counters:   maps.Clone(i.counters),
			Fusion:     fusionView(i, revealMaterials),
			InstanceID: i.id, Alias: i.alias, CardID: i.card.ID, CardType: i.card.CardType, Cost: i.cost,
			Attack: i.attack, Life: i.life, Countdown: i.countdown, Earthsigil: i.earthsigil, DamageReduction: i.damageReduction,
			Engaged: i.engaged, AttacksUsed: i.attacksUsed, AttackLimit: attackLimit(i), SummoningSick: i.summoningSick,
			Evolved: i.evolved, SuperEvolved: i.superEvolved, Keywords: keywords, Traits: traits,
		})
	}
	return views
}

func fusionView(i *instance, reveal bool) *FusionView {
	if !reveal || len(i.card.FusionAbilities) == 0 && len(i.materials) == 0 {
		return nil
	}
	view := &FusionView{Enabled: len(i.card.FusionAbilities) > 0, UsedThisTurn: i.fusedThisTurn, Materials: []FusionMaterialView{}}
	kinds := map[int]bool{}
	for _, material := range i.materials {
		view.Materials = append(view.Materials, FusionMaterialView{CardID: material.card.ID, Cost: material.card.Cost})
		view.TotalCost += material.card.Cost
		kinds[material.card.ID] = true
	}
	view.DistinctKinds = len(kinds)
	return view
}

func appendUnique(items []string, value string) []string {
	for _, item := range items {
		if item == value {
			return items
		}
	}
	return append(items, value)
}
