package ir

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCrestStrictDecode(t *testing.T) {
	valid := func() CrestDefinition {
		c := CrestDefinition{Origin: testOrigin(), Countdown: 2, Locales: map[string]Locale{}, Abilities: []Ability{{ID: strings.Repeat("a", 32), Origin: testOrigin(), Relation: "independent", Trigger: SimpleTrigger{Kind: "lastwords"}, Body: []Effect{}}}}
		for _, code := range []string{"chs", "eng", "jpn", "kor", "cht"} {
			c.Locales[code] = Locale{Name: "Crest", Text: "Rules"}
		}
		return c
	}
	for _, mutate := range []func(*CrestDefinition){
		func(c *CrestDefinition) { c.Countdown = -1 }, func(c *CrestDefinition) { c.Countdown = 65536 },
		func(c *CrestDefinition) { delete(c.Locales, "chs") }, func(c *CrestDefinition) { c.Counters = map[string]int{"x": -1} },
		func(c *CrestDefinition) { c.Abilities[0].Trigger = SimpleTrigger{Kind: "evolve"} },
		func(c *CrestDefinition) {
			c.Abilities[0].Trigger = EventTrigger{Kind: "event", Event: "turn_started", SourceZone: "hand"}
		},
	} {
		c := valid()
		mutate(&c)
		data, err := json.Marshal(c)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := decodeCrest(data, map[string]bool{}, map[string]bool{}); err == nil {
			t.Fatal("accepted invalid crest", string(data))
		}
	}
	c := valid()
	data, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodeCrest(data, map[string]bool{}, map[string]bool{}); err != nil {
		t.Fatal(err)
	}
	if _, err := decodeCrest(data, map[string]bool{c.Abilities[0].ID: true}, map[string]bool{}); err == nil {
		t.Fatal("accepted colliding ability")
	}
	if _, err := decodeRef([]byte(`{"kind":"zone","side":"own","zone":"crests"}`)); err == nil {
		t.Fatal("generic card selection can access crests")
	}
}
