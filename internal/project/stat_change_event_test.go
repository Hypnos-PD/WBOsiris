package project

import (
	"testing"

	"wbo/internal/ir"
)

// `when self stats increased` / `when own|oppo follower life decreased`：
// 战场上的身材增加与生命值减少各是一种事件，都支持 `once per own turn`。
func TestStatChangeEventsCompile(t *testing.T) {
	pack, ds := compile(t, validCard(`
		when self stats increased {
			heal own.leader 1;
		}
		when oppo follower life decreased once per own turn {
			heal own.leader 1;
		}
		when own follower life decreased {
			draw 1;
		}`))
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	first := pack.Cards[0].Abilities[0].Trigger.(ir.EventTrigger)
	if first.Event != "stats_increased" || !first.SelfOnly || first.SubjectType != "follower" || first.Side != "own" {
		t.Fatalf("self stats increased lost its shape: %#v", first)
	}
	second := pack.Cards[0].Abilities[1].Trigger.(ir.EventTrigger)
	if second.Event != "life_decreased" || second.Side != "oppo" || second.SubjectType != "follower" || second.OncePerTurn != "own" || second.SelfOnly {
		t.Fatalf("opponent life decreased lost its shape: %#v", second)
	}
	third := pack.Cards[0].Abilities[2].Trigger.(ir.EventTrigger)
	if third.Event != "life_decreased" || third.Side != "own" {
		t.Fatalf("own life decreased lost its shape: %#v", third)
	}
}

func TestStatChangeEventsRejectBadShapes(t *testing.T) {
	for _, line := range []string{
		"when self stats decreased { draw 1; }",
		"when own follower stats decreased { draw 1; }",
		"when own amulet life decreased { draw 1; }",
	} {
		if _, ds := compile(t, validCard(line)); len(ds) == 0 {
			t.Fatalf("%q must not compile", line)
		}
	}
}
