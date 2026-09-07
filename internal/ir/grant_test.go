package ir

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestGrantRoundTripAndRejectUnsupportedAbility(t *testing.T) {
	e := GrantEffect{NodeBase: NodeBase{ID: strings.Repeat("a", 32), Origin: testOrigin()}, Kind: "grant_ability", Target: SelfRef{Kind: "self", ValueType: "entity"}, Ability: Ability{ID: strings.Repeat("b", 32), Origin: testOrigin(), Relation: "independent", Trigger: SimpleTrigger{Kind: "lastwords"}, Body: []Effect{}}, Labels: map[string]string{"chs": "谢幕曲"}}
	data, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeEffect(data, map[string]bool{})
	if err != nil || !reflect.DeepEqual(decoded, e) {
		t.Fatal("round trip", err, decoded)
	}
	for _, trigger := range []Trigger{SimpleTrigger{Kind: "fanfare"}, SimpleTrigger{Kind: "spellboost"}, EventTrigger{Kind: "event", Event: "follower_summoned", Side: "own"}} {
		e.Ability.Trigger = trigger
		data, _ := json.Marshal(e)
		if _, err := decodeEffect(data, map[string]bool{}); err == nil {
			t.Fatal("accepted trigger", trigger)
		}
	}
}
