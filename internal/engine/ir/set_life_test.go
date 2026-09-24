package ir

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestSetLifeRoundTrip(t *testing.T) {
	for _, expr := range []NumericExpr{nil, &CountExpr{Kind: "count", Source: ZoneRef{Kind: "zone", Side: "own", Zone: "hand"}}} {
		e := TargetEffect{NodeBase: NodeBase{ID: strings.Repeat("a", 32), Origin: testOrigin()}, Kind: "set_life", Target: BindingRef{Kind: "binding", Name: "target"}, AmountExpr: expr}
		data, err := json.Marshal(e)
		if err != nil {
			t.Fatal(err)
		}
		got, err := decodeEffect(data, map[string]bool{})
		if err != nil || !reflect.DeepEqual(e, got) {
			t.Fatal(err, got)
		}
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(data, &raw); err != nil {
			t.Fatal(err)
		}
		for _, badTarget := range []string{`{"kind":"leader","side":"own"}`, `{"kind":"leaders","valueType":"leaders"}`} {
			raw["target"] = json.RawMessage(badTarget)
			bad, _ := json.Marshal(raw)
			if _, err := decodeEffect(bad, map[string]bool{}); err == nil {
				t.Fatal("accepted leader life setter")
			}
		}
	}
}

func TestSetLifeRejectsInvalidAmount(t *testing.T) {
	e := TargetEffect{NodeBase: NodeBase{ID: strings.Repeat("a", 32), Origin: testOrigin()}, Kind: "set_life", Target: BindingRef{Kind: "binding", Name: "target"}}
	data, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	for _, amount := range []string{"", "null", "-1", `"1"`} {
		t.Run("amount="+amount, func(t *testing.T) {
			var raw map[string]json.RawMessage
			if err := json.Unmarshal(data, &raw); err != nil {
				t.Fatal(err)
			}
			if amount == "" {
				delete(raw, "amount")
			} else {
				raw["amount"] = json.RawMessage(amount)
			}
			bad, err := json.Marshal(raw)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := decodeEffect(bad, map[string]bool{}); err == nil {
				t.Fatal("accepted invalid set life amount")
			}
		})
	}
}
