package ir

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestDamageReductionIRContract(t *testing.T) {
	e := TargetEffect{NodeBase: NodeBase{ID: strings.Repeat("a", 32), Origin: testOrigin()}, Kind: "set_damage_reduction", Target: SelfRef{Kind: "self", ValueType: "entity"}}
	for _, amount := range []int{0, 3, 65535} {
		e.Amount = amount
		data, err := json.Marshal(e)
		if err != nil {
			t.Fatal(err)
		}
		got, err := decodeEffect(data, map[string]bool{})
		if err != nil || !reflect.DeepEqual(e, got) {
			t.Fatal("valid effect failed round trip", got, err)
		}
	}
	e.Amount = 3
	data, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, field, value string }{
		{"missing amount", "amount", ""}, {"null", "amount", "null"}, {"negative", "amount", "-1"},
		{"overflow", "amount", "65536"}, {"fraction", "amount", "1.5"}, {"string", "amount", `"3"`},
		{"leader", "target", `{"kind":"leader","side":"own"}`},
		{"leaders", "target", `{"kind":"leaders","valueType":"leaders"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var raw map[string]json.RawMessage
			if err := json.Unmarshal(data, &raw); err != nil {
				t.Fatal(err)
			}
			if tc.value == "" {
				delete(raw, tc.field)
			} else {
				raw[tc.field] = json.RawMessage(tc.value)
			}
			bad, err := json.Marshal(raw)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := decodeEffect(bad, map[string]bool{}); err == nil {
				t.Fatal("accepted malformed reduction effect")
			}
		})
	}
	for _, amount := range []int{-1, 65536} {
		e.Amount = amount
		if _, err := json.Marshal(e); err == nil {
			t.Fatal("encoded invalid amount", amount)
		}
	}
}
