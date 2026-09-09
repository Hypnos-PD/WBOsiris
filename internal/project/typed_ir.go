package project

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"wbo/internal/ir"
	"wbo/internal/ruleset"
	"wbo/internal/syntax"
)

type idScope struct {
	parent string
	seen   map[string]int
	ids    map[string]bool
}

func spanIR(sp syntax.Span, sid string) ir.SourceSpan {
	return ir.SourceSpan{
		SourceID: sid, StartByte: uint32(sp.Start.Byte), EndByte: uint32(sp.End.Byte),
		StartLine: uint32(sp.Start.Line), StartColumn: uint32(sp.Start.Column),
		EndLine: uint32(sp.End.Line), EndColumn: uint32(sp.End.Column),
	}
}

func originIR(sp syntax.Span, sid string) ir.Origin {
	return ir.Origin{Primary: spanIR(sp, sid)}
}

func newScope(parent string, ids map[string]bool) *idScope {
	return &idScope{parent: parent, seen: map[string]int{}, ids: ids}
}
func (s *idScope) next(st *syntax.Statement) (string, error) {
	fp := statementFingerprint(st)
	n := s.seen[fp]
	s.seen[fp]++
	id := nodeID(s.parent, fp, strconv.Itoa(n))
	if s.ids[id] {
		return "", fmt.Errorf("WBO-E015-ID-COLLISION: 节点 ID 碰撞")
	}
	s.ids[id] = true
	return id, nil
}
func statementFingerprint(s *syntax.Statement) string {
	var b strings.Builder
	b.WriteString(words(tokenValues(s)))
	for _, block := range s.Blocks() {
		b.WriteByte('{')
		for _, x := range block {
			b.WriteString(statementFingerprint(x))
			b.WriteByte(';')
		}
		b.WriteByte('}')
	}
	return contentHash([]byte(b.String()))
}

func compileCardsTyped(l *Loaded) (ir.CardPack, error) {
	cards := append([]*Card(nil), l.Cards...)
	sort.Slice(cards, func(i, j int) bool { return cards[i].ID < cards[j].ID })
	sources := []ir.Source{}
	out := []ir.Card{}
	ids := map[string]bool{}
	for _, c := range cards {
		path := sourcePath(l, c.Path)
		sid := sourceID(path)
		sources = append(sources, ir.Source{SourceID: sid, Path: path, ContentHash: contentHash(c.File.Source), Span: spanIR(fileSpan(c.File), sid)})
		card, err := compileTypedCard(c, sid, ids)
		if err != nil {
			return ir.CardPack{}, err
		}
		out = append(out, card)
	}
	return ir.CardPack{Format: ir.Format, ContainerVersion: ir.ContainerVersion, Encoding: ir.Encoding, Kind: "card-pack", IRVersion: ir.IRVersion, SourceLanguageVersion: ir.SourceLanguageVersion, RequiredFeatures: []string{}, UnresolvedReferences: []string{}, Sources: sources, Cards: out}, nil
}

func compileTypedCard(c *Card, sid string, ids map[string]bool) (ir.Card, error) {
	counters := map[string]int{}
	intrinsic := []string{}
	restrictions := []ir.Restriction{}
	states := []ir.IntrinsicState{}
	abilitiesIR := []ir.Ability{}
	fusion := []ir.FusionAbility{}
	play := []ir.Effect{}
	kind := "card"
	if c.Type == "crest" {
		kind = "crest"
	}
	scope := newScope(nodeID(kind, c.ID), ids)
	evolveIDs := []string{}
	superIDs := []struct{ id, relation string }{}
	for _, s := range c.Effect {
		h := s.Word(0)
		switch {
		case h == "counter":
			counters[s.Word(1)] = intAt(s, 2)
		case abilities[h] && len(s.Blocks()) == 0:
			intrinsic = append(intrinsic, h)
		case h == "unplayable":
			restrictions = append(restrictions, ir.Restriction{Kind: "unplayable"})
		case h == "countdown":
			states = append(states, ir.IntrinsicState{Kind: "countdown", Initial: intAt(s, 1)})
		case h == "damage_reduction":
			states = append(states, ir.IntrinsicState{Kind: "damage_reduction", Initial: intAt(s, 1)})
		case h == "attack_limit":
			states = append(states, ir.IntrinsicState{Kind: "attack_limit", Initial: intAt(s, 1)})
		case h == "earthsigil":
			states = append(states, ir.IntrinsicState{Kind: "earthsigil", Initial: 1})
		case h == "fusion":
			n, err := compileFusion(s, sid, scope, ids)
			if err != nil {
				return ir.Card{}, err
			}
			fusion = append(fusion, n)
		case len(s.Blocks()) > 0 && set("fanfare", "lastwords", "attack", "clash", "evolve", "superevolve", "engage", "enhance", "spellboost", "when", "replace")[h]:
			n, err := compileAbility(s, sid, scope, ids)
			if err != nil {
				return ir.Card{}, err
			}
			abilitiesIR = append(abilitiesIR, n)
			id := n.ID
			if h == "evolve" {
				evolveIDs = append(evolveIDs, id)
			}
			if h == "superevolve" {
				rel := "independent"
				if len(s.Tokens()) == 3 {
					rel = s.Word(1)
				}
				superIDs = append(superIDs, struct{ id, relation string }{id, rel})
			}
		default:
			n, err := compileEffect(s, sid, scope, ids)
			if err != nil {
				return ir.Card{}, err
			}
			play = append(play, n)
		}
	}
	plans := []ir.ActionPlan{}
	if len(evolveIDs) == 1 {
		plans = append(plans, ir.ActionPlan{Action: "evolve", Steps: []ir.PlanStep{{AbilityID: evolveIDs[0], Frame: "new"}}})
		if len(superIDs) == 0 {
			plans = append(plans, ir.ActionPlan{Action: "superevolve", Steps: []ir.PlanStep{{AbilityID: evolveIDs[0], Frame: "new"}}})
		}
	}
	for _, x := range superIDs {
		steps := []ir.PlanStep{}
		switch x.relation {
		case "replaces":
			steps = append(steps, ir.PlanStep{AbilityID: x.id, Frame: "new"})
		case "extends":
			steps = append(steps, ir.PlanStep{AbilityID: evolveIDs[0], Frame: "new"}, ir.PlanStep{AbilityID: x.id, Frame: "continue"})
		default:
			if len(evolveIDs) == 1 {
				steps = append(steps, ir.PlanStep{AbilityID: evolveIDs[0], Frame: "new"})
			}
			steps = append(steps, ir.PlanStep{AbilityID: x.id, Frame: "new"})
		}
		plans = append(plans, ir.ActionPlan{Action: "superevolve", Steps: steps})
	}
	loc := map[string]ir.Locale{}
	for _, code := range locales {
		x := c.Locales[code]
		loc[code] = ir.Locale{Name: x.Name, Text: x.Text}
	}
	m := ir.Card{ID: mustInt(c.ID), CardType: c.Type, Cost: c.Cost, Traits: unique(c.Traits), Intrinsic: intrinsic, IntrinsicState: states, Restrictions: restrictions, Abilities: abilitiesIR, FusionAbilities: fusion, PlayEffects: play, ActionPlans: plans, Meta: ir.Meta{Pack: c.Meta.Pack, Class: c.Meta.Class, Rarity: c.Meta.Rarity}, Locales: loc, Origin: originIR(c.Decl.Span, sid)}
	m.Counters = counters
	if c.Stats != nil {
		m.Stats = &ir.Stats{Attack: c.Stats[0], Life: c.Stats[1]}
	}
	if c.Crest != nil {
		crest, err := compileTypedCard(c.Crest, sid, ids)
		if err != nil {
			return ir.Card{}, err
		}
		m.Crest = &ir.CrestDefinition{Counters: crest.Counters, Abilities: crest.Abilities, Locales: crest.Locales, Origin: crest.Origin}
		for _, state := range crest.IntrinsicState {
			if state.Kind == "countdown" {
				m.Crest.Countdown = state.Initial
			}
		}
	}
	return m, nil
}

func compileAbility(s *syntax.Statement, sid string, scope *idScope, ids map[string]bool) (ir.Ability, error) {
	id, err := scope.next(s)
	if err != nil {
		return ir.Ability{}, err
	}
	h := s.Word(0)
	var trigger ir.Trigger = ir.SimpleTrigger{Kind: h}
	if h == "engage" || h == "enhance" {
		trigger = ir.CostTrigger{Kind: h, Cost: intAt(s, 1)}
	}
	if h == "when" {
		trigger = eventPatternIR(s.Tokens())
	}
	if h == "replace" {
		trigger = ir.ReplacementTrigger{Kind: "replacement", Subject: valueRefIR(s.Tokens(), 1), From: "field", Phase: "before"}
	}
	rel := "independent"
	if h == "superevolve" && len(s.Tokens()) == 3 {
		rel = s.Word(1)
	}
	body, err := compileEffectBlock(s.Blocks()[0], sid, id, ids)
	if err != nil {
		return ir.Ability{}, err
	}
	return ir.Ability{ID: id, Trigger: trigger, Relation: rel, Body: body, Origin: originIR(s.Span, sid)}, nil
}
func compileFusion(s *syntax.Statement, sid string, scope *idScope, ids map[string]bool) (ir.FusionAbility, error) {
	id, err := scope.next(s)
	if err != nil {
		return ir.FusionAbility{}, err
	}
	source, end := setExprIR(s.Tokens(), 3)
	var pred ir.Predicate
	if end < len(s.Tokens()) {
		pred, _ = filterIR(s.Tokens(), end)
	}
	body, err := compileEffectBlock(s.Blocks()[0], sid, id, ids)
	if err != nil {
		return ir.FusionAbility{}, err
	}
	return ir.FusionAbility{ID: id, MaterialFilter: ir.MaterialFilter{Kind: "material_filter", Source: source, Predicate: pred, ExcludeSource: true, Minimum: 1}, Body: body, Origin: originIR(s.Span, sid)}, nil
}
func compileEffectBlock(body []*syntax.Statement, sid, parent string, ids map[string]bool) ([]ir.Effect, error) {
	scope := newScope(parent, ids)
	out := make([]ir.Effect, 0, len(body))
	for _, s := range body {
		n, err := compileEffect(s, sid, scope, ids)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, nil
}

func compileEffect(s *syntax.Statement, sid string, scope *idScope, ids map[string]bool) (ir.Effect, error) {
	id, err := scope.next(s)
	if err != nil {
		return nil, err
	}
	t := s.Tokens()
	h := s.Word(0)
	base := ir.NodeBase{ID: id, Origin: originIR(s.Span, sid)}
	switch h {
	case "repeat":
		times, expr := numericIR(t, 1)
		body, err := compileEffectBlock(s.Blocks()[0], sid, id+"/body", ids)
		return ir.RepeatEffect{NodeBase: base, Kind: "repeat", Times: times, TimesExpr: expr, Body: body}, err
	case "choose", "require", "random":
		src, end := setExprIR(t, 3)
		if end < len(t) && t[end].Value == "or" {
			src = ir.CharacterSetRef{Kind: "characters", Side: t[3].Value}
			end += 4
		}
		if end < len(t) && t[end].Value == "other" {
			src = ir.ExcludeRef{Kind: "exclude", Source: src, Value: ir.SelfRef{Kind: "self"}}
			end++
		}
		if end < len(t) && t[end].Value == "where" {
			pred, next := filterIR(t, end)
			src = ir.FilterRef{Kind: "filter", Source: src, Predicate: pred}
			end = next
		}
		var extremum *ir.SelectionExtremum
		if end < len(t) && (t[end].Value == "highest" || t[end].Value == "lowest") {
			extremum = &ir.SelectionExtremum{Direction: t[end].Value, Field: t[end+1].Value}
			end += 2
		}
		count := 0
		if end < len(t) && t[end].Value == "count" {
			count = intToken(t[end+1])
		}
		kind := h
		if h == "random" {
			kind = "random_choose"
		}
		return ir.SelectionEffect{NodeBase: base, Kind: kind, Policy: map[string]string{"choose": "optional", "require": "required", "random": "random"}[h], Binding: t[1].Value, Source: src, Count: count, Extremum: extremum}, nil
	case "if":
		blocks := s.Blocks()
		then, err := compileEffectBlock(blocks[0], sid, id+"/then", ids)
		if err != nil {
			return nil, err
		}
		els := []ir.Effect{}
		if len(blocks) == 2 {
			els, err = compileEffectBlock(blocks[1], sid, id+"/else", ids)
			if err != nil {
				return nil, err
			}
		}
		return ir.IfEffect{NodeBase: base, Kind: "if", Condition: conditionIR(t[1:]), Then: then, Else: els}, nil
	case "mode":
		opts := []ir.ModeOption{}
		for _, o := range s.Blocks()[0] {
			labels, statements, err := modeOptionParts(o.Blocks()[0])
			if err != nil {
				return nil, err
			}
			body, err := compileEffectBlock(statements, sid, id+"/option/"+o.Word(1), ids)
			if err != nil {
				return nil, err
			}
			opts = append(opts, ir.ModeOption{ID: intAt(o, 1), Body: body, Origin: originIR(o.Span, sid), Labels: labels})
		}
		return ir.ModeEffect{NodeBase: base, Kind: "mode", Options: opts}, nil
	case "earthrite", "necromancy":
		resource := "shadows"
		if h == "earthrite" {
			resource = "earthsigil"
		}
		body, err := compileEffectBlock(s.Blocks()[0], sid, id, ids)
		if err != nil {
			return nil, err
		}
		return ir.PayResourceEffect{NodeBase: base, Kind: "pay_resource", Resource: resource, Amount: intAt(s, 1), OnPaid: body}, nil
	case "draw":
		e := ir.DrawEffect{NodeBase: base, Kind: "draw", Owner: "own", SourceZone: "deck", All: t[1].Value == "all", Output: "drawn"}
		if t[1].Value != "all" {
			e.Count = intAt(s, 1)
		}
		if len(t) > 2 {
			e.Predicate, _ = filterIR(t, 4)
		}
		return e, nil
	case "add":
		if len(t) == 4 && t[2].Value == "counter" {
			return ir.AdjustEffect{NodeBase: base, Kind: "adjust_counter", Field: t[3].Value, Delta: intToken(t[1])}, nil
		}
		if t[1].Kind == syntax.Integer && t[2].Value == "card" {
			return ir.CardEffect{NodeBase: base, Kind: "add_card", Owner: "own", Count: intAt(s, 1), CardID: intToken(t[3]), Destination: "hand"}, nil
		} else if t[1].Value == "combo" {
			return ir.AdjustEffect{NodeBase: base, Kind: "adjust_resource", Owner: "own", Resource: "combo", Delta: intToken(t[2])}, nil
		} else if t[2].Value == "earthsigil" {
			return ir.AdjustEffect{NodeBase: base, Kind: "adjust_earthsigil", Owner: "own", Delta: intToken(t[1])}, nil
		} else {
			return keywordEffectIR(base, "add_keyword", t), nil
		}
	case "summon":
		if t[1].Value == "copies" {
			target := valueRefIR(t, 3)
			if end := valueRefEnd(t, 3); end < len(t) {
				predicate, _ := filterIR(t, end)
				target = ir.FilterRef{Kind: "filter", Source: target, Predicate: predicate}
			}
			return ir.CardEffect{NodeBase: base, Kind: "summon_copies", Owner: "own", Target: target, Output: "summoned"}, nil
		}
		return ir.CardEffect{NodeBase: base, Kind: "summon", Owner: "own", Count: intAt(s, 1), CardID: intToken(t[3]), Output: "summoned"}, nil
	case "grant":
		target := valueRefIR(t, 1)
		if end := valueRefEnd(t, 1); end < len(t) {
			predicate, _ := filterIR(t, end)
			target = ir.FilterRef{Kind: "filter", Source: target, Predicate: predicate}
		}
		labels, statement, err := grantParts(s)
		if err != nil {
			return nil, err
		}
		ability, err := compileAbility(statement, sid, newScope(base.ID, ids), ids)
		return ir.GrantEffect{NodeBase: base, Kind: "grant_ability", Target: target, Ability: ability, Labels: labels}, err
	case "damage", "heal":
		end := valueRefEnd(t, 1)
		e := ir.TargetEffect{NodeBase: base, Kind: h, Target: valueRefIR(t, 1)}
		e.Amount, e.AmountExpr = numericIR(t, end)
		end, _ = parseEffectAmount(t, end)
		if end < len(t) && t[end].Value == "distributed" {
			e.Distribution = "field_entry_order"
			end++
			if end < len(t) && t[end].Value == "overflow" {
				e.Overflow = valueRefIR(t, end+1)
				end += 4
			}
		}
		if end < len(t) {
			e.Predicate, _ = filterIR(t, end)
		}
		if h == "damage" {
			e.DamageType = "effect"
		}
		return e, nil
	case "buff":
		end := valueRefEnd(t, 1)
		target := valueRefIR(t, 1)
		if end < len(t) && t[end].Value == "other" {
			target = ir.ExcludeRef{Kind: "exclude", Source: target, Value: ir.SelfRef{Kind: "self"}}
			end++
		}
		e := ir.TargetEffect{NodeBase: base, Kind: "buff_stats", Target: target}
		e.AttackDelta, e.AttackExpr, end = signedNumericIR(t, end)
		e.LifeDelta, e.LifeExpr, end = signedNumericIR(t, end+1)
		if end < len(t) && t[end].Value == "where" {
			e.Predicate, end = filterIR(t, end)
		}
		if end < len(t) {
			e.Until = effectDurationIR(t, end)
		}
		return e, nil
	case "gain":
		if len(t) == 4 && t[2].Value == "crest" {
			return ir.CardEffect{NodeBase: base, Kind: "gain_crest", Owner: t[1].Value, CardID: intToken(t[3])}, nil
		}
		return ir.AdjustEffect{NodeBase: base, Kind: "adjust_resource", Owner: t[1].Value, Resource: t[3].Value, Delta: intToken(t[4])}, nil
	case "restore":
		return ir.AdjustEffect{NodeBase: base, Kind: "restore_resource", Owner: t[1].Value, Resource: "pp"}, nil
	case "destroy", "banish", "discard":
		e := ir.TargetEffect{NodeBase: base, Kind: h, Target: valueRefIR(t, 1)}
		if h == "destroy" {
			e.Output = "destroyed"
			if targets, batch := destructionBatchTargets(t); batch {
				ref := ir.DestructionBatchRef{Kind: "destruction_batch"}
				for _, target := range targets {
					ref.Targets = append(ref.Targets, valueRefIR([]syntax.Token{target}, 0))
				}
				e.Target = ref
				return e, nil
			}
		}
		end := valueRefEnd(t, 1)
		if end < len(t) {
			e.Predicate, _ = filterIR(t, end)
		}
		return e, nil
	case "remove":
		return keywordEffectIR(base, "remove_keyword", t), nil
	case "set_attack_limit":
		return ir.TargetEffect{NodeBase: base, Kind: "set_attack_limit", Target: valueRefIR(t, 1), Amount: intToken(t[2])}, nil
	case "set":
		end := valueRefEnd(t, 2)
		amount, expr := numericIR(t, end)
		return ir.TargetEffect{NodeBase: base, Kind: "set_life", Target: valueRefIR(t, 2), Amount: amount, AmountExpr: expr}, nil
	case "return":
		end := valueRefEnd(t, 1)
		e := ir.TargetEffect{NodeBase: base, Kind: "return", Target: valueRefIR(t, 1), Destination: t[end+1].Value}
		if t[end+1].Value == "deck" {
			e.DeckInsertion = "uniform_random_position"
		}
		return e, nil
	case "evolve", "superevolve":
		form := "evolved"
		if h == "superevolve" {
			form = "super_evolved"
		}
		return ir.TargetEffect{NodeBase: base, Kind: "silent_evolve", Target: valueRefIR(t, 1), Form: form}, nil
	case "reanimate":
		return ir.CardEffect{NodeBase: base, Kind: "reanimate", Owner: "own", MaxCost: intAt(s, 1), TieBreak: "random", Output: "summoned"}, nil
	case "reduce":
		end := valueRefEnd(t, 2)
		e := ir.AdjustEffect{NodeBase: base, Kind: "adjust_entity_field", Field: t[1].Value, Target: valueRefIR(t, 2), Delta: -intToken(t[end])}
		if t[1].Value == "cost" {
			e.Minimum = intToken(t[end+2])
		}
		return e, nil
	case "spellboost":
		end := valueRefEnd(t, 1)
		return ir.AdjustEffect{NodeBase: base, Kind: "spellboost", Target: valueRefIR(t, 1), Times: intToken(t[end])}, nil
	case "transform":
		end := valueRefEnd(t, 1)
		e := ir.CardEffect{NodeBase: base, Kind: "transform", Target: valueRefIR(t, 1), CardID: intToken(t[end+2]), PreserveInstanceID: true, PreserveMaterials: true}
		end += 3
		if end < len(t) && t[end].Value == "preserving" {
			end += 2
		}
		if end < len(t) {
			predicate, _ := filterIR(t, end)
			e.Target = ir.FilterRef{Kind: "filter", Source: e.Target, Predicate: predicate}
		}
		return e, nil
	default:
		return nil, fmt.Errorf("WBO-E017-IR-INCOMPATIBLE: 无法编译效果构造 %q", h)
	}
}

func valueRefIR(t []syntax.Token, i int) ir.Ref {
	if i >= len(t) {
		return nil
	}
	if i+2 < len(t) && t[i].Value == "all" && t[i+1].Value == "." && t[i+2].Value == "leaders" {
		return ir.LeaderSetRef{Kind: "leaders", ValueType: "leaders"}
	}
	if t[i].Value == "self" {
		return ir.SelfRef{Kind: "self", ValueType: "entity"}
	}
	if set("target", "summoned", "drawn", "engaged")[t[i].Value] || t[i].Kind == syntax.Identifier && !set("own", "oppo", "field")[t[i].Value] {
		return ir.BindingRef{Kind: "binding", Name: t[i].Value}
	}
	if i+2 < len(t) && set("own", "oppo")[t[i].Value] && t[i+1].Value == "." && t[i+2].Value == "leader" {
		return ir.LeaderRef{Kind: "leader", Side: t[i].Value, ValueType: "leader"}
	}
	x, _ := setExprIR(t, i)
	return x
}
func valueRefEnd(t []syntax.Token, i int) int {
	end, ok := parseValueRef(t, i)
	if !ok {
		return i
	}
	return end
}
func setExprIR(t []syntax.Token, i int) (ir.Ref, int) {
	start := i
	end, ok := parseTargetSet(t, i)
	if !ok {
		return nil, start
	}
	side := ""
	zone := t[start].Value
	member := "card"
	if zone == "own" || zone == "oppo" {
		side = zone
		zone = t[start+2].Value
	}
	memberEnd := end
	thisTurn := end >= 2 && values(t[end-2:end]) == "this turn"
	if thisTurn {
		memberEnd -= 2
	}
	if memberEnd >= 2 && set("followers", "spells", "amulets")[t[memberEnd-1].Value] {
		member = strings.TrimSuffix(t[memberEnd-1].Value, "s")
	}
	if thisTurn {
		return ir.HistoryRef{Kind: "history", Side: side, Member: member, Window: "this_turn"}, end
	}
	return ir.ZoneRef{Kind: "zone", Side: side, Zone: zone, Member: member}, end
}
func filterIR(t []syntax.Token, i int) (ir.Predicate, int) {
	end, ok := parseWhere(t, i)
	if !ok {
		return nil, i
	}
	terms := []ir.Predicate{}
	groups := []ir.Predicate{}
	flush := func() {
		if len(terms) == 1 {
			groups = append(groups, terms[0])
		} else {
			groups = append(groups, ir.AndPredicate{Kind: "and", Terms: terms})
		}
		terms = nil
	}
	j := i + 1
	for j < end {
		switch t[j].Value {
		case "spellboost":
			terms = append(terms, ir.FieldPredicate{Kind: "has_spellboost"})
			j++
		case "keyword":
			terms = append(terms, ir.FieldPredicate{Kind: "has_keyword", Keyword: t[j+1].Value})
			j += 2
		case "card":
			terms = append(terms, ir.FieldPredicate{Kind: "has_card", CardID: intToken(t[j+1])})
			j += 2
		case "type":
			terms = append(terms, ir.FieldPredicate{Kind: "has_type", CardType: t[j+1].Value})
			j += 2
		case "class":
			terms = append(terms, ir.FieldPredicate{Kind: "has_class", Class: t[j+1].Value})
			j += 2
		case "trait":
			terms = append(terms, ir.FieldPredicate{Kind: "has_trait", Trait: t[j+1].Value})
			j += 2
		case "form":
			terms = append(terms, ir.FieldPredicate{Kind: "has_form", Form: t[j+1].Value})
			j += 2
		case "life", "cost":
			predicate := ir.FieldPredicate{Kind: "compare", Field: t[j].Value, Op: compareOp(t[j+1].Value)}
			if t[j+2].Kind == syntax.Integer {
				predicate.Value = intToken(t[j+2])
				j += 3
			} else {
				predicate.ValueScalar = &ir.Scalar{Kind: "scalar", Side: t[j+2].Value, Field: t[j+4].Value}
				j += 5
			}
			terms = append(terms, predicate)
		}
		if j < end && t[j].Value == "and" {
			j++
		} else if j < end && t[j].Value == "or" {
			flush()
			j++
		}
	}
	flush()
	if len(groups) == 1 {
		return groups[0], end
	}
	return ir.OrPredicate{Kind: "or", Terms: groups}, end
}
func conditionIR(t []syntax.Token) ir.Condition {
	if counterRef(t, 0) {
		return ir.CompareCondition{Kind: "compare", Left: ir.Scalar{Kind: "self_counter", Field: t[4].Value}, Op: compareOp(t[5].Value), Right: intToken(t[6])}
	}
	for i, x := range t {
		if x.Value == "else" {
			t = t[:i]
			break
		}
	}
	if len(t) == 1 {
		return ir.OverflowCondition{Kind: "overflow", Side: "own"}
	}
	if len(t) == 3 {
		if t[0].Value == "self" {
			return ir.SelfFormCondition{Kind: "self_form", Form: t[2].Value}
		}
		if t[1].Value == "." {
			form := "evolved"
			if t[2].Value == "superevolve_unlocked" {
				form = "super_evolved"
			}
			return ir.EvolutionUnlockedCondition{Kind: "evolution_unlocked", Side: t[0].Value, Form: form}
		}
		return ir.CompareCondition{Kind: "compare", Left: ir.Scalar{Kind: "scalar", Side: "own", Field: "combo"}, Op: compareOp(t[1].Value), Right: intToken(t[2])}
	}
	if t[0].Value == "fused" {
		return ir.CompareCondition{Kind: "compare", Left: ir.Scalar{Kind: "fusion_material_scalar", Field: t[2].Value}, Op: compareOp(t[3].Value), Right: intToken(t[4])}
	}
	return ir.CompareCondition{Kind: "compare", Left: ir.Scalar{Kind: "scalar", Side: t[0].Value, Field: t[2].Value}, Op: compareOp(t[3].Value), Right: intToken(t[4])}
}
func eventPatternIR(t []syntax.Token) ir.Trigger {
	if t[1].Value == "self" {
		if t[2].Value == "discarded" {
			return ir.EventTrigger{Kind: "event", Event: "card_discarded", Side: "own", SelfOnly: true}
		}
		if t[2].Value == "summoned" {
			return ir.EventTrigger{Kind: "event", Event: "follower_summoned", Side: "own", SubjectType: "follower", SelfOnly: true}
		}
		return ir.EventTrigger{Kind: "event", Event: t[2].Value, Side: "own", SubjectType: "follower", SelfOnly: true}
	}
	m := ir.EventTrigger{Kind: "event", Side: t[1].Value}
	if t[2].Value == "turn" {
		m.Event = map[string]string{"starts": "turn_started", "ends": "turn_ended"}[t[3].Value]
	} else {
		m.SubjectType = t[2].Value
		if m.SubjectType == "card" {
			m.SubjectType = ""
		}
		m.Event = map[string]string{"summoned": "follower_summoned", "leaves": "follower_left", "destroyed": "destroyed", "healed": "healed", "fused": "card_fused", "engaged": "amulet_engaged", "discarded": "card_discarded"}[t[3].Value]
	}
	baseEnd, _, _ := parseBaseEventPattern(t)
	end, _, _ := parseEventPattern(t)
	if baseEnd < len(t) && t[baseEnd].Value == "while" {
		m.SourceZone = t[baseEnd+3].Value
		baseEnd += 4
	}
	if baseEnd < len(t) && t[baseEnd].Value == "once" {
		m.OncePerTurn = "any"
		if t[baseEnd+2].Value != "turn" {
			m.OncePerTurn = t[baseEnd+2].Value
		}
	}
	if len(t) > end && t[end].Value == "where" {
		m.Predicate, end = filterIR(t, end)
	}
	if len(t) > end && t[end].Value == "if" {
		m.Condition = conditionIR(t[end+1:])
	}
	return m
}
func compareOp(s string) string {
	return map[string]string{"==": "eq", "!=": "ne", "<": "lt", "<=": "le", ">": "gt", ">=": "ge"}[s]
}
func intAt(s *syntax.Statement, i int) int { return intToken(s.Tokens()[i]) }
func intToken(t syntax.Token) int          { n, _ := strconv.Atoi(t.Value); return n }
func signedAt(t []syntax.Token, i int) int {
	n := intToken(t[i+1])
	if t[i].Value == "-" {
		return -n
	}
	return n
}

func compileTestsTyped(l *Loaded) (ir.TestPack, error) {
	files := append([]*TestFile(nil), l.Tests...)
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	sources := []ir.Source{}
	scenarios := []ir.Scenario{}
	ids := map[string]bool{}
	for _, tf := range files {
		path := sourcePath(l, tf.Path)
		sid := sourceID(path)
		sources = append(sources, ir.Source{SourceID: sid, Path: path, ContentHash: contentHash(tf.File.Source), Span: spanIR(fileSpan(tf.File), sid)})
		for _, s := range tf.Scenarios {
			x, err := compileTypedScenario(s, sid, ids)
			if err != nil {
				return ir.TestPack{}, err
			}
			scenarios = append(scenarios, x)
		}
	}
	r := ruleset.DefaultDependency()
	dep := ir.RulesetDependency{
		ID: r.ID, ContentHash: r.ContentHash,
		RNG: ir.RNGPolicy{Algorithm: r.RNG.Algorithm, Version: r.RNG.Version},
		OrderingPolicy: ir.OrderingPolicy{
			Collections: r.OrderingPolicy.Collections, ReplacementAbilities: r.OrderingPolicy.ReplacementAbilities,
			SimultaneousTriggers: r.OrderingPolicy.SimultaneousTriggers, DeathBatchLastwords: r.OrderingPolicy.DeathBatchLastwords,
		},
		ExecutionBudget: ir.ExecutionBudgetPolicy{
			Instructions: r.ExecutionBudget.Instructions, QueryVisits: r.ExecutionBudget.QueryVisits,
			StackDepth: r.ExecutionBudget.StackDepth, Candidates: r.ExecutionBudget.Candidates,
			Events: r.ExecutionBudget.Events, Triggers: r.ExecutionBudget.Triggers,
			CreatedInstances: r.ExecutionBudget.CreatedInstances, ContinuationBytes: r.ExecutionBudget.ContinuationBytes,
		},
	}
	return ir.TestPack{Format: ir.Format, ContainerVersion: ir.ContainerVersion, Encoding: ir.Encoding, Kind: "test-pack", IRVersion: ir.IRVersion, RequiredFeatures: []string{}, UnresolvedReferences: []string{}, Ruleset: dep, Sources: sources, Scenarios: scenarios}, nil
}

func compileTypedScenario(s *syntax.Statement, sid string, ids map[string]bool) (ir.Scenario, error) {
	name := s.Tokens()[1].Value
	id := nodeID(sid, name)
	if ids[id] {
		return ir.Scenario{}, fmt.Errorf("WBO-E015-ID-COLLISION: 场景 ID 碰撞")
	}
	ids[id] = true
	b := s.Blocks()[0]
	seed, _ := strconv.ParseUint(b[0].Word(1), 10, 64)
	state, aliases := compileInitialState(b[1], id)
	actions, err := compileActions(b[2], aliases)
	if err != nil {
		return ir.Scenario{}, err
	}
	assertions, err := compileAssertions(b[3], aliases, sid)
	if err != nil {
		return ir.Scenario{}, err
	}
	return ir.Scenario{ID: id, Name: name, Seed: fmt.Sprintf("0x%016x", seed), InitialState: state, Actions: actions, Assertions: assertions, Origin: originIR(s.Span, sid)}, nil
}
func defaultPlayer() ir.PlayerState {
	return ir.PlayerState{Leader: ir.Leader{Life: 20, MaxLife: 20}, Zones: map[string][]ir.TestInstance{"deck": {}, "hand": {}, "field": {}, "graveyard": {}, "banished": {}, "destroyed": {}}}
}
func compileInitialState(s *syntax.Statement, scenarioID string) (ir.State, map[string]string) {
	players := map[string]ir.PlayerState{"own": defaultPlayer(), "oppo": defaultPlayer()}
	state := ir.State{Turn: ir.Turn{Active: "own", Number: 1}, Phase: "main", Players: players}
	aliases := map[string]string{}
	for _, x := range s.Blocks()[0] {
		t := x.Tokens()
		switch x.Word(0) {
		case "turn":
			state.Turn = ir.Turn{Active: t[1].Value, Number: intToken(t[2])}
		case "phase":
			state.Phase = t[1].Value
		case "player":
			p := players[t[1].Value]
			compilePlayerState(x, &p, aliases, scenarioID)
			players[t[1].Value] = p
		}
	}
	state.Aliases = aliases
	return state, aliases
}
func compilePlayerState(s *syntax.Statement, p *ir.PlayerState, a map[string]string, scenarioID string) {
	for _, x := range s.Blocks()[0] {
		t := x.Tokens()
		switch x.Word(0) {
		case "leader":
			p.Leader = ir.Leader{Life: intToken(t[1]), MaxLife: intToken(t[3])}
		case "pp":
			p.PP = intToken(t[1])
			p.MaxPP = intToken(t[3])
		case "ep":
			p.EP = intToken(t[1])
		case "sep":
			p.SEP = intToken(t[1])
		case "combo":
			p.Combo = intToken(t[1])
		case "shadows":
			p.Shadows = intToken(t[1])
		default:
			zone := x.Word(0)
			items := []ir.TestInstance{}
			for _, inst := range x.Blocks()[0] {
				it := inst.Tokens()
				alias := it[1].Value
				iid := nodeID(scenarioID, "instance", alias)
				a[alias] = iid
				over := ir.InstanceOverrides{}
				if len(inst.Blocks()) == 1 {
					for _, o := range inst.Blocks()[0] {
						ot := o.Tokens()
						switch o.Word(0) {
						case "counter":
							if over.Counters == nil {
								over.Counters = map[string]int{}
							}
							over.Counters[ot[1].Value] = intToken(ot[2])
						case "cost":
							v := intToken(ot[1])
							over.Cost = &v
						case "stats":
							over.Stats = &ir.Stats{Attack: intToken(ot[1]), Life: intToken(ot[3])}
						case "evolved":
							v := true
							over.Evolved = &v
						case "super_evolved":
							v := true
							over.SuperEvolved = &v
						case "earthsigil":
							v := intToken(ot[1])
							over.Earthsigil = &v
						case "countdown":
							v := intToken(ot[1])
							over.Countdown = &v
						case "engaged":
							v := ot[1].Value == "true"
							over.Engaged = &v
						default:
							over.Keywords = append(over.Keywords, o.Word(0))
						}
					}
				}
				items = append(items, ir.TestInstance{InstanceID: iid, Alias: alias, CardID: intToken(it[3]), DeclaredType: it[0].Value, Overrides: over})
			}
			p.Zones[zone] = items
		}
	}
}
func compileActions(s *syntax.Statement, a map[string]string) ([]ir.Action, error) {
	out := []ir.Action{}
	for _, x := range s.Blocks()[0] {
		t := x.Tokens()
		var action ir.Action
		switch x.Word(0) {
		case "play", "engage", "evolve", "superevolve":
			action = ir.SourceAction{Kind: x.Word(0), Actor: "own", Source: a[t[1].Value]}
		case "fuse":
			action = ir.FusionAction{Kind: "fusion", Actor: "own", Source: a[t[1].Value]}
		case "select":
			selection := ir.SelectAction{Kind: "select"}
			entities, leaders, _ := parseSelectionResponse(t)
			if len(entities) == 1 {
				selection.Target = a[entities[0].Value]
			} else {
				for _, entity := range entities {
					selection.Targets = append(selection.Targets, a[entity.Value])
				}
			}
			for _, leader := range leaders {
				selection.LeaderSides = append(selection.LeaderSides, leader.Value)
			}
			action = selection
		case "mode":
			action = ir.ModeAction{Kind: "select_mode", OptionID: intToken(t[1])}
		case "end_turn":
			action = ir.SourceAction{Kind: "end_turn", Actor: "own"}
		case "advance":
			action = ir.AdvanceAction{Kind: "advance", Timing: t[1].Value, Side: t[2].Value}
		case "attack":
			kind := "attack_leader"
			defender := t[3].Value
			if len(t) == 4 {
				kind = "attack_entity"
				defender = a[t[3].Value]
			}
			action = ir.AttackAction{Kind: kind, Actor: "own", Attacker: a[t[1].Value], Defender: defender}
		default:
			return nil, fmt.Errorf("WBT-E005-ACTION-ORDER: 无法编译动作 %q", x.Word(0))
		}
		out = append(out, action)
	}
	return out, nil
}
func compileAssertions(s *syntax.Statement, a map[string]string, sid string) ([]ir.Assertion, error) {
	out := []ir.Assertion{}
	for _, x := range s.Blocks()[0] {
		t := x.Tokens()
		h := x.Word(0)
		origin := originIR(x.Span, sid)
		var assertion ir.Assertion
		switch h {
		case "legal", "unchanged":
			assertion = ir.BasicAssertion{Kind: h, Origin: origin}
		case "illegal":
			assertion = ir.BasicAssertion{Kind: "illegal", Code: t[1].Value, Origin: origin}
		case "events":
			facts := []ir.EventMatcher{}
			for _, f := range x.Blocks()[0] {
				facts = append(facts, eventMatcherIR(f, a))
			}
			assertion = ir.EventsAssertion{Kind: "events", Mode: map[string]string{"contains": "contains_ordered", "excludes": "excludes", "exact": "exact"}[t[1].Value], Expected: facts, Origin: origin}
		case "all":
			src, end := setExprIR(t, 1)
			pred, end := filterIR(t, end)
			assertion = ir.KeywordAssertion{Kind: "all_have_keyword", Source: ir.FilterRef{Kind: "filter", Source: src, Predicate: pred}, Keyword: t[end+1].Value, Origin: origin}
		default:
			if len(t) == 3 && a[t[0].Value] != "" && set("has", "lacks")[t[1].Value] {
				assertion = ir.KeywordAssertion{Kind: "has_keyword", Target: a[t[0].Value], Keyword: t[2].Value, Expected: t[1].Value == "has", Origin: origin}
			} else if isCountTokens(t) {
				assertion = ir.ZoneAssertion{Kind: "zone_count", Side: t[0].Value, Zone: t[2].Value, CardID: intToken(t[5]), Op: "eq", Count: intToken(t[7]), Origin: origin}
			} else if containsToken(t, "[") {
				assertion = orderAssertionIR(t, a, origin)
			} else {
				eq := tokenIndex(t, "==")
				left := testRefIR(t[:eq], a)
				right := testLiteralIR(t[eq+1:])
				if left.Kind == "player_pp_pair" {
					right.Kind = "pp"
					right.Current = right.Attack
					right.Maximum = right.Life
					right.Attack = 0
					right.Life = 0
				}
				assertion = ir.CompareAssertion{Kind: "compare_value", Left: left, Op: "eq", Right: right, Origin: origin}
			}
		}
		out = append(out, assertion)
	}
	return out, nil
}
func eventMatcherIR(s *syntax.Statement, a map[string]string) ir.EventMatcher {
	t := s.Tokens()
	h := t[0].Value
	m := ir.EventMatcher{Kind: map[string]string{"damage": "damaged", "heal": "healed", "draw": "card_drawn", "destroy": "destroyed", "banish": "banished", "summon": "follower_summoned", "move": "zone_moved", "evolve": "evolved", "superevolve": "super_evolved", "engage": "amulet_engaged", "attack": "attacked", "turn_start": "turn_started", "turn_end": "turn_ended", "game_end": "game_ended", "gain": "resource_changed", "spend": "resource_changed", "return": "zone_moved"}[h]}
	if h == "discard" {
		m.Kind = "card_discarded"
	}
	switch h {
	case "damage", "heal":
		end, _ := factObject(t, 1, a, new([]syntax.Diagnostic))
		v := eventTarget(t[1:end], a)
		m.Target = &v
		m.Actual = intToken(t[end])
	case "draw":
		m.Side = t[1].Value
		m.Count = intToken(t[2])
	case "destroy", "banish", "discard":
		v := eventTarget(t[1:], a)
		m.Subject = &v
	case "summon":
		if t[1].Value == "card" {
			m.CardID = intToken(t[2])
			m.Count = intToken(t[4])
		} else {
			m.InstanceID = a[t[1].Value]
		}
	case "move":
		m.InstanceID = a[t[1].Value]
		m.To = t[3].Value
	case "evolve", "superevolve", "engage":
		m.InstanceID = a[t[1].Value]
	case "turn_start", "turn_end":
		m.Side = t[1].Value
	case "game_end":
		m.Side = t[1].Value
	case "gain", "spend":
		m.Side = t[1].Value
		m.Resource = t[3].Value
		m.Direction = h
		m.Amount = intToken(t[4])
	case "attack":
		attacker, defender := eventTarget(t[1:2], a), eventTarget(t[3:], a)
		m.Attacker, m.Defender = &attacker, &defender
	case "return":
		end, _ := factObject(t, 1, a, new([]syntax.Diagnostic))
		v := eventTarget(t[1:end], a)
		m.Reason, m.Subject, m.Destination = "return", &v, t[end+1].Value
	}
	return m
}
func eventTarget(t []syntax.Token, a map[string]string) ir.EventTarget {
	if len(t) == 1 {
		return ir.EventTarget{Kind: "instance", InstanceID: a[t[0].Value]}
	}
	if t[0].Value == "card" {
		return ir.EventTarget{Kind: "card", CardID: intToken(t[1])}
	}
	return ir.EventTarget{Kind: "leader", Side: t[0].Value}
}
func testRefIR(t []syntax.Token, a map[string]string) ir.TestRef {
	if values(t) == "rng . consumed" {
		return ir.TestRef{Kind: "rng_consumed"}
	}
	if a[t[0].Value] != "" {
		if len(t) == 5 && t[2].Value == "counter" {
			return ir.TestRef{Kind: "instance_counter", InstanceID: a[t[0].Value], Field: t[4].Value}
		}
		return ir.TestRef{Kind: "instance_field", InstanceID: a[t[0].Value], Field: t[2].Value}
	}
	field := t[len(t)-1].Value
	if len(t) == 5 {
		field = "leader." + field
	}
	if field == "pp" && len(t) == 3 {
		return ir.TestRef{Kind: "player_pp_pair", Side: t[0].Value}
	}
	return ir.TestRef{Kind: "player_field", Side: t[0].Value, Field: field}
}
func testLiteralIR(t []syntax.Token) ir.Literal {
	if len(t) == 3 {
		return ir.Literal{Kind: "stats", Attack: intToken(t[0]), Life: intToken(t[2])}
	}
	if set("true", "false")[t[0].Value] {
		return ir.Literal{Kind: "boolean", Boolean: t[0].Value == "true"}
	}
	if isUnsigned(t[0]) {
		return ir.Literal{Kind: "integer", Integer: intToken(t[0])}
	}
	return ir.Literal{Kind: "zone", Value: t[0].Value}
}
func orderAssertionIR(t []syntax.Token, a map[string]string, origin ir.Origin) ir.Assertion {
	start := tokenIndex(t, "[")
	end, _ := aliasList(t, start, a, new([]syntax.Diagnostic))
	expected := []string{}
	for i := start + 1; i < end-1; i += 2 {
		expected = append(expected, a[t[i].Value])
	}
	zone := t[2].Value
	containment := "exact"
	if t[len(t)-1].Value == "ordered" {
		containment = "subsequence"
	}
	return ir.ZoneAssertion{Kind: "ordered_instances", Source: ir.ZoneRef{Kind: "zone", Side: t[0].Value, Zone: zone}, Expected: expected, Containment: containment, Origin: origin}
}
func containsToken(t []syntax.Token, s string) bool { return tokenIndex(t, s) >= 0 }
func tokenIndex(t []syntax.Token, s string) int {
	for i, x := range t {
		if x.Value == s {
			return i
		}
	}
	return -1
}
func isCountTokens(t []syntax.Token) bool { return len(t) == 8 && t[3].Value == "count" }
