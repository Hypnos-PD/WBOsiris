package ir

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestNumericExpressionsRoundTrip(t *testing.T) {
	for _, expr := range []NumericExpr{
		&Scalar{Kind: "scalar", Side: "own", Field: "combo"},
		&Scalar{Kind: "scalar", Side: "oppo", Field: "shadows"},
		&Scalar{Kind: "self_scalar", Field: "attack"},
		&CountExpr{Kind: "count", Source: ZoneRef{Kind: "zone", Side: "own", Zone: "hand", Member: "follower"}},
	} {
		for _, kind := range []string{"damage", "heal", "buff_stats"} {
			e := TargetEffect{NodeBase: NodeBase{ID: strings.Repeat("b", 32), Origin: testOrigin()}, Kind: kind, Target: SelfRef{Kind: "self"}}
			if kind == "buff_stats" {
				e.AttackExpr, e.LifeExpr = expr, &NegateExpr{Kind: "negate", Value: expr}
			} else {
				e.AmountExpr = expr
			}
			if kind == "damage" {
				e.DamageType = "effect"
			}
			data, err := json.Marshal(e)
			if err != nil {
				t.Fatal(err)
			}
			decoded, err := decodeEffect(data, map[string]bool{})
			if err != nil || !reflect.DeepEqual(e, decoded) {
				t.Fatalf("roundtrip: %s got=%#v err=%v", data, decoded, err)
			}
		}
	}
}

func TestNumericExpressionsRejectInvalidShapes(t *testing.T) {
	for _, value := range []string{
		`{"kind":"scalar","side":"all","field":"combo"}`,
		`{"kind":"scalar","field":"pp"}`,
		`{"kind":"scalar","side":"own","field":"unknown"}`,
		`{"kind":"self_scalar","side":"own","field":"attack"}`,
		`{"kind":"self_scalar","field":"combo"}`,
		`{"kind":"self_scalar","field":"attack","source":null}`,
		`{"kind":"negate","value":2}`,
		`{"kind":"negate","value":{"kind":"negate","value":{"kind":"self_scalar","field":"attack"}}}`,
	} {
		if _, _, err := decodeNumericValue(json.RawMessage(value), true); err == nil {
			t.Fatalf("accepted %s", value)
		}
	}
	if _, _, err := decodeEffectAmount(json.RawMessage(`{"kind":"negate","value":{"kind":"self_scalar","field":"attack"}}`)); err == nil {
		t.Fatal("damage accepted explicit negation")
	}
	base := TargetEffect{NodeBase: NodeBase{ID: strings.Repeat("b", 32), Origin: testOrigin()}, Kind: "buff_stats", Target: SelfRef{Kind: "self"}, AttackExpr: &Scalar{Kind: "scalar", Side: "own", Field: "combo"}}
	base.AttackDelta = 1
	if _, err := json.Marshal(base); err == nil {
		t.Fatal("conflicting literal and expression")
	}
	base.AttackDelta, base.Kind = 0, "destroy"
	if _, err := json.Marshal(base); err == nil {
		t.Fatal("expression on removal")
	}
}

func TestNumericBuffReferencesAreValidated(t *testing.T) {
	for _, negative := range []bool{false, true} {
		var expr NumericExpr = &CountExpr{Kind: "count", Source: FilterRef{Kind: "filter", Source: ZoneRef{Kind: "zone", Side: "own", Zone: "hand"}, Predicate: FieldPredicate{Kind: "has_card", CardID: 23456789}}}
		if negative {
			expr = &NegateExpr{Kind: "negate", Value: expr}
		}
		card := Card{ID: 12345678, CardType: "follower", PlayEffects: []Effect{TargetEffect{Kind: "buff_stats", AttackExpr: expr}}}
		if err := validateCardRefs(card, map[int]bool{12345678: true}, nil); err == nil {
			t.Fatal("buff count accepted unknown card")
		}
	}
	card := Card{ID: 12345678, CardType: "spell", PlayEffects: []Effect{TargetEffect{Kind: "damage", AmountExpr: &Scalar{Kind: "self_scalar", Field: "attack"}}}}
	if err := validateCardRefs(card, map[int]bool{12345678: true}, nil); err == nil {
		t.Fatal("spell read follower stats")
	}
}
