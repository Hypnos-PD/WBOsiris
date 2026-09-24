// Package search 是"搜索住在 Go 里"的落脚点：决策时的推演不经过 JSON 协议，
// 直接在进程内克隆会话、走子、评估。训练侧只通过网络批量提供先验与价值。
//
// 这一层目前只有最小可用的克隆与随机推演，用来回答一个问题：
// 在真实引擎成本下，一次推演要多久、搜索到底有没有预算可用。
package search

import (
	"fmt"

	"wbo/internal/engine/ai"
	"wbo/internal/engine/ir"
	"wbo/internal/engine/runner"
)

// Clone 复制一个会话：走引擎自己的续局序列化（存档 → 读档），
// 这样克隆出来的会话与"重开一份存档"完全同路径，不需要另写一份深拷贝逻辑。
func Clone(cards *ir.CardPack, session *runner.Session) (*runner.Session, error) {
	if session == nil {
		return nil, fmt.Errorf("session is required")
	}
	data, err := session.EncodeContinuation()
	if err != nil {
		return nil, fmt.Errorf("encode continuation: %w", err)
	}
	continuation, err := runner.DecodeContinuation(data)
	if err != nil {
		return nil, fmt.Errorf("decode continuation: %w", err)
	}
	clone, err := runner.RestoreSession(cards, continuation)
	if err != nil {
		return nil, fmt.Errorf("restore session: %w", err)
	}
	return clone, nil
}

// PlayoutResult 是一次随机推演的结果。
type PlayoutResult struct {
	Winner  string
	Reward  float64 // 站在 own 视角：胜 +1 / 负 -1 / 平 0
	Turns   int
	Actions int
	Choices int
}

// RandomPlayout 从当前局面克隆一份，双方都用随机合法动作打到终局。
// 它的用途是给搜索一个基准：能跑多快、方差多大。
func RandomPlayout(cards *ir.CardPack, session *runner.Session, seed uint64) (PlayoutResult, error) {
	clone, err := Clone(cards, session)
	if err != nil {
		return PlayoutResult{}, err
	}
	own := ai.NewDriver("own", ai.NewRandom(seed*2+1), ai.Limits{})
	oppo := ai.NewDriver("oppo", ai.NewRandom(seed*2+2), ai.Limits{})
	for step := 0; step < 4000; step++ {
		view, err := clone.View("own")
		if err != nil {
			return PlayoutResult{}, err
		}
		if view.GameOver {
			result := PlayoutResult{
				Winner:  view.Winner,
				Turns:   view.Turn.Number,
				Actions: own.Actions() + oppo.Actions(),
				Choices: own.Choices() + oppo.Choices(),
			}
			switch view.Winner {
			case "own":
				result.Reward = 1
			case "oppo":
				result.Reward = -1
			}
			return result, nil
		}
		actedOwn, err := own.Step(clone)
		if err != nil {
			return PlayoutResult{}, err
		}
		actedOppo, err := oppo.Step(clone)
		if err != nil {
			return PlayoutResult{}, err
		}
		if !actedOwn && !actedOppo {
			return PlayoutResult{}, fmt.Errorf("推演卡住：双方都无法行动（回合 %d）", view.Turn.Number)
		}
	}
	return PlayoutResult{}, fmt.Errorf("推演超过步数上限")
}
