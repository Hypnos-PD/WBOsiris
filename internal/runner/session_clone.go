package runner

import (
	"fmt"
	"maps"

	"wbo/internal/ir"
)

// Clone 深拷贝一份会话：原生实现，给"从任意决策点分叉"的树搜索用。
//
// 为什么不走续局（EncodeContinuation/RestoreSession）：续局只表达**干净的决策点**，
// 战斗与结算的中间态表达不了（恢复时报 invalid continuation combat state）；
// 而且一次 5ms，搜索每层都要克隆。原生拷贝没有这些限制，目标 <100µs。
//
// 拷贝规则：编译产物（ir.Card / ir.Effect / ir.Ability / 卡池索引）共享（只读）；
// 运行期状态全部复制。底下的 `(*game).clone()` 会重建触发器索引，实例指针按 id 重映射，
// 保证"同一个实例在克隆里仍然只有一个"。
func (s *Session) Clone() (*Session, error) {
	if s == nil {
		return nil, fmt.Errorf("session is required")
	}
	out := *s
	out.g = s.g.clone()
	out.stack = cloneExecFrames(s.stack)
	out.pending = clonePendingChoice(s.pending)
	if s.blocks != nil {
		out.blocks = maps.Clone(s.blocks)
	}
	return &out, nil
}

func cloneExecFrames(in []execFrame) []execFrame {
	if in == nil {
		return nil
	}
	out := make([]execFrame, len(in))
	for index, item := range in {
		out[index] = item
		out[index].bindings = cloneBindings(item.bindings)
		out[index].repeatBindings = cloneBindings(item.repeatBindings)
	}
	return out
}

func clonePendingChoice(in *pendingChoice) *pendingChoice {
	if in == nil {
		return nil
	}
	out := *in
	out.bindings = cloneBindings(in.bindings)
	if in.options != nil {
		out.options = make(map[int][]ir.Effect, len(in.options))
		for key, value := range in.options {
			out.options[key] = value // 编译产物，只读
		}
	}
	out.optionBlockIDs = maps.Clone(in.optionBlockIDs)
	return &out
}

// bindings（frame = map[string][]ir.EventTarget）里 EventTarget 全是值字段，
// 复制两层容器即可，元素没有指针要重映射。
func cloneBindings(in frame) frame {
	if in == nil {
		return nil
	}
	out := make(frame, len(in))
	for key, items := range in {
		out[key] = append([]ir.EventTarget(nil), items...)
	}
	return out
}
