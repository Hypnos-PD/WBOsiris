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
	abilities = set("ward", "storm", "rush", "bane", "drain", "intimidate", "barrier", "stealth", "aura", "ability_target_guard", "cannot_attack", "cannot_attack_follower", "cannot_attack_leader")
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
	if event == "summoned" {
		bindings["summoned"] = true
	}
	if event == "engaged" {
		bindings["engaged"] = true
	}
	if event == "discarded" {
		bindings["discarded"] = true
	}
	if event == "leaves" {
		bindings["left"] = true
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
			if t[n].Value == "count" && t[n+1].Value == "(" {
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
		if h == "damage_reduction" {
			if len(t) != 2 || !s.Terminated {
				shapeError(ds, s, "damage_reduction 非负整数;")
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
			if len(t) != 2 || len(b) != 1 {
				shapeError(ds, s, h+" 整数 { ... }")
			} else {
				if _, ok := integer(t[1]); !ok {
					rangeError(ds, t[1])
				}
				validateEffectBlock(b[0], ds, bindings, "")
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
				for _, x := range t {
					if x.Value == "summoned" {
						ev = "summoned"
					}
					if x.Value == "engaged" {
						ev = "engaged"
					}
					if x.Value == "discarded" {
						ev = "discarded"
					}
					if x.Value == "leaves" {
						ev = "leaves"
					}
				}
				validateEffectBlock(b[0], ds, bindings, ev)
			}
			continue
		case "replace":
			if words(tokenValuesPrefix(t, len(t))) != "replace self leaving field" || len(b) != 1 {
				shapeError(ds, s, "replace self leaving field { ... }")
			} else {
				validateEffectBlock(b[0], ds, bindings, "")
			}
			continue
		case "choose", "require", "random":
			end, setOK := parseTargetSet(t, 3)
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
				end++
			}
			if setOK && end < len(t) && t[end].Value == "where" {
				end, setOK = parseWhere(t, end)
			}
			if setOK && end < len(t) && set("highest", "lowest")[t[end].Value] {
				setOK = end+1 < len(t) && set("attack", "life", "cost")[t[end+1].Value]
				end += 2
			}
			if setOK && end < len(t) && t[end].Value == "count" {
				if end+1 < len(t) {
					count, ok := integer(t[end+1])
					setOK = ok && count > 0
					end += 2
				} else {
					setOK = false
				}
			}
			if len(t) < 4 || t[1].Kind != syntax.Identifier || t[2].Value != "from" || !s.Terminated || len(b) > 0 || !setOK || end != len(t) {
				shapeError(ds, s, h+" 绑定 from 集合 [other] [where ...] [highest|lowest attack|life|cost] [count 正整数];")
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
			if len(t) != 1 || len(b) != 1 || len(b[0]) < 2 {
				shapeError(ds, s, "mode { 至少两个 option }")
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
	known := set("draw", "add", "summon", "damage", "heal", "buff", "gain", "restore", "destroy", "banish", "discard", "remove", "return", "evolve", "superevolve", "reanimate", "reduce", "spellboost", "transform", "set_attack_limit", "set")
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
		end, good := parseValueRef(t, 2)
		checkBindingAt(t, 2, end, bindings, ds)
		if good && (end == 5 && t[4].Value == "leader" || end == 3 && t[2].Value == "leaders" || end == 5 && values(t[2:5]) == "all . leaders") {
			good = false
		}
		if good {
			end, good = parseEffectAmount(t, end)
		}
		ok = t[1].Value == "life" && good && end == len(t)
	case "set_attack_limit":
		ok = len(t) == 3 && t[1].Value == "self" && isUnsigned(t[2])
	case "draw":
		ok = len(t) == 2 && (isUnsigned(t[1]) || t[1].Value == "all")
		if len(t) > 2 && (isUnsigned(t[1]) || t[1].Value == "all") && len(t) >= 6 && t[2].Value == "from" && t[3].Value == "deck" {
			end, good := parseWhere(t, 4)
			ok = good && end == len(t)
		}
		if t[1].Value == "all" && len(t) == 2 {
			ok = false
		}
	case "add":
		if len(t) == 4 && isUnsigned(t[1]) && t[2].Value == "counter" && t[3].Kind == syntax.Identifier && ir.ValidCounterName(t[3].Value) {
			ok = true
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
				end++
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
			if good && end < len(t) {
				end, good = parseWhere(t, end)
			}
			ok = good && end == len(t)
		}
	case "buff":
		end, good := parseValueRef(t, 1)
		if good && end < len(t) && t[end].Value == "other" {
			end++
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
		ok = len(t) == 5 && (t[1].Value == "own" || t[1].Value == "oppo") && t[2].Value == "." && set("life", "pp", "maxpp", "ep", "sep", "combo", "shadows")[t[3].Value] && isUnsigned(t[4])
	case "restore":
		ok = len(t) == 4 && set("own", "oppo")[t[1].Value] && t[2].Value == "." && t[3].Value == "pp"
	case "destroy", "banish", "discard":
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
		if good && end < len(t) {
			end, good = parseWhere(t, end)
		}
		ok = good && end == len(t)
		checkBindingAt(t, 1, end, bindings, ds)
	case "remove":
		if len(t) >= 4 && abilities[t[1].Value] && t[2].Value == "from" {
			end, good := parseValueRef(t, 3)
			if good && end < len(t) && t[end].Value == "other" {
				end++
			}
			if good && end < len(t) {
				end, good = parseWhere(t, end)
			}
			ok = good && end == len(t)
			checkBindingAt(t, 3, end, bindings, ds)
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
			if good && end < len(t) && isUnsigned(t[end]) {
				end++
				ok = t[1].Value == "countdown" && end == len(t) || t[1].Value == "cost" && end+2 == len(t) && t[end].Value == "minimum" && isUnsigned(t[end+1])
			}
			checkBindingAt(t, 2, end, bindings, ds)
		}
	case "spellboost":
		end, good := parseValueRef(t, 1)
		ok = good && end+1 == len(t) && isUnsigned(t[end])
		checkBindingAt(t, 1, end, bindings, ds)
	case "transform":
		end, good := parseValueRef(t, 1)
		ok = good && end+3 <= len(t) && t[end].Value == "into" && t[end+1].Value == "card" && isCardID(t[end+2]) && (end+3 == len(t) || end+5 == len(t) && t[end+3].Value == "preserving" && t[end+4].Value == "materials")
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
		if t[i].Value == "self" && set("attack", "life", "cost")[t[i+2].Value] ||
			set("own", "oppo")[t[i].Value] && set("combo", "pp", "maxpp", "life", "ep", "sep", "shadows")[t[i+2].Value] {
			return i + 3, true
		}
	}
	if i+1 >= len(t) || t[i].Value != "count" || t[i+1].Value != "(" {
		return i, false
	}
	end, ok := parseCountSource(t, i+2)
	if ok && end < len(t) && t[end].Value == "where" {
		end, ok = parseWhere(t, end)
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
	if i+2 >= len(t) || t[i+1].Value != "." || !set("deck", "hand", "field", "graveyard", "banished", "destroyed")[t[i+2].Value] {
		return i, false
	}
	i += 3
	if i+1 < len(t) && t[i].Value == "." && set("followers", "spells", "amulets")[t[i+1].Value] {
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
	return parseTargetSet(t, i)
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
		case "card":
			if i+1 < len(t) && isCardID(t[i+1]) {
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
		case "life", "cost":
			if i+2 < len(t) && set("==", "!=", "<", "<=", ">", ">=")[t[i+1].Value] && isUnsigned(t[i+2]) {
				i += 3
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
func isCardID(t syntax.Token) bool   { return t.Kind == syntax.Integer && len(t.Value) == 8 }
func isSign(t syntax.Token) bool     { return t.Value == "+" || t.Value == "-" }
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
	if evolutionCondition(t) {
		return
	}
	if len(t) == 1 && t[0].Value == "overflow" {
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
		if len(t) >= 4 && cardTypes[t[0].Value] && t[2].Value == "=" {
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
	if h == "events" {
		t := tokenValues(s)
		valid := words(t) == "events contains ordered" || words(t) == "events excludes" || words(t) == "events exact"
		if !valid || len(s.Blocks()) != 1 {
			shapeError(ds, s, "events contains ordered|excludes|exact { ... }")
		}
	}
}

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
	check := func(s *syntax.Statement) {
		checkEarthSigilRef(s, ids, l)
		t := s.Tokens()
		for i := 0; i+1 < len(t); i++ {
			if t[i].Value == "card" && t[i+1].Kind == syntax.Integer && len(t[i+1].Value) == 8 {
				if ids[t[i+1].Value] == nil {
					l.Unresolved[t[i+1].Value] = t[i+1].Span
				}
			}
		}
		for _, b := range s.Blocks() {
			for _, x := range b {
				checkStatementRefs(x, ids, l)
			}
		}
	}
	for _, c := range l.Cards {
		for _, s := range c.Effect {
			check(s)
		}
	}
	for _, c := range l.Dependencies {
		for _, s := range c.Effect {
			check(s)
		}
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
	if len(t) >= 4 && cardTypes[t[0].Value] && t[2].Value == "=" {
		if c := ids[t[3].Value]; c != nil {
			if c.Type != t[0].Value {
				diag(ds, "WBT-E003-CARD-TYPE", "错误", "实例声明类型与卡牌定义不一致: "+t[1].Value, t[0].Span)
			}
			if len(s.Blocks()) == 1 {
				for _, o := range s.Blocks()[0] {
					h := o.Word(0)
					if (set("stats", "evolved", "super_evolved")[h] || abilities[h]) && c.Type != "follower" {
						diag(ds, "WBT-E004-INVALID-OVERRIDE", "错误", h+" override 只适用于随从", o.Span)
					}
					if set("earthsigil", "countdown", "engaged")[h] && c.Type != "amulet" {
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
		if (t[i].Value == "card" || t[i].Value == "=") && t[i+1].Kind == syntax.Integer && len(t[i+1].Value) == 8 && ids[t[i+1].Value] == nil {
			l.Unresolved[t[i+1].Value] = t[i+1].Span
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
