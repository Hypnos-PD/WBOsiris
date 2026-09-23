package runner

import (
	"fmt"
	"maps"

	"wbo/internal/ir"
	"wbo/internal/ruleset"
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

// Determinize 把**只有隐藏信息不同**的一个等价局面做出来：在"未知槽位"之间重新分配卡牌。
//
// 为什么需要它（见 WBDecima 的 docs/determinized-search-design.md）：
// `Clone()` 出来的局面里，对手手牌内容、双方牌库顺序都是真值，于是搜索在模拟里"偷看真牌"——
// 叶子估值偏乐观、给策略的目标也是"看见了答案才给出的"。确定化把搜索改成
// "用与真值同分布、但不知道具体是哪一种的状态"，这才是不完全信息下正确的做法。
//
// 重新分配的槽位（**只动隐藏的**）：
//
//   - 对手手牌：内容未知 ⇒ 与对手牌库一起做**联合置换**（两者张数不变、合计多重集不变）；
//   - 己方牌库：内容是公开信息（构筑时就知道），顺序未知 ⇒ **只置换顺序**；
//   - 场上/墓地/放逐/破坏/护符与一切计数：全部**逐位不动**（公开信息必须不变）。
//
// 种子由调用方给，同一个种子 ⇒ 同一个确定化样本（可复现）。
func (s *Session) Determinize(seed uint64) error {
	if s == nil || s.g == nil {
		return fmt.Errorf("session is required")
	}
	rng := ruleset.NewRNG(seed)

	// 对手手牌与对手牌库：联合置换（在"哪些牌在手上、哪些还在牌库"这件事上重新采样）。
	// 注意两者的**张数**都不变，只是内容重新分配——公开信息里只有手牌张数。
	oppo := s.g.oppo
	if len(oppo.hand) > 0 || len(oppo.deck) > 0 {
		pool := make([]*instance, 0, len(oppo.hand)+len(oppo.deck))
		pool = append(pool, oppo.hand...)
		pool = append(pool, oppo.deck...)
		shuffleInstances(rng, pool)
		copy(oppo.hand, pool[:len(oppo.hand)])
		copy(oppo.deck, pool[len(oppo.hand):])
	}
	// 己方牌库顺序：自己知道牌库**内容**，但不知道顺序 ⇒ 只打乱顺序。
	shuffleInstances(rng, s.g.own.deck)
	return nil
}

func shuffleInstances(rng *ruleset.RNG, items []*instance) {
	// Fisher–Yates：与引擎其它洗牌（deck_replace.go）同一个 RNG，保证可复现。
	for i := len(items) - 1; i > 0; i-- {
		j := rng.Index(i + 1)
		if i != j {
			items[i], items[j] = items[j], items[i]
		}
	}
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
