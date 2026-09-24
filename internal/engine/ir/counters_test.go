package ir

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestCounterAdjustmentRoundTrip(t *testing.T) {
	for _, delta := range []int{0, 1, MaxCounterValue, -1, MaxCounterValue + 1} {
		want := AdjustEffect{NodeBase: NodeBase{ID: strings.Repeat("a", 32), Origin: testOrigin()}, Kind: "adjust_counter", Field: "x", Delta: delta}
		data, err := json.Marshal(want)
		if err != nil {
			t.Fatal(err)
		}
		got, err := decodeEffect(data, map[string]bool{})
		if delta < 0 || delta > MaxCounterValue {
			if err == nil {
				t.Fatal("accepted out-of-range delta")
			}
			continue
		}
		if err != nil || !reflect.DeepEqual(want, got) {
			t.Fatal("counter adjustment lost", string(data), err)
		}
	}
}

func TestCounterReferencesCheckedInsideNestedEffects(t *testing.T) {
	for _, body := range [][]Effect{
		{AdjustEffect{Kind: "adjust_counter", Field: "missing", Delta: 1}},
		{TargetEffect{Kind: "damage", AmountExpr: &Scalar{Kind: "self_counter", Field: "missing"}}},
		{TargetEffect{Kind: "buff_stats", LifeExpr: &NegateExpr{Kind: "negate", Value: &Scalar{Kind: "self_counter", Field: "missing"}}}},
		{RepeatEffect{Kind: "repeat", TimesExpr: &Scalar{Kind: "self_counter", Field: "missing"}}},
		{IfEffect{Kind: "if", Condition: CompareCondition{Kind: "compare", Left: Scalar{Kind: "self_counter", Field: "missing"}}}},
	} {
		card := Card{Counters: map[string]int{"x": 0}, PlayEffects: []Effect{ModeEffect{Kind: "mode", Options: []ModeOption{{ID: 1, Body: []Effect{RepeatEffect{Kind: "repeat", Times: 1, Body: body}}}}}}}
		if err := validateCounterRefs(card); err == nil {
			t.Fatal("accepted nested undeclared reference")
		}
		card.Counters["missing"] = 2
		if err := validateCounterRefs(card); err != nil {
			t.Fatal(err)
		}
	}
	for _, counters := range []map[string]int{{"X": 0}, {"": 0}, {"x": -1}, {"x": MaxCounterValue + 1}} {
		if err := validateCounterRefs(Card{Counters: counters}); err == nil {
			t.Fatal("accepted invalid declaration")
		}
	}
}
