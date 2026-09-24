package ir

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestHistorySummonRoundTripAndStrictShapes(t *testing.T) {
	e := HistorySummonEffect{NodeBase: NodeBase{ID: strings.Repeat("a", 32), Origin: testOrigin()}, Kind: "summon_from_history", Owner: "own", Source: ZoneRef{Kind: "zone", Side: "oppo", Zone: "destroyed", Member: "amulet"}, Count: 2, Extremum: &SelectionExtremum{Direction: "highest", Field: "base_cost"}, Output: "summoned"}
	data, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeEffect(data, map[string]bool{})
	if err != nil || !reflect.DeepEqual(e, decoded) {
		t.Fatal(decoded, err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ key, value string }{
		{"count", "0"}, {"count", "-1"}, {"count", "65536"}, {"count", "null"},
		{"source", `{"kind":"zone","side":"own","zone":"graveyard"}`},
		{"source", `{"kind":"zone","side":"own","zone":"destroyed","member":"spell"}`},
		{"source", `{"kind":"binding","name":"old"}`},
		{"source", `{"kind":"leader","side":"own"}`},
		{"owner", `"both"`}, {"output", `"old"`},
		{"extremum", `{"direction":"highest","field":"base_mana"}`},
		{"extra", "true"},
	} {
		old, exists := fields[tc.key]
		fields[tc.key] = json.RawMessage(tc.value)
		bad, _ := json.Marshal(fields)
		if _, err := decodeEffect(bad, map[string]bool{}); err == nil {
			t.Fatal("accepted malformed history summon", tc)
		}
		if exists {
			fields[tc.key] = old
		} else {
			delete(fields, tc.key)
		}
	}
	e.Source = FilterRef{Kind: "filter", Source: HistoryRef{Kind: "history", Side: "own", Window: "this_turn", Member: "follower"}, Predicate: FieldPredicate{Kind: "has_keyword", Keyword: "ward"}}
	data, _ = json.Marshal(e)
	if _, err := decodeEffect(data, map[string]bool{}); err != nil {
		t.Fatal(err)
	}
}
