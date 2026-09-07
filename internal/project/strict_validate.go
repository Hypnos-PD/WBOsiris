package project

import (
	"math"
	"strconv"
	"wbo/internal/ir"

	"wbo/internal/syntax"
)

type effectContext struct {
	cardType   string
	top        bool
	fusion     bool
	spellboost bool
}

func strictValidateCard(c *Card, ds *[]syntax.Diagnostic) {
	validateCounters(c, ds)
	validateMixedBindings(c.Effect, nil, ds)
	if c.Cost < 0 || c.Cost > math.MaxUint16 {
		diag(ds, "WBO-E014-INTEGER-RANGE", "错误", "cost 超出 u16 范围", c.Decl.Span)
	}
	if c.Stats != nil && (c.Stats[0] > math.MaxInt16 || c.Stats[1] > math.MaxInt16) {
		diag(ds, "WBO-E014-INTEGER-RANGE", "错误", "stats 超出 i16 范围", c.Decl.Span)
	}
	relations, evolves := 0, 0
	for _, s := range c.Effect {
		if s.Word(0) == "evolve" && len(s.Blocks()) == 1 {
			evolves++
		}
		if s.Word(0) == "superevolve" && len(s.Tokens()) == 3 && len(s.Blocks()) == 1 {
			relations++
		}
	}
	if relations > 1 {
		diag(ds, "WBO-E011-INVALID-RELATION", "错误", "同一卡牌不可重复声明 superevolve relation", c.Decl.Span)
	}
	if evolves > 1 {
		diag(ds, "WBO-E011-INVALID-RELATION", "错误", "普通 evolve 能力不可重复", c.Decl.Span)
	}
	if relations > 0 && evolves != 1 {
		diag(ds, "WBO-E011-INVALID-RELATION", "错误", "superevolve relation 必须对应唯一普通 evolve", c.Decl.Span)
	}
	strictEffectBlock(c.Effect, effectContext{cardType: c.Type, top: true}, ds)
}

func strictEffectBlock(body []*syntax.Statement, ctx effectContext, ds *[]syntax.Diagnostic) {
	for _, s := range body {
		t, b := s.Tokens(), s.Blocks()
		if len(t) == 0 {
			continue
		}
		h := t[0].Value
		if ctx.cardType != "follower" && set("damage", "heal", "buff", "repeat", "set")[h] {
			for n := 1; n+2 < len(t); n++ {
				if t[n].Value == "self" && t[n+1].Value == "." && set("attack", "life")[t[n+2].Value] {
					diag(ds, "WBO-E008-TYPE-MISMATCH", "错误", "self.attack 与 self.life 只允许用于随从", s.Span)
				}
			}
		}
		if ctx.cardType != "follower" && h == "set" && len(t) > 2 && t[2].Value == "self" {
			diag(ds, "WBO-E008-TYPE-MISMATCH", "错误", "set life self 只允许用于随从", s.Span)
		}
		if abilities[h] && len(b) == 0 && ctx.cardType != "follower" {
			diag(ds, "WBO-E012-INVALID-TRIGGER", "错误", h+" 固有能力只允许用于随从", s.Span)
		}
		if set("fanfare", "lastwords", "replace")[h] && ctx.cardType == "spell" {
			diag(ds, "WBO-E012-INVALID-TRIGGER", "错误", h+" 需要场上实体，不能用于法术", s.Span)
		}
		if (h == "countdown" || h == "earthsigil" || h == "engage") && ctx.cardType != "amulet" {
			diag(ds, "WBO-E012-INVALID-TRIGGER", "错误", h+" 只允许用于护符", s.Span)
		}
		if (h == "attack" || h == "clash" || (h == "evolve" || h == "superevolve") && len(b) == 1) && ctx.cardType != "follower" {
			diag(ds, "WBO-E012-INVALID-TRIGGER", "错误", h+" 只允许用于随从", s.Span)
		}
		if ctx.top && ctx.cardType != "spell" && (isPlainOperation(s) || h == "repeat" || h == "grant") {
			diag(ds, "WBO-E012-INVALID-TRIGGER", "错误", "随从或护符的最外层操作没有执行时点", s.Span)
		}
		if h == "transform" && !ctx.fusion && !ctx.spellboost {
			diag(ds, "WBO-E012-INVALID-TRIGGER", "错误", "transform 只允许出现在 fusion 或 spellboost 块内", s.Span)
		}
		if h == "transform" && ctx.fusion && !ctx.spellboost && len(t) == 5 {
			shapeError(ds, s, "fusion 内的 transform 必须显式 preserving materials")
		}
		if h == "transform" && (len(t) < 2 || t[1].Value != "self") {
			diag(ds, "WBO-E008-TYPE-MISMATCH", "错误", "transform 当前只允许以 self 为目标", s.Span)
		}
		switch h {
		case "grant":
			if _, ability, err := grantParts(s); err == nil {
				valid := ability.Word(0) == "lastwords"
				if ability.Word(0) == "when" {
					end, _, ok := parseEventPattern(ability.Tokens())
					valid = ok && end == len(ability.Tokens()) && ir.ValidGrantedTrigger(eventPatternIR(ability.Tokens()))
				}
				if !valid || len(ability.Blocks()) != 1 || repeatHasRequire(ability.Blocks()[0]) {
					shapeError(ds, s, "附加能力只允许 lastwords 或回合开始/结束触发，不允许 require")
				}
				strictEffectBlock([]*syntax.Statement{ability}, effectContext{cardType: "follower"}, ds)
			}
			continue
		case "counter":
			if !ctx.top {
				shapeError(ds, s, "counter 只能声明在 effect 最外层")
			}
			continue
		case "repeat":
			for _, block := range b {
				if repeatHasRequire(block) {
					diag(ds, "WBO-E008-TYPE-MISMATCH", "错误", "repeat 内不能使用前置校验 require；执行时选择使用 choose", s.Span)
				}
			}
		case "countdown":
			if len(t) == 2 {
				checkU16(t[1], true, ds)
			}
		case "engage", "enhance", "earthrite", "necromancy":
			if len(t) == 2 {
				checkU16(t[1], true, ds)
			}
		case "fusion":
			ok := len(t) >= 6 && values(t[:6]) == "fusion material from own . hand"
			end := 6
			if ok && end < len(t) {
				filterOK := !filterContains(t[end:], "life")
				var whereOK bool
				end, whereOK = parseWhere(t, end)
				ok = filterOK && whereOK
			}
			if !ok || end != len(t) || len(b) != 1 || s.Terminated {
				shapeError(ds, s, "fusion material from own.hand [where 完整筛选] { ... }")
			} else {
				strictEffectBlock(b[0], effectContext{cardType: ctx.cardType, fusion: true}, ds)
			}
			continue
		case "when":
			if len(t) > 2 && t[1].Value == "self" && t[2].Value != "discarded" && ctx.cardType != "follower" {
				diag(ds, "WBO-E012-INVALID-TRIGGER", "错误", "自身进化事件只允许用于随从", s.Span)
			}
			end, subject, ok := parseEventPattern(t)
			if ok && end < len(t) {
				filterOK := subject != "" && (subject == "follower" || !filterContains(t[end:], "life"))
				var whereOK bool
				end, whereOK = parseWhere(t, end)
				ok = filterOK && whereOK
			}
			if !ok || end != len(t) || len(b) != 1 || s.Terminated {
				shapeError(ds, s, "when 规范事件 [where 完整筛选] { ... }")
			} else {
				strictEffectBlock(b[0], effectContext{cardType: ctx.cardType}, ds)
			}
			continue
		case "if":
			cond := t[1:]
			hasElse := false
			for i, x := range cond {
				if x.Value == "else" {
					hasElse = true
					cond = cond[:i]
					if i != len(t[1:])-1 {
						shapeError(ds, s, "if 条件 { ... } [else { ... }]")
					}
					break
				}
			}
			if !strictCondition(cond, ctx.fusion) || len(b) < 1 || len(b) > 2 || hasElse != (len(b) == 2) {
				shapeError(ds, s, "if 合法条件 { ... } [else { ... }]")
			}
			for _, bb := range b {
				strictEffectBlock(bb, effectContext{cardType: ctx.cardType, fusion: ctx.fusion, spellboost: ctx.spellboost}, ds)
			}
			continue
		case "mode":
			if len(t) != 1 || len(b) != 1 || len(b[0]) < 2 || s.Terminated {
				shapeError(ds, s, "mode { 至少两个合法 option }")
				continue
			}
			seen := map[uint64]bool{}
			for _, o := range b[0] {
				ot := o.Tokens()
				if len(ot) != 2 || ot[0].Value != "option" || len(o.Blocks()) != 1 || o.Terminated {
					shapeError(ds, o, "option 正整数 { ... }")
					continue
				}
				n, ok := uintValue(ot[1], 16)
				if !ok || n == 0 {
					rangeError(ds, ot[1])
				} else if seen[n] {
					diag(ds, "WBO-E010-DUPLICATE-OPTION", "错误", "mode 选项编号重复", o.Span)
				}
				seen[n] = true
				_, body, err := modeOptionParts(o.Blocks()[0])
				if err != nil {
					shapeError(ds, o, err.Error())
				} else {
					strictEffectBlock(body, effectContext{cardType: ctx.cardType, fusion: ctx.fusion, spellboost: ctx.spellboost}, ds)
				}
			}
			continue
		}
		for _, x := range t {
			if x.Kind == syntax.Integer && !isCardID(x) {
				if h == "add" && len(t) == 4 && t[2].Value == "counter" {
					if !isUnsigned(x) {
						rangeError(ds, x)
					}
				} else {
					checkU16(x, true, ds)
				}
			}
		}
		if h == "buff" {
			end := valueRefEnd(t, 1)
			if end < len(t) && t[end].Value == "other" {
				end++
			}
			for part := 0; part < 2 && end+1 < len(t); part++ {
				if t[end+1].Kind == syntax.Integer {
					checkI16Magnitude(t[end+1], ds)
				}
				next, ok := parseSignedAmount(t, end)
				if !ok {
					break
				}
				end = next + 1
			}
		}
		for _, bb := range b {
			strictEffectBlock(bb, effectContext{cardType: ctx.cardType, fusion: ctx.fusion, spellboost: ctx.spellboost || h == "spellboost"}, ds)
		}
	}
}

func repeatHasRequire(body []*syntax.Statement) bool {
	for _, s := range body {
		if s.Word(0) == "require" {
			return true
		}
		for _, block := range s.Blocks() {
			if repeatHasRequire(block) {
				return true
			}
		}
	}
	return false
}

func checkI16Magnitude(t syntax.Token, ds *[]syntax.Diagnostic) {
	n, ok := uintValue(t, 16)
	if !ok || n > math.MaxInt16 {
		rangeError(ds, t)
	}
}

func isPlainOperation(s *syntax.Statement) bool {
	return len(s.Blocks()) == 0 && set("draw", "add", "summon", "damage", "heal", "buff", "gain", "restore", "destroy", "banish", "discard", "remove", "return", "evolve", "superevolve", "reanimate", "reduce", "spellboost", "transform", "set_attack_limit", "set")[s.Word(0)]
}
func parseEventPattern(t []syntax.Token) (int, string, bool) {
	end, subject, ok := parseBaseEventPattern(t)
	if ok && end < len(t) && t[end].Value == "while" {
		if t[1].Value == "self" || len(t) < end+4 || values(t[end:end+3]) != "while self in" || !set("field", "hand")[t[end+3].Value] {
			return 0, "", false
		}
		end += 4
	}
	if ok && end < len(t) && t[end].Value == "once" {
		if t[1].Value == "self" || end+2 >= len(t) || t[end+1].Value != "per" {
			return 0, "", false
		}
		end += 2
		if set("own", "oppo")[t[end].Value] {
			end++
		}
		if end >= len(t) || t[end].Value != "turn" {
			return 0, "", false
		}
		end++
	}
	return end, subject, ok
}

func parseBaseEventPattern(t []syntax.Token) (int, string, bool) {
	if len(t) == 3 && values(t) == "when self discarded" {
		return 3, "card", true
	}
	if len(t) == 3 && t[0].Value == "when" && t[1].Value == "self" && set("evolved", "super_evolved")[t[2].Value] {
		return 3, "follower", true
	}
	if len(t) < 4 || t[0].Value != "when" || !set("own", "oppo")[t[1].Value] {
		return 0, "", false
	}
	if t[2].Value == "turn" && set("starts", "ends")[t[3].Value] {
		return 4, "", true
	}
	if set("follower", "amulet")[t[2].Value] && set("summoned", "engaged")[t[3].Value] {
		return 4, t[2].Value, true
	}
	if t[2].Value == "card" && set("discarded", "fused")[t[3].Value] {
		return 4, "card", true
	}
	if len(t) >= 5 && values(t[2:5]) == "follower leaves field" {
		return 5, "follower", true
	}
	return 0, "", false
}
func filterContains(t []syntax.Token, field string) bool {
	for _, x := range t {
		if x.Value == field {
			return true
		}
	}
	return false
}
func strictCondition(t []syntax.Token, fusion bool) bool {
	if evolutionCondition(t) {
		return true
	}
	if len(t) == 1 {
		return t[0].Value == "overflow"
	}
	op := func(x string) bool { return set("==", "!=", "<", "<=", ">", ">=")[x] }
	if len(t) == 7 && counterRef(t, 0) {
		return op(t[5].Value) && isUnsigned(t[6])
	}
	if len(t) == 3 {
		return t[0].Value == "combo" && op(t[1].Value) && isUnsigned(t[2])
	}
	return len(t) == 5 && (set("own", "oppo")[t[0].Value] && t[1].Value == "." && set("life", "pp", "maxpp", "ep", "sep", "combo", "shadows")[t[2].Value] || fusion && t[0].Value == "fused" && t[1].Value == "." && set("cost", "distinct")[t[2].Value]) && op(t[3].Value) && isUnsigned(t[4])
}

func evolutionCondition(t []syntax.Token) bool {
	return len(t) == 3 && (t[0].Value == "self" && t[1].Value == "form" && set("unevolved", "evolved", "super_evolved")[t[2].Value] ||
		set("own", "oppo")[t[0].Value] && t[1].Value == "." && set("evolve_unlocked", "superevolve_unlocked")[t[2].Value])
}
func values(t []syntax.Token) string {
	v := make([]string, len(t))
	for i := range t {
		v[i] = t[i].Value
	}
	return words(v)
}
func uintValue(t syntax.Token, bits int) (uint64, bool) {
	if t.Kind != syntax.Integer || len(t.Value) > 1 && t.Value[0] == '0' {
		return 0, false
	}
	n, e := strconv.ParseUint(t.Value, 10, bits)
	return n, e == nil
}
func checkU16(t syntax.Token, allowZero bool, ds *[]syntax.Diagnostic) {
	n, ok := uintValue(t, 16)
	if !ok || !allowZero && n == 0 {
		rangeError(ds, t)
	}
}

func strictValidateTest(tf *TestFile, ds *[]syntax.Diagnostic) {
	for _, sc := range tf.Scenarios {
		strictScenario(sc, ds)
	}
}

func strictScenario(sc *syntax.Statement, ds *[]syntax.Diagnostic) {
	body := sc.Blocks()[0]
	if len(body) != 4 {
		return
	}
	aliases := map[string]string{}
	seed := body[0].Tokens()
	if len(seed) != 2 || !body[0].Terminated {
		shapeError(ds, body[0], "seed u64;")
	} else if _, ok := uintValue(seed[1], 64); !ok {
		rangeError(ds, seed[1])
	}
	strictState(body[1], aliases, ds)
	strictAction(body[2], aliases, ds)
	strictExpect(body[3], aliases, ds)
}

func strictState(s *syntax.Statement, a map[string]string, ds *[]syntax.Diagnostic) {
	if !singleBlock(s, "state") {
		shapeError(ds, s, "state { ... }")
		return
	}
	for _, x := range s.Blocks()[0] {
		t := x.Tokens()
		switch x.Word(0) {
		case "turn":
			if len(t) != 3 || !set("own", "oppo")[t[1].Value] || !x.Terminated {
				shapeError(ds, x, "turn own|oppo 整数;")
			} else if _, ok := uintValue(t[2], 32); !ok {
				rangeError(ds, t[2])
			}
		case "phase":
			if len(t) != 2 || t[1].Value != "main" || !x.Terminated {
				shapeError(ds, x, "phase main;")
			}
		case "player":
			strictPlayer(x, a, ds)
		default:
			shapeError(ds, x, "turn、phase 或 player 声明")
		}
	}
}
func strictPlayer(s *syntax.Statement, a map[string]string, ds *[]syntax.Diagnostic) {
	t := s.Tokens()
	if len(t) != 2 || t[0].Value != "player" || !set("own", "oppo")[t[1].Value] || len(s.Blocks()) != 1 || s.Terminated {
		shapeError(ds, s, "player own|oppo { ... }")
		return
	}
	for _, x := range s.Blocks()[0] {
		tt := x.Tokens()
		h := x.Word(0)
		switch h {
		case "leader", "pp":
			if !statPair(tt, 1) || len(tt) != 4 || !x.Terminated {
				shapeError(ds, x, h+" A/B;")
			} else {
				checkI16orU16(tt[1], h == "leader", ds)
				checkI16orU16(tt[3], h == "leader", ds)
			}
		case "ep", "sep", "combo", "shadows":
			if len(tt) != 2 || !x.Terminated {
				shapeError(ds, x, h+" 整数;")
			} else {
				checkU16(tt[1], true, ds)
			}
		case "deck":
			if len(tt) != 2 || tt[1].Value != "top" {
				shapeError(ds, x, "deck top { instances }")
			} else {
				strictInstances(x, a, ds)
			}
		case "hand", "field", "graveyard", "banished", "destroyed":
			strictInstances(x, a, ds)
		default:
			shapeError(ds, x, "玩家数值或区域声明")
		}
	}
}
func strictInstances(s *syntax.Statement, a map[string]string, ds *[]syntax.Diagnostic) {
	if len(s.Blocks()) != 1 || s.Terminated {
		shapeError(ds, s, "区域 { instances }")
		return
	}
	for _, x := range s.Blocks()[0] {
		t := x.Tokens()
		if len(t) != 4 || !cardTypes[t[0].Value] || t[1].Kind != syntax.Identifier || t[2].Value != "=" || !isCardID(t[3]) || !(x.Terminated && len(x.Blocks()) == 0 || !x.Terminated && len(x.Blocks()) == 1) {
			shapeError(ds, x, "card_type alias = card_id [overrides]")
			continue
		}
		if _, dup := a[t[1].Value]; dup {
			diag(ds, "WBT-E002-DUPLICATE-ALIAS", "错误", "实例别名重复: "+t[1].Value, t[1].Span)
		}
		a[t[1].Value] = t[0].Value
		if len(x.Blocks()) == 1 {
			seenCounters := map[string]bool{}
			for _, o := range x.Blocks()[0] {
				if o.Word(0) == "counter" {
					if seenCounters[o.Word(1)] {
						shapeError(ds, o, "counter override 不能重复")
					}
					seenCounters[o.Word(1)] = true
				}
				strictOverride(o, ds)
			}
		}
	}
}
func strictOverride(s *syntax.Statement, ds *[]syntax.Diagnostic) {
	t := s.Tokens()
	ok := s.Terminated && len(s.Blocks()) == 0
	switch s.Word(0) {
	case "counter":
		ok = ok && len(t) == 3 && t[1].Kind == syntax.Identifier && ir.ValidCounterName(t[1].Value) && isUnsigned(t[2])
	case "stats":
		ok = ok && len(t) == 4 && statPair(t, 1)
		if ok {
			checkI16orU16(t[1], true, ds)
			checkI16orU16(t[3], true, ds)
		}
	case "evolved", "super_evolved":
		ok = ok && len(t) == 1
	case "ward", "storm", "rush", "bane", "drain", "intimidate", "barrier", "stealth", "aura", "ability_target_guard", "cannot_attack", "cannot_attack_follower", "cannot_attack_leader":
		ok = ok && len(t) == 1
	case "cost", "earthsigil", "countdown":
		ok = ok && len(t) == 2
		if ok {
			checkU16(t[1], true, ds)
		}
	case "engaged":
		ok = ok && len(t) == 2 && set("true", "false")[t[1].Value]
	default:
		ok = false
	}
	if !ok {
		shapeError(ds, s, "合法实例 override;")
	}
}

func strictAction(s *syntax.Statement, a map[string]string, ds *[]syntax.Diagnostic) {
	if !singleBlock(s, "action") {
		shapeError(ds, s, "action { primary responses }")
		return
	}
	body := s.Blocks()[0]
	if len(body) == 0 {
		return
	}
	for i, x := range body {
		t := x.Tokens()
		ok := x.Terminated && len(x.Blocks()) == 0
		if i == 0 {
			switch x.Word(0) {
			case "play", "engage", "evolve", "superevolve", "fuse":
				ok = ok && len(t) == 2 && aliasKnown(t[1], a, ds)
			case "attack":
				ok = ok && len(t) >= 4 && aliasKnown(t[1], a, ds) && t[2].Value == "into"
				if ok {
					_, ok = parseAttackTarget(t, 3, a, ds)
					ok = ok && len(t) == 4 || ok && len(t) == 6
				}
			case "end_turn":
				ok = ok && len(t) == 1
			case "advance":
				ok = ok && len(t) == 3 && set("turn_start", "turn_end")[t[1].Value] && set("own", "oppo")[t[2].Value]
			default:
				ok = false
			}
		} else {
			switch x.Word(0) {
			case "select":
				entities, _, parsed := parseSelectionResponse(t)
				ok = ok && parsed
				for _, entity := range entities {
					known := aliasKnown(entity, a, ds)
					ok = ok && known
				}
			case "mode":
				ok = ok && len(t) == 2
				if ok {
					n, g := uintValue(t[1], 16)
					ok = g && n > 0
				}
			default:
				ok = false
			}
		}
		if !ok {
			diag(ds, "WBT-E005-ACTION-ORDER", "错误", "action/response 形状无效", x.Span)
		}
	}
}

func strictExpect(s *syntax.Statement, a map[string]string, ds *[]syntax.Diagnostic) {
	if !singleBlock(s, "expect") {
		shapeError(ds, s, "expect { assertions }")
		return
	}
	for _, x := range s.Blocks()[0] {
		if !strictAssertion(x, a, ds) {
			diag(ds, "WBT-E007-INVALID-ASSERTION", "错误", "断言形状无效或包含多余 token", x.Span)
		}
	}
}
func strictAssertion(s *syntax.Statement, a map[string]string, ds *[]syntax.Diagnostic) bool {
	t := s.Tokens()
	h := s.Word(0)
	if h == "events" {
		if !(!s.Terminated && len(s.Blocks()) == 1 && (values(t) == "events contains ordered" || values(t) == "events excludes" || values(t) == "events exact")) {
			return false
		}
		for _, f := range s.Blocks()[0] {
			if !strictEventFact(f, a, ds) {
				return false
			}
		}
		return true
	}
	if !s.Terminated || len(s.Blocks()) > 0 {
		return false
	}
	switch h {
	case "legal", "unchanged":
		return len(t) == 1
	case "illegal":
		return len(t) == 2 && t[1].Kind == syntax.Identifier
	}
	if len(t) == 3 && a[t[0].Value] != "" && set("has", "lacks")[t[1].Value] && abilities[t[2].Value] {
		return true
	}
	if h == "all" {
		end, ok := parseTargetSet(t, 1)
		if !ok {
			return false
		}
		end, ok = parseWhere(t, end)
		return ok && end+2 == len(t) && t[end].Value == "have" && abilities[t[end+1].Value]
	}
	if (h == "own" || h == "oppo") && len(t) >= 8 && t[1].Value == "." && set("deck", "hand", "field", "graveyard", "banished", "destroyed")[t[2].Value] && t[3].Value == "count" && t[4].Value == "card" && isCardID(t[5]) && t[6].Value == "==" && isUnsigned(t[7]) && len(t) == 8 {
		return true
	}
	if isOrderAssertion(t, a, ds) {
		return true
	}
	eq := -1
	for i, x := range t {
		if x.Value == "==" {
			eq = i
			break
		}
	}
	if eq < 1 {
		return false
	}
	return assertionRef(t[:eq], a, ds) && assertionValue(t[eq+1:])
}
func assertionRef(t []syntax.Token, a map[string]string, ds *[]syntax.Diagnostic) bool {
	if len(t) == 3 && set("own", "oppo")[t[0].Value] && t[1].Value == "." && set("life", "pp", "maxpp", "ep", "sep", "combo", "shadows")[t[2].Value] {
		return true
	}
	if len(t) == 5 && set("own", "oppo")[t[0].Value] && values(t[1:4]) == ". leader ." && set("life", "maxlife")[t[4].Value] {
		return true
	}
	if len(t) == 3 && a[t[0].Value] != "" && t[1].Value == "." && set("zone", "stats", "cost", "evolved", "super_evolved", "earthsigil", "countdown", "engaged")[t[2].Value] {
		return true
	}
	if len(t) == 5 && a[t[0].Value] != "" && t[1].Value == "." && t[2].Value == "counter" && t[3].Value == "." && t[4].Kind == syntax.Identifier && ir.ValidCounterName(t[4].Value) {
		return true
	}
	return len(t) == 3 && values(t) == "rng . consumed"
}
func assertionValue(t []syntax.Token) bool {
	return len(t) == 1 && (isUnsigned(t[0]) || set("true", "false", "deck", "hand", "field", "graveyard", "banished")[t[0].Value]) || len(t) == 3 && statPair(t, 0)
}
func isOrderAssertion(t []syntax.Token, a map[string]string, ds *[]syntax.Diagnostic) bool {
	start := -1
	for i, x := range t {
		if x.Value == "[" {
			start = i
			break
		}
	}
	if start < 0 {
		return false
	}
	end, ok := aliasList(t, start, a, ds)
	if !ok {
		return false
	}
	prefix := values(t[:start])
	return end == len(t) && (prefix == "own . deck top ==" || prefix == "oppo . deck top ==" || (prefix == "own . hand contains" || prefix == "own . field contains" || prefix == "oppo . hand contains" || prefix == "oppo . field contains") && t[len(t)-1].Value == "]") || end+1 == len(t) && t[end].Value == "ordered"
}

func strictEventFact(s *syntax.Statement, a map[string]string, ds *[]syntax.Diagnostic) bool {
	t := s.Tokens()
	if !s.Terminated || len(s.Blocks()) > 0 || len(t) == 0 {
		return false
	}
	h := t[0].Value
	switch h {
	case "damage", "heal":
		end, ok := factObject(t, 1, a, ds)
		return ok && end+1 == len(t) && isUnsigned(t[end])
	case "draw":
		return len(t) == 3 && set("own", "oppo")[t[1].Value] && isUnsigned(t[2])
	case "summon":
		if len(t) == 2 {
			return aliasKnown(t[1], a, ds)
		}
		return len(t) == 5 && t[1].Value == "card" && isCardID(t[2]) && t[3].Value == "count" && isUnsigned(t[4])
	case "destroy", "banish", "discard":
		end, ok := factObject(t, 1, a, ds)
		return ok && end == len(t)
	case "return":
		end, ok := factObject(t, 1, a, ds)
		return ok && end+2 == len(t) && t[end].Value == "to" && set("hand", "deck")[t[end+1].Value]
	case "evolve", "superevolve", "engage":
		return len(t) == 2 && aliasKnown(t[1], a, ds)
	case "attack":
		return len(t) >= 4 && aliasKnown(t[1], a, ds) && t[2].Value == "into" && attackTargetExact(t[3:], a, ds)
	case "turn_start", "turn_end", "game_end":
		return len(t) == 2 && set("own", "oppo")[t[1].Value]
	case "move":
		return len(t) == 4 && aliasKnown(t[1], a, ds) && t[2].Value == "to" && set("deck", "hand", "field", "graveyard", "banished")[t[3].Value]
	case "gain", "spend":
		return len(t) == 5 && set("own", "oppo")[t[1].Value] && t[2].Value == "." && set("life", "pp", "maxpp", "ep", "sep", "combo", "shadows")[t[3].Value] && isUnsigned(t[4])
	}
	return false
}

func singleBlock(s *syntax.Statement, h string) bool {
	return len(s.Tokens()) == 1 && s.Word(0) == h && len(s.Blocks()) == 1 && !s.Terminated
}
func statPair(t []syntax.Token, i int) bool {
	return i+2 < len(t) && isUnsigned(t[i]) && t[i+1].Value == "/" && isUnsigned(t[i+2])
}
func checkI16orU16(t syntax.Token, signed bool, ds *[]syntax.Diagnostic) {
	bits := 16
	if signed {
		n, ok := uintValue(t, bits)
		if !ok || n > math.MaxInt16 {
			rangeError(ds, t)
		}
		return
	}
	checkU16(t, true, ds)
}
func aliasKnown(t syntax.Token, a map[string]string, ds *[]syntax.Diagnostic) bool {
	if t.Kind != syntax.Identifier {
		return false
	}
	if a[t.Value] == "" {
		diag(ds, "WBT-E006-UNKNOWN-ALIAS", "错误", "未知实例别名: "+t.Value, t.Span)
		return false
	}
	return true
}
func parseAttackTarget(t []syntax.Token, i int, a map[string]string, ds *[]syntax.Diagnostic) (int, bool) {
	if i >= len(t) {
		return i, false
	}
	if i+2 < len(t) && set("own", "oppo")[t[i].Value] && t[i+1].Value == "." && t[i+2].Value == "leader" {
		return i + 3, true
	}
	if aliasKnown(t[i], a, ds) {
		return i + 1, true
	}
	return i, false
}
func attackTargetExact(t []syntax.Token, a map[string]string, ds *[]syntax.Diagnostic) bool {
	end, ok := parseAttackTarget(t, 0, a, ds)
	return ok && end == len(t)
}
func aliasList(t []syntax.Token, i int, a map[string]string, ds *[]syntax.Diagnostic) (int, bool) {
	if i >= len(t) || t[i].Value != "[" {
		return i, false
	}
	i++
	if i < len(t) && t[i].Value == "]" {
		return i + 1, true
	}
	for {
		if i >= len(t) || !aliasKnown(t[i], a, ds) {
			return i, false
		}
		i++
		if i < len(t) && t[i].Value == "]" {
			return i + 1, true
		}
		if i >= len(t) || t[i].Value != "," {
			return i, false
		}
		i++
	}
}
func factObject(t []syntax.Token, i int, a map[string]string, ds *[]syntax.Diagnostic) (int, bool) {
	if i >= len(t) {
		return i, false
	}
	if t[i].Value == "card" && i+1 < len(t) && isCardID(t[i+1]) {
		return i + 2, true
	}
	return parseAttackTarget(t, i, a, ds)
}
