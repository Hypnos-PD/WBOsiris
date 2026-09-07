package ir

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestDestructionBatchRoundTripAndValidation(t *testing.T) {
	batch := DestructionBatchRef{Kind: "destruction_batch", Targets: []Ref{SelfRef{Kind: "self"}, BindingRef{Kind: "binding", Name: "enemy"}}}
	e := TargetEffect{NodeBase: NodeBase{ID: strings.Repeat("a", 32), Origin: testOrigin()}, Kind: "destroy", Target: batch, Output: "destroyed"}
	data, _ := json.Marshal(e)
	decoded, err := decodeEffect(data, map[string]bool{})
	if err != nil || !reflect.DeepEqual(e, decoded) {
		t.Fatal(decoded, err)
	}
	for _, refs := range [][]Ref{
		nil, {SelfRef{Kind: "self"}}, {SelfRef{Kind: "self"}, SelfRef{Kind: "self"}},
		{SelfRef{Kind: "self"}, LeaderRef{Kind: "leader", Side: "oppo"}},
		{SelfRef{Kind: "self"}, batch},
		{BindingRef{Kind: "binding", Name: ""}, SelfRef{Kind: "self"}},
	} {
		e.Target = DestructionBatchRef{Kind: "destruction_batch", Targets: refs}
		data, _ = json.Marshal(e)
		if _, err := decodeEffect(data, map[string]bool{}); err == nil {
			t.Fatal("accepted invalid batch", string(data))
		}
	}
	e.Target = batch
	for _, kind := range []string{"banish", "discard", "damage"} {
		e.Kind, e.Output = kind, ""
		data, _ = json.Marshal(e)
		if _, err := decodeEffect(data, map[string]bool{}); err == nil {
			t.Fatal("accepted batch on", kind)
		}
	}
	e.Kind, e.Predicate = "destroy", FieldPredicate{Kind: "has_card", CardID: 10001110}
	data, _ = json.Marshal(e)
	if _, err := decodeEffect(data, map[string]bool{}); err == nil {
		t.Fatal("accepted filtered batch")
	}
}
