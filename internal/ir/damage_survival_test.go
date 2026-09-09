package ir

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestDamageSurvivalTriggerDecode(t *testing.T) {
	for _, during := range []string{"", "own", "oppo"} {
		for _, once := range []string{"", "any", "own", "oppo"} {
			trigger := EventTrigger{Kind: "event", Event: "damaged", Side: "own", SubjectType: "follower", SelfOnly: true, DuringTurn: during, OncePerTurn: once}
			data, _ := json.Marshal(trigger)
			decoded, err := decodeTrigger(data)
			if err != nil || !reflect.DeepEqual(trigger, decoded) {
				t.Fatal(decoded, err)
			}
		}
	}
	base := EventTrigger{Kind: "event", Event: "damaged", Side: "own", SubjectType: "follower", SelfOnly: true}
	for _, change := range []func(*EventTrigger){
		func(v *EventTrigger) { v.SelfOnly = false }, func(v *EventTrigger) { v.Side = "oppo" },
		func(v *EventTrigger) { v.SourceZone = "hand" }, func(v *EventTrigger) { v.SubjectType = "leader" },
		func(v *EventTrigger) { v.DuringTurn = "any" }, func(v *EventTrigger) { v.OncePerTurn = "self" },
		func(v *EventTrigger) { v.Event = "evolved"; v.DuringTurn = "own" },
	} {
		value := base
		change(&value)
		data, _ := json.Marshal(value)
		if _, err := decodeTrigger(data); err == nil {
			t.Fatal("accepted invalid survival trigger", string(data))
		}
	}
}
