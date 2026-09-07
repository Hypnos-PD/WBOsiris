package ir

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestDiscardEffectAndTriggerRoundTrip(t *testing.T) {
	effect := TargetEffect{NodeBase: NodeBase{ID: strings.Repeat("a", 32), Origin: testOrigin()}, Kind: "discard", Target: ZoneRef{Kind: "zone", Side: "own", Zone: "hand"}}
	data, err := json.Marshal(effect)
	if err != nil {
		t.Fatal(err)
	}
	got, err := decodeEffect(data, map[string]bool{})
	if err != nil || !reflect.DeepEqual(got, effect) {
		t.Fatal(string(data), err)
	}
	for _, trigger := range []EventTrigger{
		{Kind: "event", Event: "card_discarded", Side: "own", SelfOnly: true},
		{Kind: "event", Event: "card_discarded", Side: "oppo"},
	} {
		data, _ := json.Marshal(trigger)
		got, err := decodeTrigger(data)
		if err != nil || !reflect.DeepEqual(got, trigger) {
			t.Fatal(err)
		}
	}
	for _, raw := range []string{
		`{"kind":"event","event":"card_discarded","side":"oppo","selfOnly":true}`,
		`{"kind":"event","event":"card_discarded","side":"own","selfOnly":true,"subjectType":"follower"}`,
	} {
		if _, err := decodeTrigger([]byte(raw)); err == nil {
			t.Fatal("accepted invalid self discard trigger")
		}
	}
	effect.Target = LeaderRef{Kind: "leader", Side: "own", ValueType: "leader"}
	data, _ = json.Marshal(effect)
	if _, err := decodeEffect(data, map[string]bool{}); err == nil {
		t.Fatal("accepted leader discard")
	}
	matcher := EventMatcher{Kind: "card_discarded", Subject: &EventTarget{Kind: "instance", InstanceID: strings.Repeat("b", 32)}}
	data, err = json.Marshal(matcher)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeMatcher(data)
	if err != nil || !reflect.DeepEqual(matcher, decoded) {
		t.Fatal("discard matcher roundtrip", err)
	}
}
