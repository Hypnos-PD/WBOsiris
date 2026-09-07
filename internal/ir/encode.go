package ir

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

func (v Literal) MarshalJSON() ([]byte, error) {
	switch v.Kind {
	case "integer":
		return json.Marshal(struct {
			Kind  string `json:"kind"`
			Value int    `json:"value"`
		}{v.Kind, v.Integer})
	case "boolean":
		return json.Marshal(struct {
			Kind  string `json:"kind"`
			Value bool   `json:"value"`
		}{v.Kind, v.Boolean})
	case "zone":
		return json.Marshal(struct {
			Kind  string `json:"kind"`
			Value string `json:"value"`
		}{v.Kind, v.Value})
	case "stats":
		return json.Marshal(struct {
			Kind   string `json:"kind"`
			Attack int    `json:"attack"`
			Life   int    `json:"life"`
		}{v.Kind, v.Attack, v.Life})
	case "pp":
		return json.Marshal(struct {
			Kind    string `json:"kind"`
			Current int    `json:"current"`
			Maximum int    `json:"maximum"`
		}{v.Kind, v.Current, v.Maximum})
	default:
		return nil, fmt.Errorf("encode unknown literal kind %q", v.Kind)
	}
}

func (m EventMatcher) MarshalJSON() ([]byte, error) {
	object := map[string]any{"kind": m.Kind}
	switch m.Kind {
	case "healed", "damaged":
		object["target"], object["actual"] = m.Target, m.Actual
	case "destroyed", "banished", "card_discarded":
		object["subject"] = m.Subject
	case "card_drawn":
		object["side"], object["count"] = m.Side, m.Count
	case "follower_summoned":
		if m.InstanceID != "" {
			object["instanceId"] = m.InstanceID
		} else {
			object["cardId"], object["count"] = m.CardID, m.Count
		}
		if m.Side != "" {
			object["side"] = m.Side
		}
	case "zone_moved":
		if m.Reason == "return" {
			object["reason"], object["subject"], object["destination"] = m.Reason, m.Subject, m.Destination
		} else {
			object["instanceId"], object["to"] = m.InstanceID, m.To
			if m.From != "" {
				object["from"] = m.From
			}
		}
	case "evolved", "super_evolved", "amulet_engaged":
		object["instanceId"] = m.InstanceID
	case "attacked":
		object["attacker"], object["defender"] = m.Attacker, m.Defender
	case "turn_started", "turn_ended", "game_ended":
		object["side"] = m.Side
	case "resource_changed":
		object["side"], object["resource"], object["direction"], object["amount"] = m.Side, m.Resource, m.Direction, m.Amount
	default:
		return nil, fmt.Errorf("encode unknown event matcher kind %q", m.Kind)
	}
	return json.Marshal(object)
}

func (p FieldPredicate) MarshalJSON() ([]byte, error) {
	object := map[string]any{"kind": p.Kind}
	switch p.Kind {
	case "has_spellboost":
	case "has_card":
		object["cardId"] = p.CardID
	case "has_type":
		object["cardType"] = p.CardType
	case "has_class":
		object["class"] = p.Class
	case "has_trait":
		object["trait"] = p.Trait
	case "has_form":
		object["form"] = p.Form
	case "compare":
		object["field"], object["op"], object["value"] = p.Field, p.Op, p.Value
		if p.ValueScalar != nil {
			if p.Value != 0 || p.ValueScalar.Kind != "scalar" || !validNumericExpr(p.ValueScalar, false) {
				return nil, fmt.Errorf("invalid predicate scalar or conflicting literal")
			}
			object["value"] = p.ValueScalar
		}
	default:
		return nil, fmt.Errorf("encode unknown field predicate kind %q", p.Kind)
	}
	return json.Marshal(object)
}

func (a SourceAction) MarshalJSON() ([]byte, error) {
	object := map[string]any{"kind": a.Kind, "actor": a.Actor}
	if a.Kind != "end_turn" {
		object["source"] = a.Source
	}
	return json.Marshal(object)
}

func (a FusionAction) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{"kind": a.Kind, "actor": a.Actor, "source": a.Source})
}

func effectObject(base NodeBase, kind string) map[string]any {
	return map[string]any{"id": base.ID, "kind": kind, "origin": base.Origin}
}

func (e DrawEffect) MarshalJSON() ([]byte, error) {
	object := effectObject(e.NodeBase, e.Kind)
	object["owner"], object["sourceZone"], object["all"], object["output"] = e.Owner, e.SourceZone, e.All, e.Output
	if !e.All {
		object["count"] = e.Count
	}
	if e.Predicate != nil {
		object["predicate"] = e.Predicate
	}
	return json.Marshal(object)
}

func (e CardEffect) MarshalJSON() ([]byte, error) {
	object := effectObject(e.NodeBase, e.Kind)
	switch e.Kind {
	case "gain_crest":
		object["owner"], object["cardId"] = e.Owner, e.CardID
	case "add_card":
		object["owner"], object["count"], object["cardId"], object["destination"] = e.Owner, e.Count, e.CardID, e.Destination
	case "summon":
		object["owner"], object["count"], object["cardId"], object["output"] = e.Owner, e.Count, e.CardID, e.Output
	case "summon_copies":
		object["owner"], object["target"], object["output"] = e.Owner, e.Target, e.Output
	case "reanimate":
		object["owner"], object["maxCost"], object["tieBreak"], object["output"] = e.Owner, e.MaxCost, e.TieBreak, e.Output
	case "transform":
		object["target"], object["cardId"] = e.Target, e.CardID
		object["preserveInstanceId"], object["preserveMaterials"] = e.PreserveInstanceID, e.PreserveMaterials
	default:
		return nil, fmt.Errorf("encode unknown card effect kind %q", e.Kind)
	}
	return json.Marshal(object)
}

func (e TargetEffect) MarshalJSON() ([]byte, error) {
	if e.Until != "" && (!oneOf(e.Kind, "add_keyword", "buff_stats") || !oneOf(e.Until, "turn_end", "own_turn_end", "oppo_turn_end")) {
		return nil, fmt.Errorf("invalid effect duration")
	}
	object := effectObject(e.NodeBase, e.Kind)
	object["target"] = e.Target
	if e.Until != "" {
		object["until"] = e.Until
	}
	if e.Output != "" {
		if e.Kind != "destroy" || e.Output != "destroyed" {
			return nil, fmt.Errorf("invalid target effect output")
		}
		object["output"] = e.Output
	}
	if !validDamageDistribution(e.Kind, e.Distribution, e.Target, e.Overflow) {
		return nil, fmt.Errorf("invalid damage distribution")
	}
	if e.Distribution != "" {
		object["distribution"] = e.Distribution
		if e.Overflow != nil {
			object["overflow"] = e.Overflow
		}
	}
	if e.AmountExpr != nil && e.Kind != "damage" && e.Kind != "heal" && e.Kind != "set_life" ||
		(e.AttackExpr != nil || e.LifeExpr != nil) && e.Kind != "buff_stats" {
		return nil, fmt.Errorf("numeric expression is not supported for this effect")
	}
	amount, err := numericValue(e.Amount, e.AmountExpr, false)
	if err != nil {
		return nil, err
	}
	switch e.Kind {
	case "damage":
		object["amount"], object["damageType"] = amount, e.DamageType
		if e.Predicate != nil {
			object["predicate"] = e.Predicate
		}
	case "heal":
		object["amount"] = amount
		if e.Predicate != nil {
			object["predicate"] = e.Predicate
		}
	case "buff_stats":
		attack, err := numericValue(e.AttackDelta, e.AttackExpr, true)
		if err != nil {
			return nil, err
		}
		life, err := numericValue(e.LifeDelta, e.LifeExpr, true)
		if err != nil {
			return nil, err
		}
		object["attackDelta"], object["lifeDelta"] = attack, life
		if e.Predicate != nil {
			object["predicate"] = e.Predicate
		}
	case "destroy", "banish", "discard":
		if e.Predicate != nil {
			object["predicate"] = e.Predicate
		}
	case "return":
		object["destination"] = e.Destination
		if e.DeckInsertion != "" {
			object["deckInsertion"] = e.DeckInsertion
		}
	case "add_keyword", "remove_keyword":
		object["keyword"] = e.Keyword
		if e.Predicate != nil {
			object["predicate"] = e.Predicate
		}
	case "set_attack_limit", "set_life":
		object["amount"] = e.Amount
		if e.Kind == "set_life" {
			object["amount"] = amount
		}
	case "silent_evolve":
		object["form"] = e.Form
	default:
		return nil, fmt.Errorf("encode unknown target effect kind %q", e.Kind)
	}
	return json.Marshal(object)
}

func (e AdjustEffect) MarshalJSON() ([]byte, error) {
	object := effectObject(e.NodeBase, e.Kind)
	switch e.Kind {
	case "adjust_resource":
		object["owner"], object["resource"], object["delta"] = e.Owner, e.Resource, e.Delta
	case "restore_resource":
		object["owner"], object["resource"] = e.Owner, e.Resource
	case "adjust_earthsigil":
		object["owner"], object["delta"] = e.Owner, e.Delta
	case "adjust_counter":
		object["field"], object["delta"] = e.Field, e.Delta
	case "adjust_entity_field":
		object["field"], object["target"], object["delta"], object["minimum"] = e.Field, e.Target, e.Delta, e.Minimum
	case "spellboost":
		object["target"], object["times"] = e.Target, e.Times
	default:
		return nil, fmt.Errorf("encode unknown adjustment effect kind %q", e.Kind)
	}
	return json.Marshal(object)
}

func (a KeywordAssertion) MarshalJSON() ([]byte, error) {
	object := map[string]any{"kind": a.Kind, "keyword": a.Keyword, "origin": a.Origin}
	switch a.Kind {
	case "has_keyword":
		object["target"], object["expected"] = a.Target, a.Expected
	case "all_have_keyword":
		object["source"] = a.Source
	default:
		return nil, fmt.Errorf("encode unknown keyword assertion kind %q", a.Kind)
	}
	return json.Marshal(object)
}

func (a ZoneAssertion) MarshalJSON() ([]byte, error) {
	object := map[string]any{"kind": a.Kind, "origin": a.Origin}
	switch a.Kind {
	case "zone_count":
		object["side"], object["zone"], object["cardId"], object["op"], object["count"] = a.Side, a.Zone, a.CardID, a.Op, a.Count
	case "ordered_instances":
		object["source"], object["expected"], object["containment"] = a.Source, a.Expected, a.Containment
	case "rng_consumed":
		object["op"], object["count"] = a.Op, a.Count
	default:
		return nil, fmt.Errorf("encode unknown zone assertion kind %q", a.Kind)
	}
	return json.Marshal(object)
}

// EncodeCardPack produces canonical WBOS JSON and derives the content hash
// from the same typed package with an empty contentHash field.
func EncodeCardPack(pack CardPack) ([]byte, error) {
	pack.ContentHash = ""
	return encodePackage(pack)
}

// EncodeTestPack produces canonical WBOS JSON and derives the content hash
// from the same typed package with an empty contentHash field.
func EncodeTestPack(pack TestPack) ([]byte, error) {
	pack.ContentHash = ""
	return encodePackage(pack)
}

func encodePackage(pack any) ([]byte, error) {
	b, err := json.Marshal(pack)
	if err != nil {
		return nil, fmt.Errorf("encode WBOS package: %w", err)
	}
	var object map[string]any
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	if err := d.Decode(&object); err != nil {
		return nil, fmt.Errorf("normalize WBOS package: %w", err)
	}
	canonical, err := json.Marshal(object)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(canonical)
	object["contentHash"] = "sha256:" + hex.EncodeToString(sum[:])
	out, err := json.MarshalIndent(object, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}
