package project

import (
	"wbo/internal/ir"
	"wbo/internal/syntax"
)

// copyRandomIR 解析 `add random N copies from <集合> to hand|deck;`，其中集合不是破坏历史
//（例如 `oppo.hand`、`oppo.deck`）：从来源集合里等概率抽选 N 张，按卡牌定义各复制 1 张。
// 抽选不放回，每条候选消费一次随机决策；来源区的身份不会写进公开事件。
func copyRandomIR(t []syntax.Token, base ir.NodeBase) (ir.CopyRandomEffect, bool) {
	e := ir.CopyRandomEffect{NodeBase: base, Kind: "copy_random", Owner: "own", Output: "added"}
	if len(t) < 9 || t[0].Value != "add" || t[1].Value != "random" || !isUnsigned(t[2]) ||
		t[3].Value != "copies" || t[4].Value != "from" {
		return e, false
	}
	e.Count = intToken(t[2])
	if e.Count < 1 || e.Count > 65535 {
		return e, false
	}
	source, end := setExprIR(t, 5)
	if source == nil {
		return e, false
	}
	if end < len(t) && t[end].Value == "where" {
		predicate, next := filterIR(t, end)
		source = ir.FilterRef{Kind: "filter", Source: source, Predicate: predicate}
		end = next
	}
	if end+2 != len(t) || t[end].Value != "to" || !set("hand", "deck")[t[end+1].Value] {
		return e, false
	}
	e.Source, e.Destination = source, t[end+1].Value
	return e, true
}
