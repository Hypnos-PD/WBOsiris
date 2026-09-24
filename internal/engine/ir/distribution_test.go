package ir

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestDistributedDamageRoundTrip(t *testing.T) {
	for _, overflow := range []Ref{nil, LeaderRef{Kind: "leader", Side: "oppo", ValueType: "leader"}} {
		for _, count := range []NumericExpr{nil, &CountExpr{Kind: "count", Source: ZoneRef{Kind: "zone", Side: "own", Zone: "hand"}}} {
			effect := TargetEffect{NodeBase: NodeBase{ID: strings.Repeat("b", 32), Origin: testOrigin()},
				Kind: "damage", DamageType: "effect", Distribution: "field_entry_order", Overflow: overflow,
				Target:     ZoneRef{Kind: "zone", Side: "oppo", Zone: "field", Member: "follower"},
				AmountExpr: count, Predicate: FieldPredicate{Kind: "has_trait", Trait: "officer"},
			}
			if count == nil {
				effect.Amount = 7
			}
			data, err := json.Marshal(effect)
			if err != nil {
				t.Fatal(err)
			}
			decoded, err := decodeEffect(data, map[string]bool{})
			if err != nil || !reflect.DeepEqual(effect, decoded) {
				t.Fatalf("distribution changed on round trip: %#v %v", decoded, err)
			}
		}
	}
}

func TestRejectInvalidIRDamageDistribution(t *testing.T) {
	base := TargetEffect{NodeBase: NodeBase{ID: strings.Repeat("b", 32), Origin: testOrigin()},
		Kind: "damage", DamageType: "effect", Distribution: "field_entry_order", Amount: 7,
		Target: ZoneRef{Kind: "zone", Side: "oppo", Zone: "field", Member: "follower"},
	}
	data, err := json.Marshal(base)
	if err != nil {
		t.Fatal(err)
	}
	for name, changes := range map[string]map[string]any{
		"random_order":           {"distribution": "random"},
		"heal":                   {"kind": "heal", "damageType": ""},
		"both_fields":            {"target": ZoneRef{Kind: "zone", Zone: "field", Member: "follower"}},
		"hand":                   {"target": ZoneRef{Kind: "zone", Side: "oppo", Zone: "hand", Member: "follower"}},
		"amulets":                {"target": ZoneRef{Kind: "zone", Side: "oppo", Zone: "field", Member: "amulet"}},
		"binding":                {"target": BindingRef{Kind: "binding", Name: "target"}},
		"opposite_overflow":      {"overflow": LeaderRef{Kind: "leader", Side: "own", ValueType: "leader"}},
		"entity_overflow":        {"overflow": SelfRef{Kind: "self", ValueType: "entity"}},
		"undistributed_overflow": {"distribution": "", "overflow": LeaderRef{Kind: "leader", Side: "oppo", ValueType: "leader"}},
		"null_overflow":          {"overflow": nil},
	} {
		t.Run(name, func(t *testing.T) {
			var object map[string]any
			if err := json.Unmarshal(data, &object); err != nil {
				t.Fatal(err)
			}
			for key, value := range changes {
				object[key] = value
			}
			mutated, err := json.Marshal(object)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := decodeEffect(mutated, map[string]bool{}); err == nil {
				t.Fatal("accepted invalid distribution")
			}
		})
	}
	base.Overflow = LeaderRef{Kind: "leader", Side: "own", ValueType: "leader"}
	if _, err := json.Marshal(base); err == nil {
		t.Fatal("encoded cross-side overflow")
	}
}
