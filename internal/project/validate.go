package project

import (
	"path/filepath"
	"strconv"
	"strings"

	"wbo/internal/ir"
	"wbo/internal/syntax"
)

var (
	cardTypes = set("follower", "spell", "amulet")
	abilities = set("ward", "storm", "rush", "bane", "drain", "intimidate", "barrier", "stealth", "aura", "ability_target_guard", "ability_destruction_guard", "damage_taken_up", "cannot_attack", "cannot_attack_follower", "cannot_attack_leader")
	classes   = set("neutral", "forestcraft", "swordcraft", "runecraft", "dragoncraft", "abysscraft", "havencraft", "portalcraft")
	rarities  = set("bronze", "silver", "gold", "legendary")
	locales   = []string{"chs", "eng", "jpn", "kor", "cht"}
)

func set(v ...string) map[string]bool {
	m := map[string]bool{}
	for _, x := range v {
		m[x] = true
	}
	return m
}

func validateCard(f *syntax.File, ds *[]syntax.Diagnostic) *Card {
	if len(f.Statements) != 2 {
		diag(ds, "WBO-E005-INVALID-CARD-SHAPE", "错误", "卡牌文件必须只包含版本声明和一张卡牌", fileSpan(f))
		return nil
	}
	v, ok := version(f.Statements[0], "wbo")
	if !ok {
		shapeError(ds, f.Statements[0], "wbo 主.次.修订;")
		return nil
	}
	if !strings.HasPrefix(v, "0.1.") {
		diag(ds, "WBO-E002-VERSION", "错误", "仅支持 WBO 0.1.x", f.Statements[0].Span)
	}
	d := f.Statements[1]
	tt := d.Tokens()
	blocks := d.Blocks()
	if len(tt) != 2 || tt[0].Value != "card" || tt[1].Kind != syntax.Integer || len(tt[1].Value) != 8 || len(blocks) != 1 || d.Terminated {
		shapeError(ds, d, "card 八位ID { ... }")
		return nil
	}
	c := &Card{Path: f.Path, Version: v, ID: tt[1].Value, Locales: map[string]Locale{}, File: f, Decl: d}
	body := blocks[0]
	stage := 0
	localeIndex := 0
	for _, s := range body {
		t := s.Tokens()
		if len(t) == 0 {
			shapeError(ds, s, "非空声明")
			continue
		}
		switch t[0].Value {
		case "type":
			if stage != 0 || len(t) != 2 || !cardTypes[t[1].Value] || !s.Terminated {
				shapeError(ds, s, "type follower|spell|amulet;")
			} else {
				c.Type = t[1].Value
			}
			stage = 1
		case "cost":
			if stage != 1 || len(t) != 2 || !s.Terminated {
				shapeError(ds, s, "cost 非负整数;")
			} else if n, ok := integer(t[1]); !ok {
				rangeError(ds, t[1])
			} else {
				c.Cost = n
			}
			stage = 2
		case "stats":
			if stage != 2 || len(t) != 4 || t[2].Value != "/" || !s.Terminated {
				shapeError(ds, s, "stats 攻击/生命;")
			} else {
				a, ao := integer(t[1])
				b, bo := integer(t[3])
				if !ao {
					rangeError(ds, t[1])
				}
				if !bo {
					rangeError(ds, t[3])
				}
				if ao && bo {
					c.Stats = &[2]int{a, b}
				}
			}
			stage = 3
		case "trait":
			if stage < 2 || stage > 3 || len(t) != 2 || !s.Terminated {
				shapeError(ds, s, "trait 标识符;")
			} else {
				if !ir.ValidTrait(t[1].Value) {
					unknown(ds, t[1], "种族")
				}
				c.Traits = append(c.Traits, t[1].Value)
			}
			stage = 3
		case "effect":
			if stage < 2 || stage > 3 || len(t) != 1 || len(s.Blocks()) != 1 || s.Terminated {
				shapeError(ds, s, "effect { ... }")
			} else {
				c.Effect = s.Blocks()[0]
				validateEffectBlock(c.Effect, ds, map[string]bool{"self": true}, "")
			}
			stage = 4
		case "crest":
			if stage != 4 || c.Crest != nil || len(t) != 1 || len(s.Blocks()) != 1 || s.Terminated {
				shapeError(ds, s, "crest { 能力与五种本地化 }")
			} else {
				c.Crest = validateCrest(c, s, ds)
			}
		case "meta":
			if stage != 4 || len(t) != 1 || len(s.Blocks()) != 1 {
				shapeError(ds, s, "meta { pack ...; class ...; rarity ...; }")
			} else {
				parseMeta(c, s.Blocks()[0], ds)
			}
			stage = 5
		case "locale":
			if stage < 5 || len(t) != 2 || len(s.Blocks()) != 1 {
				shapeError(ds, s, "locale 语言 { name ...; text ...; }")
				continue
			}
			if localeIndex >= len(locales) || t[1].Value != locales[localeIndex] {
				diag(ds, "WBO-E006-DUPLICATE-LOCALE", "错误", "本地化必须按 chs、eng、jpn、kor、cht 顺序且各出现一次", s.Span)
			} else {
				localeIndex++
			}
			parseLocale(c, t[1].Value, s.Blocks()[0], ds)
			stage = 5
		default:
			unknown(ds, t[0], "顶层字段")
		}
	}
	if c.Type == "follower" && c.Stats == nil {
		diag(ds, "WBO-E005-INVALID-CARD-SHAPE", "错误", "随从必须声明 stats", d.Span)
	}
	if c.Type != "follower" && c.Stats != nil {
		diag(ds, "WBO-E005-INVALID-CARD-SHAPE", "错误", "法术和护符不得声明 stats", d.Span)
	}
	if stage < 5 || localeIndex != 5 {
		diag(ds, "WBO-E005-INVALID-CARD-SHAPE", "错误", "卡牌缺少 effect、meta 或五种本地化", d.Span)
	}
	strictValidateCard(c, ds)
	return c
}

func parseMeta(c *Card, b []*syntax.Statement, ds *[]syntax.Diagnostic) {
	if len(b) != 3 || b[0].Word(0) != "pack" || b[1].Word(0) != "class" || b[2].Word(0) != "rarity" {
		diag(ds, "WBO-E005-INVALID-CARD-SHAPE", "错误", "meta 字段必须依次为 pack、class、rarity", c.Decl.Span)
		return
	}
	for _, s := range b {
		if len(s.Tokens()) != 2 || !s.Terminated {
			shapeError(ds, s, s.Word(0)+" 值;")
			continue
		}
		t := s.Tokens()
		switch t[0].Value {
		case "pack":
			n64, e := strconv.ParseUint(t[1].Value, 10, 32)
			n, ok := int(n64), e == nil && t[1].Kind == syntax.Integer && !(len(t[1].Value) > 1 && t[1].Value[0] == '0')
			if !ok {
				rangeError(ds, t[1])
			} else {
				c.Meta.Pack = n
			}
		case "class":
			c.Meta.Class = t[1].Value
			if !classes[t[1].Value] {
				unknown(ds, t[1], "职业")
			}
		case "rarity":
			c.Meta.Rarity = t[1].Value
			if !rarities[t[1].Value] {
				unknown(ds, t[1], "稀有度")
			}
		}
	}
}

func parseLocale(c *Card, code string, b []*syntax.Statement, ds *[]syntax.Diagnostic) {
	if len(b) != 2 {
		diag(ds, "WBO-E005-INVALID-CARD-SHAPE", "错误", "locale 必须依次包含 name 和 text", c.Decl.Span)
		return
	}
	n, nok := parseText(b[0], "name")
	t, tok := parseText(b[1], "text")
	if !nok {
		shapeError(ds, b[0], "name 字符串;")
	}
	if !tok {
		shapeError(ds, b[1], "text 字符串;")
	}
	if nok && tok {
		if _, dup := c.Locales[code]; dup {
			diag(ds, "WBO-E006-DUPLICATE-LOCALE", "错误", "本地化重复: "+code, b[0].Span)
		}
		c.Locales[code] = Locale{Name: n, Text: t}
	}
}

func validateEffectBlock(body []*syntax.Statement, ds *[]syntax.Diagnostic, inherited map[string]bool, event string) {
	bindings := map[string]bool{}
	for k, v := range inherited {
		bindings[k] = v
	}
	// 一次打出的入场曲与爆能强化共享同一个打出帧：后声明的块可以读取先声明块的输出
	// （例如"爆能强化_6：使其获得【毁灭】"里的"其"指向入场曲召唤的随从）。
	playOutputs := map[string]bool{}
	if event == "summoned" {
		bindings["summoned"] = true
	}
	if event == "engaged" {
		bindings["engaged"] = true
	}
	if event == "played" {
		bindings["played"] = true
	}
	if event == "discarded" {
		bindings["discarded"] = true
	}
	if event == "destroyed" {
		bindings["destroyed"] = true
	}
	if event == "healed" {
		bindings["healed"] = true
	}
	if event == "survives" {
		bindings["damaged"] = true
	}
	if event == "leaves" {
		bindings["left"] = true
	}
	if event == "evolved" || event == "super_evolved" {
		bindings["evolved"] = true
	}
	if event == "attack" || event == "clash" {
		bindings["opponent"] = true
	}
	var evolveOutputs map[string]bool
	for _, candidate := range body {
		if candidate.Word(0) == "evolve" && len(candidate.Blocks()) == 1 {
			evolveOutputs = producedBindings(candidate.Blocks()[0])
			break
		}
	}
	for _, s := range body {
		t := s.Tokens()
		b := s.Blocks()
		if len(t) == 0 {
			shapeError(ds, s, "效果语句")
			continue
		}
		h := t[0].Value
		for n := 0; n+2 < len(t); n++ {
			if (t[n].Value == "count" || t[n].Value == "sum") && t[n+1].Value == "(" {
				checkBindingAt(t, n+2, n+3, bindings, ds)
			}
		}
		if abilities[h] || h == "unplayable" || h == "earthsigil" {
			if len(t) != 1 || !s.Terminated || len(b) > 0 {
				shapeError(ds, s, h+";")
			}
			continue
		}
		if h == "countdown" {
			if len(t) != 2 || !s.Terminated {
				shapeError(ds, s, "countdown 整数;")
			} else if _, ok := integer(t[1]); !ok {
				rangeError(ds, t[1])
			}
			continue
		}
		if h == "damage_reduction" || h == "damage_cap" {
			if len(t) != 2 || !s.Terminated {
				shapeError(ds, s, h+" 非负整数;")
			} else if _, ok := integer(t[1]); !ok {
				rangeError(ds, t[1])
			}
			continue
		}
		if h == "attack_limit" {
			if len(t) != 2 || !s.Terminated {
				shapeError(ds, s, "attack_limit 正整数;")
			} else if n, ok := integer(t[1]); !ok || n < 1 {
				rangeError(ds, t[1])
			}
			continue
		}
		switch h {
		case "counter":
			continue
		case "repeat":
			end, ok := parseEffectAmount(t, 1)
			if !ok || end != len(t) || len(b) != 1 || s.Terminated {
				shapeError(ds, s, "repeat 数值 { ... }")
			} else {
				validateEffectBlock(b[0], ds, bindings, "")
			}
			continue
		case "fanfare", "lastwords", "attack", "clash", "evolve", "spellboost":
			if len(t) == 1 && len(b) == 1 && !s.Terminated {
				validateEffectBlock(b[0], ds, bindings, h)
				if h == "fanfare" {
					for k, v := range producedBindings(b[0]) {
						playOutputs[k] = v
					}
				}
				continue
			}
			if h != "evolve" && h != "spellboost" {
				shapeError(ds, s, h+" { ... }")
				continue
			}
		case "superevolve":
			if len(b) == 0 {
				break
			}
			if !(len(t) == 1 || len(t) == 3 && (t[1].Value == "replaces" || t[1].Value == "extends") && t[2].Value == "evolve") || len(b) != 1 {
				shapeError(ds, s, "superevolve [replaces|extends evolve] { ... }")
			} else {
				visible := bindings
				if len(t) == 3 && t[1].Value == "extends" {
					visible = map[string]bool{}
					for k, v := range bindings {
						visible[k] = v
					}
					for k, v := range evolveOutputs {
						visible[k] = v
					}
				}
				validateEffectBlock(b[0], ds, visible, "")
			}
			continue
		case "engage", "enhance", "earthrite", "necromancy":
			// enhance 额外允许 `replaces`：支付该档时改为只执行这个块。
			replaces := h == "enhance" && len(t) == 3 && t[2].Value == "replaces"
			if !(len(t) == 2 || replaces) || len(b) != 1 {
				shapeError(ds, s, h+" 整数 [replaces] { ... }")
			} else {
				if _, ok := integer(t[1]); !ok {
					rangeError(ds, t[1])
				}
				visible := bindings
				if h == "enhance" && len(playOutputs) > 0 {
					visible = map[string]bool{}
					for k, v := range bindings {
						visible[k] = v
					}
					for k, v := range playOutputs {
						visible[k] = v
					}
				}
				validateEffectBlock(b[0], ds, visible, "")
			}
			continue
		case "fusion":
			if len(t) < 6 || words(tokenValuesPrefix(t, 4)) != "fusion material from own" || t[4].Value != "." || t[5].Value != "hand" || len(b) != 1 {
				shapeError(ds, s, "fusion material from own.hand [where ...] { ... }")
			} else {
				validateFilterTail(t, 6, ds)
				validateEffectBlock(b[0], ds, bindings, "")
			}
			continue
		case "grant":
			end, good := parseValueRef(t, 1)
			checkBindingAt(t, 1, end, bindings, ds)
			if good && end < len(t) {
				end, good = parseWhere(t, end)
			}
			_, ability, err := grantParts(s)
			if !good || end != len(t) || err != nil || s.Terminated {
				shapeError(ds, s, "grant 对象 [where ...] { lastwords 或回合触发能力 }")
			} else {
				validateEffectBlock([]*syntax.Statement{ability}, ds, nil, "")
			}
			continue
		case "when":
			if len(t) < 3 || len(b) != 1 {
				shapeError(ds, s, "when 事件 [where ...] { ... }")
			} else {
				ev := ""
				if _, _, ok := parseBaseEventPattern(t); ok {
					if t[1].Value == "self" {
						ev = t[2].Value
						if ev == "survives" {
							ev = ""
						}
					} else {
						ev = t[3].Value
					}
				}
				validateEffectBlock(b[0], ds, bindings, ev)
			}
			continue
		case "replace":
			if len(b) == 0 {
				if _, ok := deckReplaceIR(t, ir.NodeBase{}); !ok || !s.Terminated {
					shapeError(ds, s, "replace own.deck|oppo.deck with shuffled 数量 card ID [, 数量 card ID ...];")
				}
				continue
			}
			if words(tokenValuesPrefix(t, len(t))) != "replace self leaving field" || len(b) != 1 {
				shapeError(ds, s, "replace self leaving field { ... }")
			} else {
				validateEffectBlock(b[0], ds, bindings, "")
			}
			continue
		case "choose", "require", "random":
			end, setOK := parseTargetSet(t, 3)
			if setOK && len(t) > 5 && t[5].Value == "destroyed" {
				setOK = false
			}
			if setOK && end < len(t) && t[end].Value == "or" {
				if end == 8 && len(t) >= 12 && values(t[4:8]) == ". field . followers" && values(t[end:end+4]) == "or "+t[3].Value+" . leader" {
					end += 4
				} else {
					setOK = false
				}
				if end < len(t) && t[end].Value != "count" {
					setOK = false
				}
			}
			if setOK && end < len(t) && t[end].Value == "other" {
				end = otherExclusionEnd(t, end)
			}
			if setOK && end < len(t) && t[end].Value == "where" {
				end, setOK = parseWhere(t, end)
			}
			if setOK && end < len(t) && set("highest", "lowest")[t[end].Value] {
				_, end, setOK = parseExtremum(t, end)
			}
			if setOK && end < len(t) && t[end].Value == "count" {
				if end+1 < len(t) {
					if count, ok := integer(t[end+1]); ok {
						setOK = count > 0
						end += 2
					} else {
						// 动态数量：`random target … count own.crests`。
						end, setOK = parseEffectAmount(t, end+1)
					}
				} else {
					setOK = false
				}
			}
			if len(t) < 4 || t[1].Kind != syntax.Identifier || t[2].Value != "from" || !s.Terminated || len(b) > 0 || !setOK || end != len(t) {
				shapeError(ds, s, h+" 绑定 from 集合 [other] [where ...] [highest|lowest [base.]attack|life|cost] [count 正整数];")
			} else {
				bindings[t[1].Value] = true
			}
			continue
		case "if":
			if len(b) < 1 || len(b) > 2 || len(t) < 2 {
				shapeError(ds, s, "if 条件 { ... } [else { ... }]")
			} else {
				condition := t[1:]
				for i, x := range condition {
					if x.Value == "else" {
						condition = condition[:i]
						break
					}
				}
				validateCondition(condition, ds)
				for _, bb := range b {
					validateEffectBlock(bb, ds, bindings, "")
				}
			}
			continue
		case "mode":
			// `mode { ... }` 选 1 个；`mode N { ... }` 选 N 个（【模式】选择 N 个能力发动）。
			if len(t) < 1 || len(t) > 2 || len(b) != 1 || len(b[0]) < 2 || len(t) == 2 && !isUnsigned(t[1]) {
				shapeError(ds, s, "mode [数量] { 至少两个 option }")
			} else {
				seen := map[string]bool{}
				for _, o := range b[0] {
					ot := o.Tokens()
					if len(ot) != 2 || ot[0].Value != "option" || len(o.Blocks()) != 1 {
						shapeError(ds, o, "option 整数 { ... }")
						continue
					}
					if seen[ot[1].Value] {
						diag(ds, "WBO-E010-DUPLICATE-OPTION", "错误", "mode 选项编号重复", o.Span)
					}
					seen[ot[1].Value] = true
					_, body, err := modeOptionParts(o.Blocks()[0])
					if err != nil {
						shapeError(ds, o, err.Error())
					} else {
						validateEffectBlock(body, ds, bindings, "")
					}
				}
			}
			continue
		}
		if !validateOperation(s, ds, bindings) {
			unknown(ds, t[0], "效果语句")
		}
		if h == "draw" {
			bindings["drawn"] = true
		}
		if h == "add" {
			bindings["added"] = true
		}
		if h == "destroy" {
			bindings["destroyed"] = true
		}
		if h == "summon" || h == "reanimate" {
			bindings["summoned"] = true
		}
	}
}

func producedBindings(body []*syntax.Statement) map[string]bool {
	out := map[string]bool{}
	for _, s := range body {
	switch s.Word(0) {
	case "draw":
		out["drawn"] = true
	case "add":
		out["added"] = true
	case "destroy":
		out["destroyed"] = true
		case "summon", "reanimate":
			out["summoned"] = true
		case "choose", "require", "random":
			if len(s.Tokens()) > 1 {
				out[s.Word(1)] = true
			}
		}
	}
	return out
}

func validateOperation(s *syntax.Statement, ds *[]syntax.Diagnostic, bindings map[string]bool) bool {
	t := s.Tokens()
	if len(t) == 0 {
		return false
	}
	h := t[0].Value
	known := set("draw", "add", "summon", "damage", "heal", "buff", "gain", "restore", "destroy", "banish", "discard", "remove", "return", "evolve", "superevolve", "reanimate", "reduce", "raise", "halve", "double", "spellboost", "transform", "set_attack_limit", "set_damage_reduction", "set")
	if !known[h] {
		return false
	}
	if !s.Terminated || len(s.Blocks()) > 0 {
		shapeError(ds, s, h+" ...;")
		return true
	}
	if len(t) < 2 {
		shapeError(ds, s, h+" 参数;")
		return true
	}
	ok := false
	switch h {
	case "set":
		if t[1].Value == "maxlife" {
			_, ok = leaderMaxLifeIR(t, ir.NodeBase{})
			break
		}
		end, good := parseValueRef(t, 2)
		checkBindingAt(t, 2, end, bindings, ds)
		if good && (end == 5 && t[4].Value == "leader" || end == 3 && t[2].Value == "leaders" || end == 5 && values(t[2:5]) == "all . leaders") {
			good = false
		}
		if good {
			end, good = parseEffectAmount(t, end)
		}
		if good && end < len(t) && t[end].Value == "until" {
			// `set cost T N until own turn ends`：临时费用修改。
			end, good = parseEffectDuration(t, end)
		}
		ok = set("life", "cost", "attack")[t[1].Value] && good && end == len(t)
	case "set_attack_limit":
		end, good := parseValueRef(t, 1)
		checkBindingAt(t, 1, end, bindings, ds)
		ok = good && end+1 == len(t) && isU16(t[end])
		if ok {
			n, _ := integer(t[end])
			ok = n >= 1
		}
	case "set_damage_reduction":
		end, good := parseValueRef(t, 1)
		checkBindingAt(t, 1, end, bindings, ds)
		if good && (end == 4 && t[3].Value == "leader" || end == 4 && values(t[1:4]) == "all . leaders" || end == 2 && t[1].Value == "leaders") {
			good = false
		}
		ok = good && end+1 == len(t) && isU16(t[end])
	case "draw":
		// draw <amount> [for own|oppo] [from deck <where>]
		offset := 2
		ok = len(t) >= 2 && (isUnsigned(t[1]) || t[1].Value == "all")
		if ok && len(t) >= 4 && t[2].Value == "for" && set("own", "oppo")[t[3].Value] {
			offset = 4
		}
		if ok && len(t) == offset {
			// draw all 必须带过滤器。
			ok = t[1].Value != "all"
		}
		if ok && len(t) > offset {
			if len(t) >= offset+2 && t[offset].Value == "from" && t[offset+1].Value == "deck" {
				end, good := parseWhere(t, offset+2)
				ok = good && end == len(t)
			} else {
				ok = false
			}
		}
	case "add":
		if len(t) == 4 && isUnsigned(t[1]) && t[2].Value == "counter" && t[3].Kind == syntax.Identifier && ir.ValidCounterName(t[3].Value) {
			ok = true
		}
		if len(t) >= 6 && t[1].Value == "copies" && t[2].Value == "of" {
			// add copies of 集合 to hand|deck：把集合里每个对象的同名卡加入目标区域。
			end, good := parseValueRef(t, 3)
			checkBindingAt(t, 3, end, bindings, ds)
			if good && end+2 == len(t) && t[end].Value == "to" && set("hand", "deck")[t[end+1].Value] {
				ok = true
			}
		}
		if len(t) == 6 && isUnsigned(t[1]) && t[2].Value == "card" && isCardID(t[3]) && t[4].Value == "to" && t[5].Value == "hand" {
			ok = true
		}
		if len(t) == 3 && t[1].Value == "combo" && isUnsigned(t[2]) {
			ok = true
		}
		if len(t) == 3 && isUnsigned(t[1]) && t[2].Value == "earthsigil" {
			ok = true
		}
		if len(t) >= 4 && abilities[t[1].Value] && t[2].Value == "to" {
			end, good := parseValueRef(t, 3)
			if good && end < len(t) && t[end].Value == "other" {
				end = otherExclusionEnd(t, end)
			}
			if good && end < len(t) && t[end].Value == "where" {
				end, good = parseWhere(t, end)
			}
			if good && end < len(t) {
				end, good = parseEffectDuration(t, end)
			}
			ok = good && end == len(t)
			checkBindingAt(t, 3, end, bindings, ds)
		}
	case "summon":
		ok = len(t) == 4 && isUnsigned(t[1]) && t[2].Value == "card" && isCardID(t[3])
		if len(t) == 2 && t[1].Kind == syntax.Identifier {
			// `summon target;`：把已经存在于手牌的对象直接放到战场（不发动入场曲）。
			checkBindingAt(t, 1, 2, bindings, ds)
			ok = true
		}
		if len(t) == 6 && isUnsigned(t[1]) && t[2].Value == "card" && isCardID(t[3]) && t[4].Value == "for" && set("own", "oppo")[t[5].Value] {
			// summon N card X for own|oppo：在指定一方的战场上召唤。
			ok = true
		}
		if len(t) > 1 && t[1].Value == "random" {
			_, ok = historySummonIR(t, ir.NodeBase{})
			if !ok {
				_, ok = deckSummonIR(t, ir.NodeBase{})
			}
		}
		if len(t) >= 4 && t[1].Value == "copies" && t[2].Value == "of" {
			end, good := parseValueRef(t, 3)
			checkBindingAt(t, 3, end, bindings, ds)
			if good && end < len(t) && t[end].Value == "where" {
				end, good = parseWhere(t, end)
			}
			ok = good && end == len(t)
		}
	case "damage", "heal":
		end, good := parseValueRef(t, 1)
		if good {
			checkBindingAt(t, 1, end, bindings, ds)
			if end < len(t) && t[end].Value == "other" {
				end = otherExclusionEnd(t, end)
			}
			targetEnd := end
			end, good = parseEffectAmount(t, end)
			if good && end < len(t) && t[end].Value == "distributed" {
				good = h == "damage" && targetEnd == 6 && set("own", "oppo")[t[1].Value] && t[3].Value == "field" && t[5].Value == "followers"
				end++
				if good && end < len(t) && t[end].Value == "overflow" {
					good = end+3 < len(t) && t[end+1].Value == t[1].Value && t[end+2].Value == "." && t[end+3].Value == "leader"
					end += 4
				}
			}
			if good {
				extremum, next, extremumOK := parseExtremum(t, end)
				good = extremumOK
				if extremum != nil {
					end = next
				}
			}
			if good && end < len(t) {
				end, good = parseWhere(t, end)
			}
			ok = good && end == len(t)
		}
	case "buff":
		end, good := parseValueRef(t, 1)
		if good && end < len(t) && t[end].Value == "other" {
			end = otherExclusionEnd(t, end)
		}
		if good {
			end, good = parseSignedAmount(t, end)
			if good && end < len(t) && t[end].Value == "/" {
				end, good = parseSignedAmount(t, end+1)
			} else {
				good = false
			}
			if good && end < len(t) && t[end].Value == "where" {
				end, good = parseWhere(t, end)
			}
			if good && end < len(t) {
				end, good = parseEffectDuration(t, end)
			}
			ok = good && end == len(t)
		}
		checkBindingAt(t, 1, end, bindings, ds)
	case "gain":
		ok = len(t) == 5 && (t[1].Value == "own" || t[1].Value == "oppo") && t[2].Value == "." && set("life", "pp", "maxpp", "ep", "sep", "combo", "shadows", "rally")[t[3].Value] && isUnsigned(t[4])
		ok = ok || len(t) == 4 && set("own", "oppo")[t[1].Value] && t[2].Value == "crest" && isCardID(t[3])
		if len(t) >= 4 && t[1].Value == "skybound" {
			// `gain skybound 集合 N`：奥义槽 +N。
			end, good := parseValueRef(t, 2)
			checkBindingAt(t, 2, end, bindings, ds)
			ok = good && end+1 == len(t) && isUnsigned(t[end])
		}
	case "restore":
		ok = len(t) == 4 && set("own", "oppo")[t[1].Value] && t[2].Value == "." && t[3].Value == "pp"
	case "destroy", "banish", "discard":
		// `banish duplicates in own|oppo.deck`：牌组去重，只保留每种卡牌的第一张。
		if h == "banish" && len(t) == 6 && t[1].Value == "duplicates" && t[2].Value == "in" && set("own", "oppo")[t[3].Value] && t[4].Value == "." && t[5].Value == "deck" {
			ok = true
			break
		}
		if targets, batch := destructionBatchTargets(t); batch {
			for _, target := range targets {
				if target.Value != "self" && (!bindings[target.Value] || set("own", "oppo", "field")[target.Value]) {
					diag(ds, "WBO-E009-BINDING-SCOPE", "错误", "批次破坏目标必须是已定义绑定或 self", target.Span)
				}
			}
			ok = true
			break
		}
		end, good := parseValueRef(t, 1)
		if h == "discard" && good && (end == 4 && t[3].Value == "leader" || end == 2 && t[1].Value == "leaders") {
			good = false
		}
		if good && end < len(t) && t[end].Value == "other" {
			// destroy 集合 other：排除来源实例自身。
			end = otherExclusionEnd(t, end)
		}
		if good && end < len(t) {
			end, good = parseWhere(t, end)
		}
		ok = good && end == len(t)
		checkBindingAt(t, 1, end, bindings, ds)
	case "remove":
		// remove <固有关键词> from 集合
		// remove lastwords from 集合
		// remove all abilities from 集合
		start := 0
		switch {
		case len(t) >= 4 && abilities[t[1].Value] && t[2].Value == "from":
			start = 3
		case len(t) >= 3 && t[1].Value == "lastwords" && t[2].Value == "from":
			start = 3
		case len(t) >= 4 && values(t[1:3]) == "all abilities" && t[3].Value == "from":
			start = 4
		}
		if start > 0 {
			end, good := parseValueRef(t, start)
			if good && end < len(t) && t[end].Value == "other" {
				end = otherExclusionEnd(t, end)
			}
			if good && end < len(t) {
				end, good = parseWhere(t, end)
			}
			ok = good && end == len(t)
			checkBindingAt(t, start, end, bindings, ds)
		}
	case "return":
		end, good := parseValueRef(t, 1)
		ok = good && end+2 == len(t) && t[end].Value == "to" && (t[end+1].Value == "hand" || t[end+1].Value == "deck")
		checkBindingAt(t, 1, end, bindings, ds)
	case "evolve", "superevolve":
		end, good := parseValueRef(t, 1)
		ok = good && end+1 == len(t) && t[end].Value == "silent"
		checkBindingAt(t, 1, end, bindings, ds)
	case "reanimate":
		ok = len(t) == 2 && isUnsigned(t[1])
	case "reduce":
		if len(t) >= 4 && (t[1].Value == "countdown" || t[1].Value == "cost") {
			end, good := parseValueRef(t, 2)
			if good {
				// 增量可以是常量或数值引用：`reduce countdown self own.crests`。
				if next, amountOK := parseEffectAmount(t, end); amountOK {
					end = next
					if t[1].Value == "countdown" {
						ok = end == len(t)
					} else if end < len(t) && t[end].Value == "until" {
						// `reduce cost T N until ...`：临时降费，到期按差量还原。
						end, ok = parseEffectDuration(t, end)
						ok = ok && end == len(t)
					} else {
						ok = end+2 == len(t) && t[end].Value == "minimum" && isUnsigned(t[end+1])
					}
				}
			}
			checkBindingAt(t, 2, end, bindings, ds)
		}
	case "halve":
		if len(t) >= 3 && t[1].Value == "cost" {
			end, good := parseValueRef(t, 2)
			ok = good && end == len(t)
			checkBindingAt(t, 2, end, bindings, ds)
		}
	case "raise":
		if len(t) >= 4 && (t[1].Value == "cost" || t[1].Value == "countdown") {
			end, good := parseValueRef(t, 2)
			if good && end < len(t) && isUnsigned(t[end]) {
				end++
			} else {
				good = false
			}
			if good && end < len(t) && t[end].Value == "until" {
				// `raise cost T N until ...`：临时加费，到期按差量还原。
				end, good = parseEffectDuration(t, end)
			}
			ok = good && end == len(t)
			checkBindingAt(t, 2, end, bindings, ds)
		}
	case "double":
		if len(t) >= 3 && t[1].Value == "stats" {
			end, good := parseValueRef(t, 2)
			ok = good && end == len(t)
			checkBindingAt(t, 2, end, bindings, ds)
		}
	case "spellboost":
		end, good := parseValueRef(t, 1)
		ok = good && end+1 == len(t) && isUnsigned(t[end])
		checkBindingAt(t, 1, end, bindings, ds)
	case "transform":
		end, good := parseValueRef(t, 1)
		checkBindingAt(t, 1, end, bindings, ds)
		good = good && ir.ValidTransformTarget(valueRefIR(t, 1)) && end+3 <= len(t) && t[end].Value == "into" && t[end+1].Value == "card" && isCardID(t[end+2])
		if good {
			end += 3
			if end+1 < len(t) && t[end].Value == "preserving" && t[end+1].Value == "materials" {
				end += 2
			}
			if end < len(t) {
				end, good = parseWhere(t, end)
			}
		}
		ok = good && end == len(t)
	}
	if !ok {
		shapeError(ds, s, h+" 的规范参数;")
	}
	return true
}

func parseEffectAmount(t []syntax.Token, i int) (int, bool) {
	if counterRef(t, i) {
		return i + 5, true
	}
	if i < len(t) && isUnsigned(t[i]) {
		return i + 1, true
	}
	if i+2 < len(t) && t[i+1].Value == "." {
		if t[i].Value == "self" && set("attack", "life", "cost", "damage_taken")[t[i+2].Value] ||
			set("own", "oppo")[t[i].Value] && ir.ValidPlayerScalar(t[i+2].Value) ||
			t[i].Value == "fused" && set("cost", "distinct")[t[i+2].Value] ||
			t[i].Kind == syntax.Identifier && set("attack", "life", "cost")[t[i+2].Value] {
			// 最后一条是 `<绑定>.attack|life|cost`（绑定名不在这里校验存在性）。
			return i + 3, true
		}
	}
	if i+1 >= len(t) || !set("count", "sum")[t[i].Value] || t[i+1].Value != "(" {
		return i, false
	}
	end, ok := parseCountSource(t, i+2)
	if ok && end < len(t) && t[end].Value == "other" {
		// count(集合 other)：统计时排除来源实例自身。
		end = otherExclusionEnd(t, end)
	}
	if ok && end < len(t) && t[end].Value == "where" {
		end, ok = parseWhere(t, end)
	}
	if ok && t[i].Value == "sum" {
		if end+1 < len(t) && t[end].Value == "," && set("attack", "life", "cost")[t[end+1].Value] {
			end += 2
		} else {
			ok = end+3 < len(t) && t[end].Value == "," && t[end+1].Value == "base" && t[end+2].Value == "." && set("attack", "life", "cost")[t[end+3].Value]
			end += 4
		}
	}
	if !ok || end >= len(t) || t[end].Value != ")" {
		return end, false
	}
	return end + 1, true
}

func parseSignedAmount(t []syntax.Token, i int) (int, bool) {
	if i >= len(t) || !isSign(t[i]) {
		return i, false
	}
	return parseEffectAmount(t, i+1)
}

func parseCountSource(t []syntax.Token, i int) (int, bool) {
	if i < len(t) && t[i].Kind == syntax.Identifier && !set("self", "own", "oppo", "field", "all", "leaders")[t[i].Value] {
		return i + 1, true
	}
	return parseTargetSet(t, i)
}

func parseTargetSet(t []syntax.Token, i int) (int, bool) {
	start := i
	if i >= len(t) {
		return i, false
	}
	if t[i].Value == "field" {
		i++
		if i+1 < len(t) && t[i].Value == "." && set("followers", "spells", "amulets")[t[i+1].Value] {
			i += 2
		}
		return i, true
	}
	if t[i].Value != "own" && t[i].Value != "oppo" {
		return i, false
	}
	if i+2 >= len(t) || t[i+1].Value != "." || !set("deck", "hand", "field", "graveyard", "banished", "destroyed", "crests")[t[i+2].Value] {
		return i, false
	}
	i += 3
	if i+1 < len(t) && t[i].Value == "." && set("followers", "spells", "amulets")[t[i+1].Value] {
		i += 2
	}
	if i+1 < len(t) && t[i].Value == "this" && t[i+1].Value == "turn" {
		if t[start+2].Value != "destroyed" || t[i-1].Value == "spells" {
			return i, false
		}
		i += 2
	}
	return i, true
}
func parseValueRef(t []syntax.Token, i int) (int, bool) {
	if i >= len(t) {
		return i, false
	}
	if t[i].Value == "all" && i+2 < len(t) && t[i+1].Value == "." && t[i+2].Value == "leaders" {
		return i + 3, true
	}
	if set("self", "target", "summoned", "drawn")[t[i].Value] || t[i].Kind == syntax.Identifier && !set("own", "oppo", "field")[t[i].Value] {
		return i + 1, true
	}
	if (t[i].Value == "own" || t[i].Value == "oppo") && i+2 < len(t) && t[i+1].Value == "." && t[i+2].Value == "leader" {
		return i + 3, true
	}
	end, ok := parseTargetSet(t, i)
	return end, ok && !(i+2 < len(t) && t[i+2].Value == "destroyed")
}
// otherExclusionEnd 跳过 `other [绑定名]`，与 typed_ir.otherExclusion 保持一致。
func otherExclusionEnd(t []syntax.Token, i int) int {
	next := i + 1
	if next < len(t) && t[next].Kind == syntax.Identifier && !otherFollowers[t[next].Value] {
		return next + 1
	}
	return next
}

func parseWhere(t []syntax.Token, i int) (int, bool) {
	if i >= len(t) || t[i].Value != "where" {
		return i, false
	}
	i++
	term := false
	for i < len(t) {
		start := i
		switch t[i].Value {
		case "spellboost":
			i++
		case "base":
			if i+4 < len(t) && t[i+1].Value == "." && set("attack", "life", "cost")[t[i+2].Value] && set("==", "!=", "<", "<=", ">", ">=")[t[i+3].Value] {
				if isUnsigned(t[i+4]) {
					i += 5
				} else if i+6 < len(t) && set("own", "oppo")[t[i+4].Value] && t[i+5].Value == "." && ir.ValidPlayerScalar(t[i+6].Value) {
					i += 7
				}
			}
		case "damaged":
			i++
		case "attacked":
			if i+2 < len(t) && t[i+1].Value == "this" && t[i+2].Value == "turn" {
				i += 3
			}
		case "not":
			if i+3 < len(t) && t[i+1].Value == "attacked" && t[i+2].Value == "this" && t[i+3].Value == "turn" {
				i += 4
			}
		case "cost":
			if i+1 < len(t) && t[i+1].Value == "changed" {
				i += 2
				break
			}
			if i+2 < len(t) && set("==", "!=", "<", "<=", ">", ">=")[t[i+1].Value] {
				if isUnsigned(t[i+2]) {
					i += 3
				} else if i+4 < len(t) && set("own", "oppo")[t[i+2].Value] && t[i+3].Value == "." && ir.ValidPlayerScalar(t[i+4].Value) {
					i += 5
				}
			}
		case "keyword":
			if i+1 < len(t) && ir.ValidKeyword(t[i+1].Value) {
				i += 2
			}
		case "card":
			if i+1 < len(t) && (isCardID(t[i+1]) || t[i+1].Kind == syntax.Identifier) {
				i += 2
			}
		case "type":
			if i+1 < len(t) && cardTypes[t[i+1].Value] {
				i += 2
			}
		case "class":
			if i+1 < len(t) && classes[t[i+1].Value] {
				i += 2
			}
		case "trait":
			if i+1 < len(t) && ir.ValidTrait(t[i+1].Value) {
				i += 2
			}
		case "form":
			if i+1 < len(t) && set("unevolved", "evolved", "super_evolved")[t[i+1].Value] {
				i += 2
			}
		case "life", "attack":
			if i+2 < len(t) && set("==", "!=", "<", "<=", ">", ">=")[t[i+1].Value] {
				if isUnsigned(t[i+2]) {
					i += 3
				} else if i+4 < len(t) && set("own", "oppo")[t[i+2].Value] && t[i+3].Value == "." &&
					ir.ValidPlayerScalar(t[i+4].Value) {
					i += 5
				}
			}
		}
		if i == start {
			return i, false
		}
		term = true
		if i < len(t) && (t[i].Value == "and" || t[i].Value == "or") {
			i++
			term = false
			continue
		}
		break
	}
	return i, term
}
func isUnsigned(t syntax.Token) bool { _, ok := integer(t); return ok }
func isU16(t syntax.Token) bool {
	n, ok := integer(t)
	return ok && n <= 65535
}
func isCardID(t syntax.Token) bool { return t.Kind == syntax.Integer && len(t.Value) == 8 }
func isSign(t syntax.Token) bool   { return t.Value == "+" || t.Value == "-" }
func checkBindingAt(t []syntax.Token, start, end int, b map[string]bool, ds *[]syntax.Diagnostic) {
	if start >= len(t) {
		return
	}
	name := t[start].Value
	if t[start].Kind == syntax.Identifier && !set("self", "own", "oppo", "field", "all")[name] && !b[name] {
		diag(ds, "WBO-E009-BINDING-SCOPE", "错误", "绑定在使用前未定义: "+name, t[start].Span)
	}
}

func validateCondition(t []syntax.Token, ds *[]syntax.Diagnostic) {
	if evolutionCondition(t) || attackHistoryCondition(t) {
		return
	}
	if len(t) == 1 && set("overflow", "skybound_art", "super_skybound_art")[t[0].Value] {
		return
	}
	if damagedBindingCondition(t) {
		return
	}
	if deckDuplicatesCondition(t) {
		return
	}
	if len(t) > 0 && (t[0].Value == "count" || t[0].Value == "sum") {
		next, ok := parseEffectAmount(t, 0)
		if !ok || next+1 >= len(t) || !set("==", "!=", "<", "<=", ">", ">=")[t[next].Value] || !isUnsigned(t[next+1]) {
			diag(ds, "WBO-E001-SYNTAX", "错误", "count 条件必须写成 count(集合) 比较 整数", t[0].Span)
		}
		return
	}
	found := false
	for _, x := range t {
		if set("==", "!=", "<", "<=", ">", ">=")[x.Value] {
			found = true
		}
	}
	if !found {
		diag(ds, "WBO-E001-SYNTAX", "错误", "条件缺少比较运算符", t[0].Span)
	}
}
func validateFilterTail(t []syntax.Token, start int, ds *[]syntax.Diagnostic) {
	if len(t) == start {
		return
	}
	if t[start].Value != "where" {
		diag(ds, "WBO-E013-INVALID-FILTER", "错误", "集合后只能跟 where 筛选", t[start].Span)
	}
}
func tokenValuesPrefix(t []syntax.Token, n int) []string {
	if n > len(t) {
		n = len(t)
	}
	r := make([]string, n)
	for i := range r {
		r[i] = t[i].Value
	}
	return r
}

func validateTest(f *syntax.File, ds *[]syntax.Diagnostic) *TestFile {
	if len(f.Statements) < 3 {
		diag(ds, "WBO-E001-SYNTAX", "错误", "测试文件必须包含版本、use cards 和至少一个 scenario", fileSpan(f))
		return nil
	}
	v, ok := version(f.Statements[0], "wbotest")
	if !ok {
		shapeError(ds, f.Statements[0], "wbotest 主.次.修订;")
		return nil
	}
	if !strings.HasPrefix(v, "0.1.") {
		diag(ds, "WBO-E002-VERSION", "错误", "仅支持 wbotest 0.1.x", f.Statements[0].Span)
	}
	u := f.Statements[1].Tokens()
	if len(u) != 3 || u[0].Value != "use" || u[1].Value != "cards" || u[2].Kind != syntax.String || !f.Statements[1].Terminated {
		shapeError(ds, f.Statements[1], "use cards 字符串;")
		return nil
	}
	r := &TestFile{Path: f.Path, Version: v, UseCards: u[2].Value, File: f}
	names := map[string]bool{}
	for _, s := range f.Statements[2:] {
		t := s.Tokens()
		b := s.Blocks()
		if len(t) != 2 || t[0].Value != "scenario" || (t[1].Kind != syntax.String && t[1].Kind != syntax.TripleString) || len(b) != 1 {
			shapeError(ds, s, "scenario 字符串 { ... }")
			continue
		}
		if names[t[1].Value] {
			diag(ds, "WBT-E001-DUPLICATE-SCENARIO", "错误", "场景名称重复: "+t[1].Value, s.Span)
		}
		names[t[1].Value] = true
		validateScenario(b[0], ds)
		r.Scenarios = append(r.Scenarios, s)
	}
	strictValidateTest(r, ds)
	return r
}

func validateScenario(b []*syntax.Statement, ds *[]syntax.Diagnostic) {
	if len(b) != 4 || b[0].Word(0) != "seed" || b[1].Word(0) != "state" || b[2].Word(0) != "action" || b[3].Word(0) != "expect" {
		if len(b) > 0 {
			diag(ds, "WBO-E001-SYNTAX", "错误", "scenario 必须依次包含 seed、state、action、expect", b[0].Span)
		}
		return
	}
	st := b[0].Tokens()
	if len(st) != 2 || !b[0].Terminated {
		shapeError(ds, b[0], "seed 非负整数;")
	} else if _, err := strconv.ParseUint(st[1].Value, 10, 64); err != nil {
		rangeError(ds, st[1])
	}
	for _, i := range []int{1, 2, 3} {
		if len(b[i].Tokens()) != 1 || len(b[i].Blocks()) != 1 {
			shapeError(ds, b[i], b[i].Word(0)+" { ... }")
			return
		}
	}
	aliases := map[string]bool{}
	scanAliases(b[1].Blocks()[0], aliases, ds)
	a := b[2].Blocks()[0]
	primary := set("play", "engage", "evolve", "superevolve", "fuse", "attack", "end_turn", "advance")
	if len(a) == 0 || !primary[a[0].Word(0)] {
		diag(ds, "WBT-E005-ACTION-ORDER", "错误", "action 必须以一个主动作开始", b[2].Span)
	}
	for i, s := range a {
		if i > 0 && !set("select", "mode")[s.Word(0)] {
			diag(ds, "WBT-E005-ACTION-ORDER", "错误", "主动作后只允许 select 或 mode 响应", s.Span)
		}
		checkAliasUse(s, aliases, ds)
	}
	e := b[3].Blocks()[0]
	legal := 0
	for _, s := range e {
		if s.Word(0) == "legal" || s.Word(0) == "illegal" {
			legal++
		}
		validateAssertion(s, aliases, ds)
	}
	if legal != 1 {
		diag(ds, "WBT-E007-INVALID-ASSERTION", "错误", "expect 必须且只能包含一个 legal 或 illegal", b[3].Span)
	}
}

func scanAliases(body []*syntax.Statement, aliases map[string]bool, ds *[]syntax.Diagnostic) {
	for _, s := range body {
		t := s.Tokens()
		if len(t) >= 4 && (cardTypes[t[0].Value] || t[0].Value == "crest") && t[2].Value == "=" {
			if aliases[t[1].Value] {
				diag(ds, "WBT-E002-DUPLICATE-ALIAS", "错误", "实例别名重复: "+t[1].Value, t[1].Span)
			}
			aliases[t[1].Value] = true
		}
		for _, b := range s.Blocks() {
			scanAliases(b, aliases, ds)
		}
	}
}
func checkAliasUse(s *syntax.Statement, a map[string]bool, ds *[]syntax.Diagnostic) {
	t := s.Tokens()
	if s.Word(0) == "select" {
		entities, _, _ := parseSelectionResponse(t)
		for _, entity := range entities {
			if !a[entity.Value] {
				diag(ds, "WBT-E006-UNKNOWN-ALIAS", "错误", "未知实例别名: "+entity.Value, entity.Span)
			}
		}
		return
	}
	if len(t) > 1 && set("play", "engage", "evolve", "superevolve", "select", "attack")[t[0].Value] && !a[t[1].Value] {
		diag(ds, "WBT-E006-UNKNOWN-ALIAS", "错误", "未知实例别名: "+t[1].Value, t[1].Span)
	}
}
func validateAssertion(s *syntax.Statement, a map[string]bool, ds *[]syntax.Diagnostic) {
	h := s.Word(0)
	if !set("legal", "illegal", "unchanged", "own", "oppo", "rng", "all", "events")[h] && !a[h] {
		diag(ds, "WBT-E007-INVALID-ASSERTION", "错误", "未知断言或实例别名: "+h, s.Span)
	}
	t := s.Tokens()
	// 实例字段写错会静默退化成"永远不相等"，在这里直接拦住。
	if len(t) >= 3 && a[t[0].Value] && t[1].Value == "." && t[2].Value != "counter" && !instanceFields[t[2].Value] {
		diag(ds, "WBT-E007-INVALID-ASSERTION", "错误", "未知实例字段: "+t[2].Value, s.Span)
	}
	if h == "events" {
		v := tokenValues(s)
		valid := words(v) == "events contains ordered" || words(v) == "events excludes" || words(v) == "events exact"
		if !valid || len(s.Blocks()) != 1 {
			shapeError(ds, s, "events contains ordered|excludes|exact { ... }")
		}
	}
}

// instanceFields 与 internal/runner 的字段求值保持一致。
var instanceFields = set(
	"zone", "cost", "stats", "evolved", "super_evolved",
	"earthsigil", "countdown", "engaged", "attack_limit", "damage_reduction",
)

func validateCollection(l *Loaded, strict bool) {
	ids := map[string]*Card{}
	for _, c := range l.Cards {
		if old := ids[c.ID]; old != nil {
			diag(&l.Diagnostics, "WBO-E003-DUPLICATE-CARD", "错误", "卡牌 ID 重复: "+c.ID, c.Decl.Span)
		} else {
			ids[c.ID] = c
		}
		base := strings.TrimSuffix(filepath.Base(c.Path), filepath.Ext(c.Path))
		if base != c.ID {
			diag(&l.Diagnostics, "WBO-E005-INVALID-CARD-SHAPE", "错误", "文件名必须与卡牌 ID 一致", c.Decl.Span)
		}
		packDir := filepath.Base(filepath.Dir(c.Path))
		if n, e := strconv.Atoi(packDir); e == nil && n != c.Meta.Pack {
			diag(&l.Diagnostics, "WBO-E005-INVALID-CARD-SHAPE", "错误", "meta.pack 与所属目录不一致", c.Decl.Span)
		}
	}
	for _, c := range l.Dependencies {
		if ids[c.ID] == nil {
			ids[c.ID] = c
		}
	}
	for _, c := range l.Cards {
		checkStatementRefs(c.Decl, ids, l)
	}
	for _, c := range l.Dependencies {
		checkStatementRefs(c.Decl, ids, l)
	}
	for _, tf := range l.Tests {
		for _, s := range tf.Scenarios {
			checkStatementRefs(s, ids, l)
			checkDeclaredTypes(s, ids, &l.Diagnostics)
			validateScenarioCounters(s, ids, &l.Diagnostics)
		}
	}
	sev := "警告"
	if strict {
		sev = "错误"
	}
	keys := make([]string, 0, len(l.Unresolved))
	for id := range l.Unresolved {
		keys = append(keys, id)
	}
	sortStrings(keys)
	for _, id := range keys {
		diag(&l.Diagnostics, "WBO-E004-UNKNOWN-CARD", sev, "引用的卡牌不存在: "+id, l.Unresolved[id])
	}
}

func checkDeclaredTypes(s *syntax.Statement, ids map[string]*Card, ds *[]syntax.Diagnostic) {
	t := s.Tokens()
	if len(t) >= 4 && (cardTypes[t[0].Value] || t[0].Value == "crest") && t[2].Value == "=" {
		if c := ids[t[3].Value]; c != nil {
			if t[0].Value == "crest" {
				if c.Crest == nil {
					diag(ds, "WBT-E003-CARD-TYPE", "错误", "卡牌没有纹章定义", t[0].Span)
					return
				}
				c = c.Crest
			}
			if c.Type != t[0].Value {
				diag(ds, "WBT-E003-CARD-TYPE", "错误", "实例声明类型与卡牌定义不一致: "+t[1].Value, t[0].Span)
			}
			if len(s.Blocks()) == 1 {
				initialLife := 0
				if c.Stats != nil {
					initialLife = c.Stats[1]
				}
				for _, o := range s.Blocks()[0] {
					if o.Word(0) == "stats" && len(o.Tokens()) == 4 {
						initialLife, _ = integer(o.Tokens()[3])
					}
				}
				seenDamage := false
				for _, o := range s.Blocks()[0] {
					h := o.Word(0)
					if h == "damage_taken" && len(o.Tokens()) == 2 {
						amount, ok := integer(o.Tokens()[1])
						if seenDamage || !ok || amount < 0 || amount >= initialLife {
							diag(ds, "WBT-E004-INVALID-OVERRIDE", "错误", "damage_taken 必须小于初始生命值，且不能重复声明", o.Span)
						}
						seenDamage = true
					}
					if c.Type == "crest" && !set("counter", "countdown")[h] {
						diag(ds, "WBT-E004-INVALID-OVERRIDE", "错误", "纹章仅允许覆盖计数器和吟唱", o.Span)
					}
					if (set("stats", "damage_taken", "evolved", "super_evolved")[h] || abilities[h]) && c.Type != "follower" {
						diag(ds, "WBT-E004-INVALID-OVERRIDE", "错误", h+" override 只适用于随从", o.Span)
					}
					if set("earthsigil", "countdown", "engaged")[h] && c.Type != "amulet" && !(h == "countdown" && c.Type == "crest") {
						diag(ds, "WBT-E004-INVALID-OVERRIDE", "错误", h+" override 只适用于护符", o.Span)
					}
				}
			}
		}
	}
	for _, b := range s.Blocks() {
		for _, child := range b {
			checkDeclaredTypes(child, ids, ds)
		}
	}
}
func checkStatementRefs(s *syntax.Statement, ids map[string]*Card, l *Loaded) {
	checkEarthSigilRef(s, ids, l)
	t := s.Tokens()
	for i := 0; i+1 < len(t); i++ {
		if (t[i].Value == "card" || t[i].Value == "=" || t[i].Value == "crest") && t[i+1].Kind == syntax.Integer && len(t[i+1].Value) == 8 {
			card := ids[t[i+1].Value]
			if card == nil {
				l.Unresolved[t[i+1].Value] = t[i+1].Span
			} else if t[i].Value == "crest" && card.Crest == nil {
				diag(&l.Diagnostics, "WBO-E008-TYPE-MISMATCH", "错误", "引用的卡牌没有纹章定义: "+card.ID, t[i+1].Span)
			}
		}
	}
	for _, b := range s.Blocks() {
		for _, x := range b {
			checkStatementRefs(x, ids, l)
		}
	}
}
func checkEarthSigilRef(s *syntax.Statement, ids map[string]*Card, l *Loaded) {
	t := s.Tokens()
	id := strconv.Itoa(ir.MagicSedimentCardID)
	if len(t) == 3 && t[0].Value == "add" && t[2].Value == "earthsigil" && intToken(t[1]) > 0 && ids[id] == nil {
		l.Unresolved[id] = s.Span
	}
}
func sortStrings(v []string) {
	for i := 1; i < len(v); i++ {
		for j := i; j > 0 && v[j] < v[j-1]; j-- {
			v[j], v[j-1] = v[j-1], v[j]
		}
	}
}
func rangeError(ds *[]syntax.Diagnostic, t syntax.Token) {
	diag(ds, "WBO-E014-INTEGER-RANGE", "错误", "整数格式无效或超出范围", t.Span)
}
func unknown(ds *[]syntax.Diagnostic, t syntax.Token, what string) {
	diag(ds, "WBO-E007-UNKNOWN-NAME", "错误", "未知"+what+": "+t.Value, t.Span)
}
func fileSpan(f *syntax.File) syntax.Span {
	return syntax.Span{File: f.Path, Start: syntax.Position{Line: 1, Column: 1}, End: syntax.Position{Byte: len(f.Source), Line: 1, Column: 1}}
}
