// Package ai 提供可替换的对局策略：既能驱动服务端的练习模式，
// 也能在批跑（自对弈）里当压测器和将来的训练基线。
//
// 协议上的约定：合法动作（runner.LegalAction）的词表与命令
// （runner.SimulatorCommand）的词表并不完全相同——攻击在合法动作里是
// attack_leader / attack_entity，提交时要写成 attack + 可选的 defender。
// CommandFor 负责这层翻译，是"策略决定"到"引擎命令"的唯一出口。
package ai

import (
	"errors"
	"fmt"

	"wbo/internal/runner"
)

// Policy 是唯一需要替换的东西：决定主动作，并回答引擎抛出的选择请求。
type Policy interface {
	Name() string
	// Choose 从合法主动作里挑一个。返回 false 表示放弃（调用方应当视为异常）。
	Choose(view runner.StateView, actions []runner.LegalAction) (runner.LegalAction, bool)
	// Answer 回答一个待处理的选择请求。
	Answer(view runner.StateView, request runner.ChoiceRequest) runner.ChoiceResponse
}

// CommandFor 把合法动作翻译成引擎命令。
func CommandFor(action runner.LegalAction) runner.SimulatorCommand {
	switch action.Kind {
	case "attack_leader":
		return runner.SimulatorCommand{Kind: "attack", Source: action.Source}
	case "attack_entity":
		return runner.SimulatorCommand{Kind: "attack", Source: action.Source, Defender: action.Defender}
	default:
		return runner.SimulatorCommand{Kind: action.Kind, Source: action.Source, Defender: action.Defender}
	}
}

// entity 按实例 ID 在视图里找出随从（自己的手牌/战场、对手的战场）。
// found 为 false 时调用方应当退回保守估计。
func entity(view runner.StateView, instanceID string) (runner.EntityView, bool) {
	for _, zones := range [][]runner.EntityView{view.Own.Hand, view.Own.Field, view.Oppo.Field} {
		for _, item := range zones {
			if item.InstanceID == instanceID {
				return item, true
			}
		}
	}
	return runner.EntityView{}, false
}

// opponentLeaderSide 返回"从这一侧看过去的对手主战者"的引擎座位名。
// 视图里的 Viewer 是引擎座位（own/oppo），候选里的 LeaderSide 也是引擎座位。
func opponentLeaderSide(view runner.StateView) string {
	if view.Viewer == "oppo" {
		return "own"
	}
	return "oppo"
}

func describe(result runner.StepResult) string {
	if result.ErrorCode != "" {
		return fmt.Sprintf("%s/%s", result.Status, result.ErrorCode)
	}
	return string(result.Status)
}

var errNoAction = errors.New("policy produced no legal action")
