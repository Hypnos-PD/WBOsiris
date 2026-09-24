package project

import (
	"testing"

	"wbo/internal/engine/ir"
)

// `damage_cap N` 是实例固有状态（单次受到伤害最多 N），`add damage_taken_up to <主战者>`
// 是主战者关键词。两者都必须编译进可执行 IR，并拒绝错误形状。
func TestDamageCapCompilesToIntrinsicState(t *testing.T) {
	pack, ds := compile(t, validCard(`
		ward;
		damage_cap 3;`))
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	states := pack.Cards[0].IntrinsicState
	if len(states) != 1 || states[0].Kind != "damage_cap" || states[0].Initial != 3 {
		t.Fatalf("damage_cap did not compile into an intrinsic state: %#v", states)
	}
}

func TestDamageTakenUpTargetsALeader(t *testing.T) {
	pack, ds := compile(t, validCard(`
		fanfare {
			add damage_taken_up to oppo.leader;
		}`))
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	fanfare := pack.Cards[0].Abilities[0].Body
	if len(fanfare) != 1 {
		t.Fatalf("fanfare body = %#v", fanfare)
	}
	effect, ok := fanfare[0].(ir.TargetEffect)
	if !ok || effect.Kind != "add_keyword" || effect.Keyword != "damage_taken_up" {
		t.Fatalf("damage_taken_up did not compile into a keyword grant: %#v", fanfare[0])
	}
	if leader, ok := effect.Target.(ir.LeaderRef); !ok || leader.Side != "oppo" {
		t.Fatalf("keyword grant lost its leader target: %#v", effect.Target)
	}
}

func TestDamageCapRejectsInvalidShapes(t *testing.T) {
	if _, ds := compile(t, validCard(`damage_cap;`)); len(ds) == 0 {
		t.Fatal("damage_cap without a value was accepted")
	}
	if _, ds := compile(t, validCard(`damage_cap -1;`)); len(ds) == 0 {
		t.Fatal("damage_cap with a negative value was accepted")
	}
}
