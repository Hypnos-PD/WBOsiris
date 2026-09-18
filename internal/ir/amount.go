package ir

import (
	"encoding/json"
	"fmt"
)

func ValidPlayerScalar(field string) bool {
	return oneOf(field, "combo", "pp", "maxpp", "life", "ep", "sep", "shadows", "hand_count", "earthsigils", "entered_artifacts", "rally", "crests", "evolutions")
}

func validCountSource(source Ref) bool {
	switch r := source.(type) {
	case BindingRef:
		return r.Kind == "binding" && r.Name != ""
	case HistoryRef:
		return validHistoryRef(r)
	case ZoneRef:
		return r.Kind == "zone" && validZone(r.Zone) && (validSide(r.Side) || r.Side == "" && r.Zone == "field") &&
		(r.Member == "" || oneOf(r.Member, "card", "follower", "spell", "amulet"))
	case ExcludeRef:
		// 被排除的对象可以是来源自身，也可以是某个绑定；两者都不是"集合"。
		if r.Kind != "exclude" || !validCountSource(r.Source) {
			return false
		}
		switch v := r.Value.(type) {
		case nil:
			return true
		case SelfRef:
			return true
		case BindingRef:
			return v.Kind == "binding" && v.Name != ""
		default:
			return false
		}
	case FilterRef:
		// `count(集合 other where …)` 先排除来源实例再筛选，因此这里允许
		// 被筛选的对象是 ExcludeRef；两层筛选不合法。
		if _, nested := r.Source.(FilterRef); nested {
			return false
		}
		return r.Kind == "filter" && validCountSource(r.Source) && r.Predicate != nil
	default:
		return false
	}
}

func validNumericExpr(expr NumericExpr, signed bool) bool {
	switch e := expr.(type) {
	case *CountExpr:
		return e != nil && e.Kind == "count" && validCountSource(e.Source)
	case *SumExpr:
		return e != nil && e.Kind == "sum" && validCountSource(e.Source) && oneOf(e.Field, "base_attack", "base_life", "base_cost", "attack", "life", "cost") &&
			(e.Limit == 0 && e.Direction == "" || e.Limit > 0 && e.Limit <= 65535 && oneOf(e.Direction, "highest", "lowest"))
	case *Scalar:
		return e != nil && (e.Kind == "scalar" && validSide(e.Side) && ValidPlayerScalar(e.Field) ||
			e.Kind == "self_scalar" && e.Side == "" && oneOf(e.Field, "attack", "life", "cost", "damage_taken") ||
			e.Kind == "binding_scalar" && ValidBindingName(e.Side) && oneOf(e.Field, "attack", "life", "cost", "base_attack", "base_life", "base_cost") ||
			e.Kind == "self_counter" && e.Side == "" && ValidCounterName(e.Field) ||
			e.Kind == "fusion_material_scalar" && e.Side == "" && oneOf(e.Field, "cost", "distinct"))
	case *NegateExpr:
		return signed && e != nil && e.Kind == "negate" && validNumericExpr(e.Value, false)
	case *DifferenceExpr:
		return e != nil && e.Kind == "difference" && validNumericExpr(e.Left, false) && validNumericExpr(e.Right, false)
	default:
		return false
	}
}

func decodeEffectAmount(data json.RawMessage) (int, NumericExpr, error) {
	return decodeNumericValue(data, false)
}

func decodeNumericValue(data json.RawMessage, signed bool) (int, NumericExpr, error) {
	var literal *int
	if json.Unmarshal(data, &literal) == nil && literal != nil {
		if !signed && *literal < 0 {
			return 0, nil, fmt.Errorf("effect amount must be nonnegative")
		}
		return *literal, nil, nil
	}
	var raw struct {
		Kind   string          `json:"kind"`
		Source json.RawMessage `json:"source"`
		Side   string          `json:"side"`
		Field  string          `json:"field"`
		Value  json.RawMessage `json:"value"`
		Left   json.RawMessage `json:"left"`
		Right  json.RawMessage `json:"right"`
		Limit  int             `json:"limit"`
		Direction string       `json:"direction"`
	}
	if err := strict(data, &raw); err != nil {
		return 0, nil, err
	}
	var expr NumericExpr
	switch raw.Kind {
	case "count", "sum":
		if raw.Side != "" || raw.Kind == "count" && raw.Field != "" || len(raw.Value) > 0 {
			return 0, nil, fmt.Errorf("invalid count fields")
		}
		source, err := decodeRef(raw.Source)
		if err != nil {
			return 0, nil, err
		}
		expr = &CountExpr{Kind: "count", Source: source}
		if raw.Kind == "sum" {
			expr = &SumExpr{Kind: "sum", Source: source, Field: raw.Field, Limit: raw.Limit, Direction: raw.Direction}
		} else if raw.Limit != 0 || raw.Direction != "" {
			return 0, nil, fmt.Errorf("invalid count fields")
		}
	case "scalar", "self_scalar", "binding_scalar", "self_counter", "fusion_material_scalar":
		if len(raw.Source) > 0 || len(raw.Value) > 0 {
			return 0, nil, fmt.Errorf("invalid scalar fields")
		}
		expr = &Scalar{Kind: raw.Kind, Side: raw.Side, Field: raw.Field}
	case "negate":
		if !signed || raw.Side != "" || raw.Field != "" || len(raw.Source) > 0 {
			return 0, nil, fmt.Errorf("invalid negation fields")
		}
		_, value, err := decodeNumericValue(raw.Value, false)
		if err != nil {
			return 0, nil, err
		}
		expr = &NegateExpr{Kind: "negate", Value: value}
	case "difference":
		if raw.Side != "" || raw.Field != "" || len(raw.Source) > 0 || len(raw.Value) > 0 {
			return 0, nil, fmt.Errorf("invalid difference fields")
		}
		_, left, err := decodeNumericValue(raw.Left, false)
		if err != nil {
			return 0, nil, err
		}
		_, right, err := decodeNumericValue(raw.Right, false)
		if err != nil {
			return 0, nil, err
		}
		if left == nil || right == nil {
			return 0, nil, fmt.Errorf("difference operands must be numeric expressions")
		}
		expr = &DifferenceExpr{Kind: "difference", Left: left, Right: right}
	default:
		return 0, nil, fmt.Errorf("invalid effect amount kind %q", raw.Kind)
	}
	if !validNumericExpr(expr, signed) {
		return 0, nil, fmt.Errorf("invalid numeric expression")
	}
	return 0, expr, nil
}

func numericValue(literal int, expr NumericExpr, signed bool) (any, error) {
	if expr == nil {
		if !signed && literal < 0 {
			return nil, fmt.Errorf("effect amount must be nonnegative")
		}
		return literal, nil
	}
	if literal != 0 || !validNumericExpr(expr, signed) {
		return nil, fmt.Errorf("invalid numeric expression or conflicting literal")
	}
	return expr, nil
}

func numericCardRefs(expr NumericExpr, cards map[int]bool, cardType string) error {
	switch e := expr.(type) {
	case *CountExpr:
		if source, ok := e.Source.(FilterRef); ok {
			return validatePredicateCardRefs(source.Predicate, cards)
		}
	case *SumExpr:
		if source, ok := e.Source.(FilterRef); ok {
			return validatePredicateCardRefs(source.Predicate, cards)
		}
	case *NegateExpr:
		return numericCardRefs(e.Value, cards, cardType)
	case *Scalar:
		if e.Kind == "self_scalar" && e.Field != "cost" && cardType != "follower" {
			return fmt.Errorf("self.%s requires a follower", e.Field)
		}
	}
	return nil
}
