package ir

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestRestoreResourceRoundTripAndValidation(t *testing.T) {
	for _, side := range []string{"own", "oppo"} {
		want := AdjustEffect{NodeBase: NodeBase{ID: strings.Repeat("a", 32), Origin: testOrigin()}, Kind: "restore_resource", Owner: side, Resource: "pp"}
		data, err := json.Marshal(want)
		if err != nil {
			t.Fatal(err)
		}
		got, err := decodeEffect(data, map[string]bool{})
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatal("restore resource roundtrip failed", err)
		}
		for _, patch := range []map[string]any{{"owner": "field"}, {"resource": "ep"}, {"delta": 1}, {"times": 1}, {"target": map[string]string{"kind": "self"}}} {
			var object map[string]any
			if err := json.Unmarshal(data, &object); err != nil {
				t.Fatal(err)
			}
			for key, value := range patch {
				object[key] = value
			}
			invalid, _ := json.Marshal(object)
			if _, err := decodeEffect(invalid, map[string]bool{}); err == nil {
				t.Fatalf("accepted invalid restore %s", invalid)
			}
		}
	}
}

func TestInstanceCostOverrideSupportsZeroAndRejectsInvalidRange(t *testing.T) {
	for _, cost := range []int{-1, 0, 5, 65535, 65536} {
		want := TestInstance{InstanceID: strings.Repeat("a", 32), Alias: "source", CardID: 10134310, DeclaredType: "spell", Overrides: InstanceOverrides{Cost: &cost}}
		data, err := json.Marshal(want)
		if err != nil {
			t.Fatal(err)
		}
		got, err := decodeInstance(data)
		if cost < 0 || cost > 65535 {
			if err == nil {
				t.Fatal("accepted invalid cost", cost)
			}
		} else if err != nil || got.Overrides.Cost == nil || *got.Overrides.Cost != cost {
			t.Fatal("cost override lost zero or integer value", cost, err)
		}
	}
}
