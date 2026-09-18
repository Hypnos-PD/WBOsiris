package project

import (
	"testing"

	"wbo/internal/ir"
)

// S-71：`when own|oppo follower attacks [leader] [where …]` 监听其他随从宣告攻击，
// 绑定名 `attacker` 指向攻击方。
func TestAttackEventCompiles(t *testing.T) {
	pack, ds := compile(t, validCard(`
		when own follower attacks where trait marine {
			buff attacker +1/+0;
		}
		when oppo follower attacks leader where keyword storm {
			buff attacker -3/-0 until turn ends;
		}`))
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	ally, ok := pack.Cards[0].Abilities[0].Trigger.(ir.EventTrigger)
	if !ok || ally.Event != "attacked" || ally.Side != "own" || ally.SubjectType != "follower" || ally.TargetKind != "" || ally.Predicate == nil {
		t.Fatalf("attack listener did not compile: %#v", pack.Cards[0].Abilities[0].Trigger)
	}
	enemy, ok := pack.Cards[0].Abilities[1].Trigger.(ir.EventTrigger)
	if !ok || enemy.Event != "attacked" || enemy.Side != "oppo" || enemy.TargetKind != "leader" {
		t.Fatalf("leader attack listener did not compile: %#v", pack.Cards[0].Abilities[1].Trigger)
	}
	body := pack.Cards[0].Abilities[1].Body
	buff, ok := body[0].(ir.TargetEffect)
	if !ok || buff.Kind != "buff_stats" || buff.Until != "turn_end" {
		t.Fatalf("temporary buff did not compile: %#v", body[0])
	}
	if ref, ok := buff.Target.(ir.BindingRef); !ok || ref.Name != "attacker" {
		t.Fatalf("attacker binding lost: %#v", buff.Target)
	}
	if _, ds := compile(t, validCard(`when own follower attacks leader { draw 1; }`)); len(ds) != 0 {
		t.Fatal(ds)
	}
}
