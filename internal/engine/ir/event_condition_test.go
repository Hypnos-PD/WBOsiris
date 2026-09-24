package ir

import (
	"encoding/json"
	"testing"
)

func TestEventConditionStrictDecode(t *testing.T) {
	for _, field := range []string{"hand_count", "earthsigils"} {
		trigger := EventTrigger{Kind: "event", Event: "turn_ended", Side: "own", Condition: CompareCondition{Kind: "compare", Left: Scalar{Kind: "scalar", Side: "own", Field: field}, Op: "le", Right: 5}}
		data, err := json.Marshal(trigger)
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := decodeTrigger(data)
		if err != nil || decoded.(EventTrigger).Condition.(CompareCondition).Left.Field != field {
			t.Fatal(err, decoded)
		}
	}
	for _, condition := range []string{`null`, `{}`, `{"kind":"compare","left":{"kind":"scalar","side":"own","field":"hand"},"op":"le","right":5}`, `{"kind":"compare","left":{"kind":"fusion_material_scalar","field":"cost"},"op":"le","right":5}`, `{"kind":"overflow","side":"own","extra":1}`} {
		data := []byte(`{"kind":"event","event":"turn_ended","side":"own","condition":` + condition + `}`)
		if _, err := decodeTrigger(data); err == nil {
			t.Fatal("accepted malformed condition", condition)
		}
	}
	card := Card{Abilities: []Ability{{Trigger: EventTrigger{Condition: CompareCondition{Left: Scalar{Kind: "self_counter", Field: "missing"}}}}}}
	if err := validateCounterRefs(card); err == nil {
		t.Fatal("accepted undeclared event counter")
	}
}
