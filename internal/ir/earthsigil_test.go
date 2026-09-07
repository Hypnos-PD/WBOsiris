package ir

import "testing"

func TestEarthSigilDependencyValidation(t *testing.T) {
	for _, delta := range []int{0, 4} {
		for _, present := range []bool{false, true} {
			card := Card{CardType: "spell", PlayEffects: []Effect{ModeEffect{Options: []ModeOption{{ID: 1, Body: []Effect{AdjustEffect{Kind: "adjust_earthsigil", Delta: delta}}}}}}}
			cards := map[int]bool{}
			if present {
				cards[MagicSedimentCardID] = true
			}
			if err := validateCardRefs(card, cards); (err != nil) != (delta > 0 && !present) {
				t.Fatalf("delta=%d present=%t err=%v", delta, present, err)
			}
		}
	}
}
