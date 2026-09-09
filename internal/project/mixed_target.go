package project

import "wbo/internal/syntax"

func validateMixedBindings(body []*syntax.Statement, inherited map[string]bool, ds *[]syntax.Diagnostic) map[string]bool {
	mixed := map[string]bool{}
	for name, value := range inherited {
		mixed[name] = value
	}
	var evolved map[string]bool
	for _, s := range body {
		if s.Word(0) == "evolve" && len(s.Blocks()) == 1 {
			evolved = validateMixedBindings(s.Blocks()[0], mixed, ds)
			break
		}
	}
	for _, s := range body {
		t := s.Tokens()
		if len(t) == 0 {
			continue
		}
		h := s.Word(0)
		if targets, batch := destructionBatchTargets(t); batch {
			for _, target := range targets {
				if mixed[target.Value] {
					diag(ds, "WBO-E008-TYPE-MISMATCH", "错误", "批次破坏不能包含主战者混合绑定", target.Span)
				}
			}
		}
		if set("choose", "require", "random")[h] && len(t) > 1 {
			mixed[t[1].Value] = len(t) >= 12 && t[8].Value == "or"
			continue
		}
		index := 1
		switch h {
		case "set", "reduce":
			index = 2
		case "add", "summon", "remove":
			index = 3
		}
		if (isPlainOperation(s) || h == "grant") && index < len(t) && mixed[t[index].Value] {
			end, valid := parseEffectAmount(t, index+1)
			if h != "damage" && h != "heal" || !valid || end != len(t) {
				diag(ds, "WBO-E008-TYPE-MISMATCH", "错误", "随从或主战者的混合目标仅支持不带筛选的 damage 或 heal；count 可统计绑定数量", s.Span)
			}
		}
		branchMixed := map[string]bool{}
		for _, block := range s.Blocks() {
			parent := mixed
			if h == "grant" {
				parent = nil
			} else if h == "superevolve" && s.Word(1) == "extends" {
				parent = evolved
			}
			if h != "evolve" {
				result := validateMixedBindings(block, parent, ds)
				if set("if", "mode", "option", "earthrite", "necromancy")[h] {
					for name, value := range result {
						branchMixed[name] = branchMixed[name] || value
					}
				}
			}
		}
		for name, value := range branchMixed {
			mixed[name] = mixed[name] || value
		}
		for name := range producedBindings([]*syntax.Statement{s}) {
			delete(mixed, name)
		}
	}
	return mixed
}
