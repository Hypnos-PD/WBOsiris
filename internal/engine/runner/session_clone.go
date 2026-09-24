package runner

import (
	"fmt"
	"maps"

	"wbo/internal/engine/ir"
	"wbo/internal/engine/ruleset"
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

// Believe 用**外部给定的**对手隐藏牌多重集重排对手的隐藏槽位。
//
// 与 Determinize 的区别只有一处，但那一处正是"信息集"的全部内容：
// Determinize 用的是引擎里的**真值多重集**——它知道"对手手牌+牌库里一共还剩这些牌"
// （上帝视角）；Believe 用调用方给的多重集，那一份应该来自**公开证据 + 卡组先验**
// 推断出来的范围（range）：一个真实玩家能看到的就是这些。
//
// 张数必须与对手隐藏区当前张数一致（手牌 + 牌库），否则直接报错——调用方算错了要早失败，
// 不能悄悄改张数（张数是公开信息，改了就破坏了"公开信息逐位不变"这条硬要求）。
func (s *Session) Believe(seed uint64, cards []int) error {
	if s == nil || s.g == nil {
		return fmt.Errorf("session is required")
	}
	oppo := s.g.oppo
	hidden := len(oppo.hand) + len(oppo.deck)
	if len(cards) != hidden {
		return fmt.Errorf("对手隐藏区有 %d 张，给定 %d 张", hidden, len(cards))
	}
	// 先按种子把这些牌洗一遍：哪几张进手牌、哪几张留在牌库是随机的（张数固定）。
	order := make([]int, len(cards))
	copy(order, cards)
	rng := ruleset.NewRNG(seed)
	for i := len(order) - 1; i > 0; i-- {
		j := rng.Index(i + 1)
		if i != j {
			order[i], order[j] = order[j], order[i]
		}
	}
	// 复用现有实例（保持 id/位置），只换它们承载的卡牌并重置卡牌派生状态。
	// 不新建实例：实例身份在引擎里被触发器、附着材料等引用，换掉容易留下悬挂引用。
	instances := make([]*instance, 0, hidden)
	instances = append(instances, oppo.hand...)
	instances = append(instances, oppo.deck...)
	for index, item := range instances {
		card := s.g.cards[order[index]]
		if card == nil {
			return fmt.Errorf("卡牌 %d 不在卡池里", order[index])
		}
		resetCardState(item, card)
	}
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
