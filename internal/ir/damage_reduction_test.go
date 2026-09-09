package ir

import "testing"

func TestDecodeRejectsNegativeDamageReduction(t *testing.T) {
	data := []byte(`{"kind":"set_damage_reduction","target":{"kind":"self","valueType":"entity"},"amount":-1}`)
	if _, err := decodeEffect(data, map[string]bool{}); err == nil {
		t.Fatal("decoded negative damage reduction")
	}
}
