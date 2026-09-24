package ai

import (
	"sort"

	"wbo/internal/engine/runner"
)

// Greedy 是一个"能打完一局"的启发式：优先斩杀，其次解场换怪，然后尽量把
// 能量点花光（大费优先），进化留给当回合能攻击的随从；选择请求按
// "能杀的先杀、自己的随从挑大的、模式按编号从小到大"来回答。
//
// 它不追求强度：它的价值是稳定的陪练与压测器，同时充当以后 RL 策略的基线。
type Greedy struct{}

func (p *Greedy) Name() string { return "greedy" }

func (p *Greedy) Choose(view runner.StateView, actions []runner.LegalAction) (runner.LegalAction, bool) {
	best, bestScore, found := runner.LegalAction{}, 0, false
	for _, action := range actions {
		score := p.score(view, action)
		if !found || score > bestScore {
			best, bestScore, found = action, score, true
		}
	}
	if !found {
		return runner.LegalAction{}, false
	}
	return best, true
}

func (p *Greedy) score(view runner.StateView, action runner.LegalAction) int {
	switch action.Kind {
	case "attack_leader":
		me, ok := entity(view, action.Source)
		if !ok {
			return 1
		}
		damage := me.Attack
		if damage >= view.Oppo.LeaderLife {
			return lethalScore + damage
		}
		if damage <= 0 {
			return 0
		}
		return 100 + damage*2
	case "attack_entity":
		me, mine := entity(view, action.Source)
		them, theirs := entity(view, action.Defender)
		if !mine || !theirs {
			return 1
		}
		theirValue := them.Attack + them.Life
		myValue := me.Attack + me.Life
		kills := me.Attack >= them.Life
		survives := them.Attack < me.Life
		switch {
		case kills && survives:
			return 400 + theirValue*2 - myValue
		case kills:
			// 同归于尽也可以接受，但不如白吃。
			return 250 + theirValue*2 - myValue
		case survives:
			// 打不死但能磨掉一些生命值。
			return 120 + me.Attack - theirValue
		default:
			return 10
		}
	case "play", "accelerate", "crystallize", "fusion", "engage":
		item, ok := entity(view, action.Source)
		if !ok {
			return 1
		}
		return 50 + item.Cost*5
	case "evolve", "superevolve":
		item, ok := entity(view, action.Source)
		if !ok {
			return 1
		}
		budget := 5 + item.Cost
		if item.AttackLimit > item.AttacksUsed && !item.SummoningSick && item.Attack > 0 {
			budget += 120
		}
		if action.Kind == "superevolve" {
			budget += 40
		}
		return budget
	case "use_extra_pp":
		// 只有当额外的能量点能解锁一张现在打不出的手牌时才值得用
		//（合法动作里只有"现在付得起"的出牌，所以要看手牌的费用）。
		for _, item := range view.Own.Hand {
			if item.Cost == view.Own.PP+1 {
				return 400
			}
		}
		// 解锁不了任何东西时，它比结束回合更差（额外能量点留着没有收益）。
		return -5
	case "end_turn":
		return 0
	}
	return 1
}

const lethalScore = 100000

func (p *Greedy) Answer(view runner.StateView, request runner.ChoiceRequest) runner.ChoiceResponse {
	response := newResponse(request)
	need := request.MinSelections
	if need <= 0 {
		return response
	}
	candidates := append([]runner.ChoiceCandidate(nil), request.Candidates...)
	sort.SliceStable(candidates, func(i, j int) bool {
		return p.candidateScore(view, candidates[i]) > p.candidateScore(view, candidates[j])
	})
	for _, candidate := range candidates {
		if len(response.SelectedInstanceIDs)+len(response.SelectedLeaderSides)+len(response.SelectedOptionIDs) >= need {
			break
		}
		appendSelection(&response, candidate)
	}
	return response
}

func (p *Greedy) candidateScore(view runner.StateView, candidate runner.ChoiceCandidate) int {
	switch {
	case candidate.OptionID != 0 || candidate.InstanceID == "" && candidate.LeaderSide == "":
		// 模式选项：编号越小越先发动，保持确定性且容易解释。
		return 1000 - candidate.OptionID
	case candidate.LeaderSide != "":
		if candidate.LeaderSide == opponentLeaderSide(view) {
			return 2000
		}
		return 100
	default:
		item, ok := entity(view, candidate.InstanceID)
		if !ok {
			return 0
		}
		for _, mine := range append(append([]runner.EntityView{}, view.Own.Hand...), view.Own.Field...) {
			if mine.InstanceID == candidate.InstanceID {
				// 自己的目标：优先选攻击力大的（强化）或生命值低的（代价小）。
				return 500 + item.Attack*2 - item.Life
			}
		}
		// 对手的目标：能杀的优先，其次威胁大的。
		return 1500 + item.Attack*2 - item.Life
	}
}
