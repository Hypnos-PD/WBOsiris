package ir

import (
	"encoding/json"
	"testing"
)

func TestEventSourceZoneDecoder(t *testing.T) {
	for _, zone := range []string{"", "hand", "field", "deck", "graveyard"} {
		trigger := EventTrigger{Kind: "event", Event: "follower_left", Side: "own", SubjectType: "follower", SourceZone: zone}
		data, err := json.Marshal(trigger)
		if err != nil {
			t.Fatal(err)
		}
		_, err = decodeTrigger(data)
		valid := zone == "" || zone == "hand" || zone == "field"
		if (err == nil) != valid {
			t.Fatal(zone, err)
		}
	}
}
