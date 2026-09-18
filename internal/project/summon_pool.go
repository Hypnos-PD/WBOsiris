package project

import (
	"wbo/internal/ir"
	"wbo/internal/syntax"
)

// summonPoolTokens 匹配 `summon random N card C1 or card C2 [...] [for own|oppo]`
// （"召唤随机1个『A』或『B』"）。第二个返回值是词法末尾位置。
func summonPoolTokens(t []syntax.Token) (int, bool) {
	if len(t) < 8 || t[1].Value != "random" || !isUnsigned(t[2]) || t[3].Value != "card" || !isCardID(t[4]) {
		return 0, false
	}
	end := 5
	for end+2 < len(t) && t[end].Value == "or" && t[end+1].Value == "card" && isCardID(t[end+2]) {
		end += 3
	}
	if end < len(t) && t[end].Value == "for" {
		if end+1 >= len(t) || !set("own", "oppo")[t[end+1].Value] {
			return 0, false
		}
		end += 2
	}
	return end, end == len(t) && end > 5
}

func summonPoolIR(t []syntax.Token, base ir.NodeBase) (ir.SummonPoolEffect, bool) {
	if _, ok := summonPoolTokens(t); !ok {
		return ir.SummonPoolEffect{}, false
	}
	owner := "own"
	pool := []int{}
	for n := 3; n < len(t); n++ {
		if t[n].Value == "card" {
			pool = append(pool, intToken(t[n+1]))
			n++
		} else if t[n].Value == "for" {
			owner = t[n+1].Value
			break
		}
	}
	return ir.SummonPoolEffect{NodeBase: base, Kind: "summon_random_pool", Owner: owner, Count: intToken(t[2]), Pool: pool, Output: "summoned"}, true
}
