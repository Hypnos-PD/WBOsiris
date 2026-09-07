package ir

import (
	"encoding/json"
	"testing"
)

func TestTriggerLimitDecoder(t *testing.T) {
	for _, scope := range []string{"", "own", "oppo", "any", "both", "2"} {
		trigger := EventTrigger{Kind: "event", Event: "follower_summoned", Side: "own", SubjectType: "follower", OncePerTurn: scope}
		data, err := json.Marshal(trigger)
		if err != nil {
			t.Fatal(err)
		}
		_, err = decodeTrigger(data)
		if (err == nil) != (scope == "" || scope == "own" || scope == "oppo" || scope == "any") {
			t.Fatal(scope, err)
		}
	}
	for _, event := range []string{"evolved", "super_evolved", "card_discarded"} {
		trigger := EventTrigger{Kind: "event", Event: event, Side: "own", SelfOnly: true, OncePerTurn: "own"}
		if event != "card_discarded" {
			trigger.SubjectType = "follower"
		}
		data, _ := json.Marshal(trigger)
		if _, err := decodeTrigger(data); err == nil {
			t.Fatal("accepted self limit", event)
		}
	}
	if ValidGrantedTrigger(EventTrigger{Kind: "event", Event: "turn_ended", Side: "own", OncePerTurn: "own"}) {
		t.Fatal("accepted limited grant")
	}
}
