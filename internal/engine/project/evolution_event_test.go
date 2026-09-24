package project

import (
	"testing"

	"wbo/internal/engine/ir"
)

// `when own|oppo follower evolved|super_evolved [other]`：监听其它随从的进化，
// 并为本次进化的随从提供 `evolved` 绑定。
func TestEvolutionEventsCarrySubjectAndExclusion(t *testing.T) {
	pack, ds := compile(t, validCard(`
		when own follower super_evolved other {
			buff evolved +2/+0;
			buff self +2/+0;
		}
		when own follower evolved while self in hand {
			set cost self 1;
		}`))
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	if len(pack.Cards[0].Abilities) != 2 {
		t.Fatalf("abilities = %d", len(pack.Cards[0].Abilities))
	}
	other := pack.Cards[0].Abilities[0].Trigger.(ir.EventTrigger)
	if other.Event != "super_evolved" || other.Side != "own" || other.SubjectType != "follower" || !other.ExcludeSelf || other.SelfOnly {
		t.Fatalf("other-follower listener lost its shape: %#v", other)
	}
	hand := pack.Cards[0].Abilities[1].Trigger.(ir.EventTrigger)
	if hand.Event != "evolved" || hand.SourceZone != "hand" || hand.ExcludeSelf {
		t.Fatalf("hand listener lost its shape: %#v", hand)
	}
	body := pack.Cards[0].Abilities[1].Body
	if len(body) != 1 || body[0].(ir.TargetEffect).Kind != "set_cost" {
		t.Fatalf("set cost did not compile: %#v", body)
	}
}

func TestEvolutionEventAndSetCostRejectBadShapes(t *testing.T) {
	for _, effect := range []string{
		`when self super_evolved other { draw 1; }`,
		`when own follower super_evolved other other { draw 1; }`,
		`when own turn starts other { draw 1; }`,
		`when own follower super_evolved { set cost; }`,
		`when own follower super_evolved { set cost self; }`,
	} {
		if _, ds := compile(t, validCard(effect)); len(ds) == 0 {
			t.Fatalf("%q must not compile", effect)
		}
	}
}
