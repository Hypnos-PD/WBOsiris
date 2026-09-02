package runner

import (
	"fmt"
	"sort"

	"wbo/internal/ir"
)

type SimulatorCapabilities struct {
	Play         bool `json:"play"`
	Engage       bool `json:"engage"`
	SuperEvolve  bool `json:"superEvolve"`
	TargetChoice bool `json:"targetChoice"`
	ModeChoice   bool `json:"modeChoice"`
	Evolve       bool `json:"evolve"`
	Attack       bool `json:"attack"`
	EndTurn      bool `json:"endTurn"`
}

type SimulatorCommand struct {
	Kind   string `json:"kind"`
	Source string `json:"source,omitempty"`
}

type LegalAction struct {
	Kind   string `json:"kind"`
	Actor  string `json:"actor"`
	Source string `json:"source,omitempty"`
}

type StateView struct {
	Turn          TurnView       `json:"turn"`
	Phase         string         `json:"phase"`
	Revision      uint64         `json:"revision"`
	Viewer        string         `json:"viewer"`
	Own           PlayerView     `json:"own"`
	Oppo          PlayerView     `json:"oppo"`
	PendingChoice *ChoiceRequest `json:"pendingChoice,omitempty"`
}

type TurnView struct {
	Active string `json:"active"`
	Number int    `json:"number"`
}

type PlayerView struct {
	LeaderLife int          `json:"leaderLife"`
	LeaderMax  int          `json:"leaderMax"`
	PP         int          `json:"pp"`
	MaxPP      int          `json:"maxpp"`
	EP         int          `json:"ep"`
	SEP        int          `json:"sep"`
	Combo      int          `json:"combo"`
	Shadows    int          `json:"shadows"`
	DeckCount  int          `json:"deckCount"`
	HandCount  int          `json:"handCount"`
	Hand       []EntityView `json:"hand,omitempty"`
	Field      []EntityView `json:"field"`
	Graveyard  []EntityView `json:"graveyard"`
	Banished   []EntityView `json:"banished"`
	Destroyed  []EntityView `json:"destroyed"`
}

type EntityView struct {
	InstanceID   string   `json:"instanceId"`
	Alias        string   `json:"alias,omitempty"`
	CardID       int      `json:"cardId"`
	CardType     string   `json:"cardType"`
	Attack       int      `json:"attack,omitempty"`
	Life         int      `json:"life,omitempty"`
	Countdown    int      `json:"countdown,omitempty"`
	Earthsigil   int      `json:"earthsigil,omitempty"`
	Engaged      bool     `json:"engaged,omitempty"`
	Evolved      bool     `json:"evolved,omitempty"`
	SuperEvolved bool     `json:"superEvolved,omitempty"`
	Keywords     []string `json:"keywords,omitempty"`
}

// SupportedSimulatorCapabilities 返回运行时当前真正支持的交互范围。
func SupportedSimulatorCapabilities() SimulatorCapabilities {
	return SimulatorCapabilities{Play: true, Engage: true, SuperEvolve: true, TargetChoice: true, ModeChoice: true, EndTurn: true}
}

// Submit 接受模拟器命令，未定义的规则动作会明确拒绝。
func (s *Session) Submit(actionID string, command SimulatorCommand) StepResult {
	switch command.Kind {
	case "play", "engage", "superevolve", "end_turn":
		return s.Begin(actionID, ir.SourceAction{Kind: command.Kind, Actor: "own", Source: command.Source})
	default:
		return StepResult{Status: StatusRejected, ErrorCode: "unsupported_feature"}
	}
}

// LegalActions 只列出已经通过完整合法性预检的主动作。
func (s *Session) LegalActions() []LegalAction {
	if s == nil || s.g == nil || s.actionID != "" || s.pending != nil || len(s.stack) != 0 || s.fault != "" || s.g.turn.Active != "own" || s.g.phase != "main" {
		return []LegalAction{}
	}
	actions := make([]LegalAction, 0)
	add := func(kind string, source *instance) {
		var budget budgetTracker
		budget.reset(s.budgetPolicy)
		action := ir.SourceAction{Kind: kind, Actor: "own", Source: source.id}
		if code := s.g.preflight(action, &budget); code == "" && !budget.exceeded {
			actions = append(actions, LegalAction{Kind: kind, Actor: "own", Source: source.id})
		}
	}
	for _, source := range s.g.own.hand {
		add("play", source)
	}
	for _, source := range s.g.own.field {
		add("engage", source)
	}
	for _, source := range s.g.own.field {
		add("superevolve", source)
	}
	if s.g.preflight(ir.SourceAction{Kind: "end_turn", Actor: "own"}, &budgetTracker{}) == "" {
		actions = append(actions, LegalAction{Kind: "end_turn", Actor: "own"})
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
	if viewer == "oppo" {
		own, oppo = oppo, own
		if active == "own" {
			active = "oppo"
		} else {
			active = "own"
		}
	}
	return StateView{
		Turn: TurnView{Active: active, Number: s.g.turn.Number}, Phase: s.g.phase,
		Revision: s.g.revision, Viewer: viewer,
		Own: playerView(own, true), Oppo: playerView(oppo, false), PendingChoice: s.pendingChoiceFor(viewer),
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

func playerView(p *player, revealHand bool) PlayerView {
	view := PlayerView{
		LeaderLife: p.leaderLife, LeaderMax: p.leaderMax, PP: p.pp, MaxPP: p.maxpp,
		EP: p.ep, SEP: p.sep, Combo: p.combo, Shadows: p.shadows,
		DeckCount: len(p.deck), HandCount: len(p.hand), Field: entityViews(p.field),
		Graveyard: entityViews(p.graveyard), Banished: entityViews(p.banished), Destroyed: entityViews(p.destroyed),
	}
	if revealHand {
		view.Hand = entityViews(p.hand)
	}
	return view
}

func entityViews(instances []*instance) []EntityView {
	views := make([]EntityView, 0, len(instances))
	for _, i := range instances {
		keywords := make([]string, 0, len(i.abilities))
		for keyword, enabled := range i.abilities {
			if enabled {
				keywords = append(keywords, keyword)
			}
		}
		sort.Strings(keywords)
		views = append(views, EntityView{
			InstanceID: i.id, Alias: i.alias, CardID: i.card.ID, CardType: i.card.CardType,
			Attack: i.attack, Life: i.life, Countdown: i.countdown, Earthsigil: i.earthsigil,
			Engaged: i.engaged, Evolved: i.evolved, SuperEvolved: i.superEvolved, Keywords: keywords,
		})
	}
	return views
}
