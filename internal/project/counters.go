package project

import (
	"wbo/internal/ir"
	"wbo/internal/syntax"
)

func counterRef(t []syntax.Token, i int) bool {
	return i+4 < len(t) && t[i].Value == "self" && t[i+1].Value == "." && t[i+2].Value == "counter" && t[i+3].Value == "." && t[i+4].Kind == syntax.Identifier && ir.ValidCounterName(t[i+4].Value)
}

func validateCounters(c *Card, ds *[]syntax.Diagnostic) {
	names := map[string]bool{}
	for _, s := range c.Effect {
		if s.Word(0) != "counter" {
			continue
		}
		t := s.Tokens()
		if len(t) != 3 || t[1].Kind != syntax.Identifier || !ir.ValidCounterName(t[1].Value) || !isUnsigned(t[2]) || !s.Terminated || len(s.Blocks()) != 0 {
			shapeError(ds, s, "counter 名称 初始非负整数;")
			continue
		}
		if names[t[1].Value] {
			shapeError(ds, s, "counter 名称不能重复")
		}
		names[t[1].Value] = true
	}
	check := func(name string, span syntax.Span) {
		if !names[name] {
			diag(ds, "WBO-E008-TYPE-MISMATCH", "错误", "计数器未声明: "+name, span)
		}
	}
	var walk func([]*syntax.Statement)
	walk = func(body []*syntax.Statement) {
		for _, s := range body {
			t := s.Tokens()
			if len(t) == 4 && t[0].Value == "add" && t[2].Value == "counter" {
				check(t[3].Value, t[3].Span)
			}
			for n := range t {
				if counterRef(t, n) {
					check(t[n+4].Value, t[n+4].Span)
				}
			}
			for _, block := range s.Blocks() {
				walk(block)
			}
		}
	}
	walk(c.Effect)
}

func validateScenarioCounters(scenario *syntax.Statement, cards map[string]*Card, ds *[]syntax.Diagnostic) {
	aliases := map[string]*Card{}
	declared := func(card *Card, name string) bool {
		if card == nil {
			return true
		}
		for _, effect := range card.Effect {
			if effect.Word(0) == "counter" && effect.Word(1) == name {
				return true
			}
		}
		return false
	}
	var walk func(*syntax.Statement, bool)
	walk = func(s *syntax.Statement, declarations bool) {
		t := s.Tokens()
		if declarations && len(t) == 4 && cardTypes[t[0].Value] && t[2].Value == "=" {
			card := cards[t[3].Value]
			aliases[t[1].Value] = card
			for _, block := range s.Blocks() {
				for _, o := range block {
					if o.Word(0) == "counter" && !declared(card, o.Word(1)) {
						shapeError(ds, o, "counter override 必须引用卡牌已声明的计数器")
					}
				}
			}
		}
		if !declarations && len(t) >= 5 && t[1].Value == "." && t[2].Value == "counter" && t[3].Value == "." && !declared(aliases[t[0].Value], t[4].Value) {
			shapeError(ds, s, "counter 断言必须引用卡牌已声明的计数器")
		}
		for _, block := range s.Blocks() {
			for _, child := range block {
				walk(child, declarations)
			}
		}
	}
	walk(scenario, true)
	walk(scenario, false)
}
