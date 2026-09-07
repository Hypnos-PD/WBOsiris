package ir

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestSummonCopiesRoundTripAndShape(t *testing.T) {
	e := CardEffect{NodeBase: NodeBase{ID: strings.Repeat("a", 32), Origin: testOrigin()}, Kind: "summon_copies", Owner: "own", Output: "summoned", Target: BindingRef{Kind: "binding", Name: "targets"}}
	data, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeEffect(data, map[string]bool{})
	if err != nil || !reflect.DeepEqual(e, decoded) {
		t.Fatalf("copy changed on round trip: %#v %v", decoded, err)
	}
	for field, invalid := range map[string]string{"owner": `"field"`, "output": `"drawn"`, "target": `null`, "count": `1`, "cardId": `12345678`, "preserveInstanceId": `true`} {
		var object map[string]json.RawMessage
		if err := json.Unmarshal(data, &object); err != nil {
			t.Fatal(err)
		}
		object[field] = json.RawMessage(invalid)
		raw, _ := json.Marshal(object)
		if _, err := decodeEffect(raw, map[string]bool{}); err == nil {
			t.Fatalf("accepted malformed copy %s", raw)
		}
	}
}

func TestCurrentCostPredicateRoundTrip(t *testing.T) {
	for _, op := range []string{"eq", "ne", "lt", "le", "gt", "ge"} {
		p := FieldPredicate{Kind: "compare", Field: "cost", Op: op, Value: 5}
		data, err := json.Marshal(p)
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := decodePredicate(data)
		if err != nil || !reflect.DeepEqual(p, decoded) {
			t.Fatalf("cost predicate changed: %#v %v", decoded, err)
		}
	}
}
