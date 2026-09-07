package ir

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestSelectionCountRoundTrip(t *testing.T) {
	for _, count := range []int{0, 1, 2, 65535} {
		e := SelectionEffect{NodeBase: NodeBase{ID: strings.Repeat("a", 32), Origin: testOrigin()}, Kind: "choose", Policy: "optional", Binding: "targets", Source: ZoneRef{Kind: "zone", Side: "oppo", Zone: "field", Member: "follower"}, Count: count}
		data, err := json.Marshal(e)
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := decodeEffect(data, map[string]bool{})
		if err != nil || !reflect.DeepEqual(e, decoded) {
			t.Fatalf("selection changed: %#v %v", decoded, err)
		}
		if e.SelectionCount() != max(1, count) {
			t.Fatal("wrong default count")
		}
		var object map[string]json.RawMessage
		if err := json.Unmarshal(data, &object); err != nil {
			t.Fatal(err)
		}
		for _, invalid := range []string{`0`, `-1`, `65536`, `null`, `1.5`, `"2"`, `{}`} {
			object["count"] = json.RawMessage(invalid)
			raw, _ := json.Marshal(object)
			if _, err := decodeEffect(raw, map[string]bool{}); err == nil {
				t.Fatalf("accepted count %s", invalid)
			}
		}
	}
}

func TestMultipleSelectionActionRoundTrip(t *testing.T) {
	a, b := strings.Repeat("a", 32), strings.Repeat("b", 32)
	for _, action := range []SelectAction{{Kind: "select", Target: a}, {Kind: "select", Targets: []string{a, b}}} {
		data, _ := json.Marshal(action)
		decoded, err := decodeAction(data)
		if err != nil || !reflect.DeepEqual(decoded, action) {
			t.Fatalf("selection response changed: %#v %v", decoded, err)
		}
	}
	for _, action := range []SelectAction{{Kind: "select"}, {Kind: "select", Target: a, Targets: []string{b}}, {Kind: "select", Targets: []string{a, a}}, {Kind: "select", Targets: []string{"unknown"}}} {
		data, _ := json.Marshal(action)
		if _, err := decodeAction(data); err == nil {
			t.Fatalf("accepted invalid selection: %s", data)
		}
	}
}
