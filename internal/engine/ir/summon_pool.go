package ir

// SummonPoolEffect 表示"从几种指定卡中随机召唤 N 个"，
// 例如"在自己的战场上召唤随机1个『纯洁白狐』或『神圣猎鹰』"。
type SummonPoolEffect struct {
	NodeBase
	Kind   string `json:"kind"`
	Owner  string `json:"owner"`
	Count  int    `json:"count"`
	Pool   []int  `json:"pool"`
	Output string `json:"output"`
}

func (e SummonPoolEffect) effectKind() string   { return e.Kind }
func (e SummonPoolEffect) effectBase() NodeBase { return e.NodeBase }

// ValidSummonPool 要求至少两种互不相同的卡牌定义与 1..65535 的召唤数量。
func ValidSummonPool(pool []int, count int) bool {
	if count < 1 || count > 65535 || len(pool) < 2 || len(pool) > 16 {
		return false
	}
	seen := map[int]bool{}
	for _, id := range pool {
		if !validCardID(id) || seen[id] {
			return false
		}
		seen[id] = true
	}
	return true
}
