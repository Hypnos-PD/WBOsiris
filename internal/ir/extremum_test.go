package ir

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestExtremumSelectionRoundTrip(t *testing.T) {
	for _, direction := range []string{"highest", "lowest"} {
		for _, field := range []string{"attack", "life", "cost", "base_attack", "base_life", "base_cost"} {
			e := SelectionEffect{NodeBase: NodeBase{ID: strings.Repeat("a", 32), Origin: testOrigin()}, Kind: "random_choose", Policy: "random", Binding: "target", Source: ZoneRef{Kind: "zone", Side: "oppo", Zone: "field", Member: "follower"}, Extremum: &SelectionExtremum{Direction: direction, Field: field}}
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
			for _, invalid := range []string{`{}`, `{"direction":"highest","field":"mana"}`, `{"direction":"middle","field":"attack"}`, `{"direction":"highest","field":"cost","extra":1}`, `{"direction":"highest","direction":"lowest","field":"cost"}`} {
				raw["extremum"] = json.RawMessage(invalid)
				bad, _ := json.Marshal(raw)
				if _, err := decodeEffect(bad, map[string]bool{}); err == nil {
					t.Fatal("accepted invalid extremum", invalid)
				}
			}
		}
	}
}
