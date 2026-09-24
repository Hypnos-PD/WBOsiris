package ir

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestDestructionResultRoundTrip(t *testing.T) {
	e := TargetEffect{NodeBase: NodeBase{ID: strings.Repeat("a", 32), Origin: testOrigin()}, Kind: "destroy", Output: "destroyed", Target: ZoneRef{Kind: "zone", Side: "own", Zone: "field", Member: "amulet"}}
	data, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	got, err := decodeEffect(data, map[string]bool{})
	if err != nil || !reflect.DeepEqual(e, got) {
		t.Fatal(err, got)
	}
	for _, output := range []string{"destroyed", "unknown"} {
		var raw map[string]any
		if err := json.Unmarshal(data, &raw); err != nil {
			t.Fatal(err)
		}
		raw["output"] = output
		if output == "destroyed" {
			raw["kind"] = "banish"
		}
		bad, _ := json.Marshal(raw)
		if _, err := decodeEffect(bad, map[string]bool{}); err == nil {
			t.Fatal("accepted invalid result output")
		}
	}
	for _, source := range []Ref{BindingRef{Kind: "binding", Name: "destroyed"}, FilterRef{Kind: "filter", Source: BindingRef{Kind: "binding", Name: "drawn"}, Predicate: FieldPredicate{Kind: "has_type", CardType: "follower"}}} {
		expr := &CountExpr{Kind: "count", Source: source}
		data, _ := json.Marshal(expr)
		_, got, err := decodeEffectAmount(data)
		if err != nil || !reflect.DeepEqual(expr, got) {
			t.Fatal(err, got)
		}
	}
}
