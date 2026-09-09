package ir

import "testing"

func TestDecodeAttackHistoryRequiresExplicitBooleanAndSide(t *testing.T) {
	for _, raw := range []string{
		`{"kind":"attack_history","side":"own"}`,
		`{"kind":"attack_history","side":"own","attacked":null}`,
		`{"kind":"attack_history","side":"own","attacked":0}`,
		`{"kind":"attack_history","side":"own","attacked":"false"}`,
		`{"kind":"attack_history","side":"self","attacked":false}`,
		`{"kind":"attack_history","attacked":false}`,
		`{"kind":"attack_history","side":"oppo","attacked":true,"turn":1}`,
	} {
		if _, err := decodeCondition([]byte(raw)); err == nil {
			t.Fatal("accepted malformed attack history", raw)
		}
	}
}
