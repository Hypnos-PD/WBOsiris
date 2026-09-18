package project

import (
	"testing"

	"wbo/internal/ir"
)

// S-68：`during own|oppo turn` 不再只用于"受到伤害时"，也可以限制主战者回复事件
// （"自己的主战者回复时，若为自己的回合"）。
func TestHealedEventAcceptsDuringTurn(t *testing.T) {
	pack, ds := compile(t, validCard(`
		when own leader healed during own turn {
			summon 1 card 12345678;
		}`))
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	trigger, ok := pack.Cards[0].Abilities[0].Trigger.(ir.EventTrigger)
	if !ok || trigger.Event != "healed" || trigger.DuringTurn != "own" {
		t.Fatalf("during-turn restriction was lost: %#v", pack.Cards[0].Abilities[0].Trigger)
	}
	if _, ds := compile(t, validCard(`
		when self survives damage during own turn {
			draw 1;
		}`)); len(ds) != 0 {
		t.Fatal("self survives damage during own turn regressed:", ds)
	}
}
