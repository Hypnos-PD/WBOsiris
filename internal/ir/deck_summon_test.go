package ir

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestDeckSummonRoundTripAndStrictSources(t *testing.T) {
	e := DeckSummonEffect{NodeBase: NodeBase{ID: strings.Repeat("a", 32), Origin: testOrigin()}, Kind: "summon_from_deck", Source: ZoneRef{Kind: "zone", Side: "own", Zone: "deck", Member: "amulet"}, Count: 3, DistinctNames: true, Output: "summoned"}
	data, _ := json.Marshal(e)
	got, err := decodeEffect(data, map[string]bool{})
	if err != nil || !reflect.DeepEqual(got, e) {
		t.Fatal(got, err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ key, value string }{
		{"source", `{"kind":"zone","side":"oppo","zone":"deck","member":"amulet"}`},
		{"source", `{"kind":"zone","side":"own","zone":"hand","member":"amulet"}`},
		{"source", `{"kind":"zone","side":"own","zone":"deck","member":"spell"}`},
		{"source", `{"kind":"zone","side":"own","zone":"deck"}`},
		{"source", `{"kind":"binding","name":"held"}`},
		{"count", "0"}, {"count", "65536"}, {"count", "null"}, {"distinctNames", `"true"`}, {"output", `"held"`},
	} {
		old := fields[tc.key]
		fields[tc.key] = json.RawMessage(tc.value)
		bad, _ := json.Marshal(fields)
		if _, err := decodeEffect(bad, map[string]bool{}); err == nil {
			t.Fatal("accepted invalid deck summon", tc)
		}
		fields[tc.key] = old
	}
	for _, raw := range []string{
		`{"kind":"event","event":"amulet_summoned","side":"own","subjectType":"follower"}`,
		`{"kind":"event","event":"follower_summoned","side":"own","subjectType":"amulet"}`,
		`{"kind":"event","event":"amulet_summoned","side":"own","subjectType":"amulet","selfOnly":true}`,
	} {
		if _, err := decodeTrigger([]byte(raw)); err == nil {
			t.Fatal("accepted wrong entry listener", raw)
		}
	}
	if _, err := decodeTrigger([]byte(`{"kind":"event","event":"amulet_summoned","side":"oppo","subjectType":"amulet"}`)); err != nil {
		t.Fatal(err)
	}
}
