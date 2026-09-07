package ir

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestTransformTargetRoundTripAndValidation(t *testing.T) {
	for _, tc := range []struct {
		target Ref
		valid  bool
	}{
		{SelfRef{Kind: "self"}, true},
		{BindingRef{Kind: "binding", Name: "target"}, true},
		{ZoneRef{Kind: "zone", Side: "own", Zone: "hand", Member: "card"}, true},
		{ZoneRef{Kind: "zone", Side: "oppo", Zone: "deck", Member: "card"}, true},
		{FilterRef{Kind: "filter", Source: ZoneRef{Kind: "zone", Side: "own", Zone: "field", Member: "follower"}, Predicate: FieldPredicate{Kind: "compare", Field: "cost", Op: "le", Value: 2}}, true},
		{LeaderRef{Kind: "leader", Side: "own"}, false},
		{LeaderSetRef{Kind: "leaders"}, false},
		{ZoneRef{Kind: "zone", Side: "own", Zone: "graveyard", Member: "card"}, false},
		{FilterRef{Kind: "filter", Source: ZoneRef{Kind: "zone", Side: "own", Zone: "destroyed", Member: "card"}, Predicate: FieldPredicate{Kind: "compare", Field: "cost", Op: "le", Value: 2}}, false},
	} {
		e := CardEffect{NodeBase: NodeBase{ID: strings.Repeat("a", 32), Origin: testOrigin()}, Kind: "transform", Target: tc.target, CardID: 12345678, PreserveInstanceID: true, PreserveMaterials: true}
		data, err := json.Marshal(e)
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := decodeEffect(data, map[string]bool{})
		if (err == nil) != tc.valid {
			t.Fatal(string(data), err)
		}
		if tc.valid && !reflect.DeepEqual(e, decoded) {
			t.Fatal("transform round trip changed", decoded)
		}
	}
}
