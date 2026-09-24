package ai

import (
	"wbo/internal/engine/ruleset"
	"wbo/internal/engine/runner"
)

// Random 只在合法动作里均匀随机，用作品尺：它总是"能走完一局"的最低标准，
// 也是压测引擎时最容易撞出异常局面的策略。
type Random struct {
	rng *ruleset.RNG
}

func NewRandom(seed uint64) *Random {
	return &Random{rng: ruleset.NewRNG(seed)}
}

func (p *Random) Name() string { return "random" }

func (p *Random) Choose(_ runner.StateView, actions []runner.LegalAction) (runner.LegalAction, bool) {
	if len(actions) == 0 {
		return runner.LegalAction{}, false
	}
	return actions[p.rng.Index(len(actions))], true
}

func (p *Random) Answer(_ runner.StateView, request runner.ChoiceRequest) runner.ChoiceResponse {
	response := newResponse(request)
	need := request.MinSelections
	if need <= 0 {
		return response
	}
	pool := make([]runner.ChoiceCandidate, len(request.Candidates))
	copy(pool, request.Candidates)
	for len(pool) > 0 && len(response.SelectedInstanceIDs)+len(response.SelectedLeaderSides)+len(response.SelectedOptionIDs) < need {
		index := p.rng.Index(len(pool))
		candidate := pool[index]
		pool = append(pool[:index], pool[index+1:]...)
		appendSelection(&response, candidate)
	}
	return response
}

func newResponse(request runner.ChoiceRequest) runner.ChoiceResponse {
	return runner.ChoiceResponse{RequestID: request.RequestID, ActionID: request.ActionID, StateRevision: request.StateRevision}
}

func appendSelection(response *runner.ChoiceResponse, candidate runner.ChoiceCandidate) {
	switch {
	case candidate.InstanceID != "":
		response.SelectedInstanceIDs = append(response.SelectedInstanceIDs, candidate.InstanceID)
	case candidate.LeaderSide != "":
		response.SelectedLeaderSides = append(response.SelectedLeaderSides, candidate.LeaderSide)
	default:
		response.SelectedOptionIDs = append(response.SelectedOptionIDs, candidate.OptionID)
	}
}
