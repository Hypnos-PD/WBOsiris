package ai

import (
	"fmt"

	"wbo/internal/engine/runner"
)

// Limits 是驱动一个策略时的安全上限：即使策略写错了，也必须能停下来，
// 否则批量自对弈会挂在这里。
type Limits struct {
	// MaxSteps 是"连续为同一方推进"的最大步数（一个主动作或一次选择算一步）。
	MaxSteps int
	// MaxTurns 是一局的最大回合数（超过就判为异常局面）。
	MaxTurns int
	// MaxActions 是一局的最大动作数（含双方）。
	MaxActions int
}

func (l Limits) withDefaults() Limits {
	if l.MaxSteps <= 0 {
		l.MaxSteps = 500
	}
	if l.MaxTurns <= 0 {
		l.MaxTurns = 120
	}
	if l.MaxActions <= 0 {
		l.MaxActions = 5000
	}
	return l
}

// Driver 用一个策略驱动引擎里的某一方。
type Driver struct {
	Policy   Policy
	Side     string
	Limits   Limits
	actionID uint64
	actions  int
	choices  int
}

func NewDriver(side string, policy Policy, limits Limits) *Driver {
	return &Driver{Policy: policy, Side: side, Limits: limits.withDefaults()}
}

func (d *Driver) Actions() int { return d.actions }
func (d *Driver) Choices() int { return d.choices }

func (d *Driver) nextActionID() string {
	d.actionID++
	return fmt.Sprintf("%032x", d.actionID)
}

// Step 执行一步。acted=false 表示现在轮不到这一方（对局结束、轮到对手，
// 或者待处理的选择属于对手）。
func (d *Driver) Step(session *runner.Session) (acted bool, err error) {
	if session == nil {
		return false, fmt.Errorf("session is required")
	}
	view, err := session.View(d.Side)
	if err != nil {
		return false, err
	}
	if view.GameOver {
		return false, nil
	}
	if pending := session.PendingChoice(); pending != nil {
		if pending.PublicTo != d.Side {
			return false, nil
		}
		result := session.Resume(d.Policy.Answer(view, *pending))
		if !acceptable(result) {
			return false, fmt.Errorf("%s: choice rejected (%s)", d.Policy.Name(), describe(result))
		}
		d.choices++
		return true, nil
	}
	// 视图里的 Turn.Active 是"观察者视角"的：对任何观察者来说，自己的回合都是 own。
	if view.Turn.Active != "own" || view.Phase != "main" {
		return false, nil
	}
	actions := session.LegalActionsFor(d.Side)
	if len(actions) == 0 {
		return false, fmt.Errorf("%s: no legal actions on its own turn", d.Policy.Name())
	}
	action, ok := d.Policy.Choose(view, actions)
	if !ok {
		return false, errNoAction
	}
	result := session.SubmitAs(d.nextActionID(), d.Side, CommandFor(action))
	if !acceptable(result) {
		return false, fmt.Errorf("%s: action %s rejected (%s)", d.Policy.Name(), action.Kind, describe(result))
	}
	d.actions++
	return true, nil
}

// acceptable 判断一次提交是否"被引擎接受了"：suspended 表示引擎在等这一步
// 引发的选择请求，属于正常流程。
func acceptable(result runner.StepResult) bool {
	return result.Status == runner.StatusCompleted || result.Status == runner.StatusSuspended
}

// RunUntilBlocked 反复执行，直到轮不到这一方（或对局结束）。
func (d *Driver) RunUntilBlocked(session *runner.Session) (int, error) {
	steps := 0
	for steps < d.Limits.MaxSteps {
		acted, err := d.Step(session)
		if err != nil {
			return steps, err
		}
		if !acted {
			return steps, nil
		}
		steps++
	}
	return steps, fmt.Errorf("driver for %s exceeded %d consecutive steps", d.Side, d.Limits.MaxSteps)
}

// MatchResult 是一局自对弈的结果。Fault 非空表示这一局以异常结束。
type MatchResult struct {
	Winner      string
	Turns       int
	Actions     int
	Choices     int
	Fault       string
	FirstPlayer string
}

// KeepAllMulligan 不作换牌：开场就保留全部起手。
func KeepAllMulligan(session *runner.Session) error {
	for _, side := range []string{"own", "oppo"} {
		if err := session.Mulligan(side, nil); err != nil {
			return err
		}
	}
	if result := session.StartMatch(); result.Status != runner.StatusCompleted {
		return fmt.Errorf("match start failed (%s)", describe(result))
	}
	return nil
}

// PlayMatch 让两个策略对打一局，直到分出胜负或触发上限。
// 返回的 error 只用于"引擎/策略层面出错"，对局本身的异常（超时）记在 Fault 里。
func PlayMatch(session *runner.Session, own, oppo Policy, limits Limits) (MatchResult, error) {
	limits = limits.withDefaults()
	result := MatchResult{}
	view, err := session.View("own")
	if err != nil {
		return result, err
	}
	result.FirstPlayer = view.FirstPlayer
	drivers := map[string]*Driver{"own": NewDriver("own", own, limits), "oppo": NewDriver("oppo", oppo, limits)}
	if err := KeepAllMulligan(session); err != nil {
		return result, err
	}
	for {
		view, err := session.View("own")
		if err != nil {
			return result, err
		}
		result.Turns = view.Turn.Number
		if view.GameOver {
			result.Winner = view.Winner
			return result, nil
		}
		if view.Turn.Number > limits.MaxTurns {
			result.Fault = fmt.Sprintf("turn limit %d reached without a winner", limits.MaxTurns)
			return result, nil
		}
		side := view.Turn.Active
		if pending := session.PendingChoice(); pending != nil {
			side = pending.PublicTo
		}
		driver := drivers[side]
		if driver == nil {
			return result, fmt.Errorf("unknown side %q", side)
		}
		before := view.Revision
		acted, err := driver.Step(session)
		if err != nil {
			return result, err
		}
		if !acted {
			return result, fmt.Errorf("side %s cannot act at revision %d", side, before)
		}
		result.Actions, result.Choices = drivers["own"].Actions()+drivers["oppo"].Actions(), drivers["own"].Choices()+drivers["oppo"].Choices()
		if result.Actions+result.Choices > limits.MaxActions {
			result.Fault = fmt.Sprintf("action limit %d reached without a winner", limits.MaxActions)
			return result, nil
		}
		if after, err := session.View("own"); err == nil && after.Revision == before && session.PendingChoice() == nil {
			return result, fmt.Errorf("no progress at revision %d (side %s)", before, side)
		}
	}
}
