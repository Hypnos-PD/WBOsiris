package ir

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestLeaderHealedTriggerDecoder(t *testing.T) {
	for _, side := range []string{"own", "oppo"} {
		trigger := EventTrigger{Kind: "event", Event: "healed", Side: side, SubjectType: "leader", OncePerTurn: "own"}
		data, err := json.Marshal(trigger)
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := decodeTrigger(data)
		if err != nil || !reflect.DeepEqual(decoded, trigger) {
			t.Fatal(decoded, err)
		}
	}
	for _, raw := range []string{
		`{"kind":"event","event":"healed","side":"own"}`,
		`{"kind":"event","event":"healed","side":"own","subjectType":"follower"}`,
		`{"kind":"event","event":"healed","side":"own","subjectType":"leader","selfOnly":true}`,
		`{"kind":"event","event":"follower_summoned","side":"own","subjectType":"leader"}`,
		`{"kind":"event","event":"healed","side":"own","subjectType":"leader","predicate":{"kind":"has_keyword","keyword":"ward"}}`,
	} {
		if _, err := decodeTrigger([]byte(raw)); err == nil {
			t.Fatal("accepted invalid leader listener", raw)
		}
	}
}
