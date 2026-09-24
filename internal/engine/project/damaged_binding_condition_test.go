package project

import (
	"testing"

	"wbo/internal/engine/ir"
)

// `if <绑定> damaged { … }`：条件读取绑定实例的受伤状态（例如【攻击时】的交战对象）。
func TestDamagedBindingConditionCompiles(t *testing.T) {
	pack, ds := compile(t, validCard(`
		attack {
			if opponent damaged {
				destroy opponent;
			}
		}`))
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	ability := pack.Cards[0].Abilities[0]
	if ir.TriggerKind(ability.Trigger) != "attack" {
		t.Fatalf("unexpected trigger: %#v", ability.Trigger)
	}
	branch, ok := ability.Body[0].(ir.IfEffect)
	if !ok {
		t.Fatalf("unexpected body: %#v", ability.Body[0])
	}
	condition, ok := branch.Condition.(ir.IsDamagedCondition)
	if !ok || condition.Name != "opponent" {
		t.Fatalf("damaged condition not preserved: %#v", branch.Condition)
	}
}

func TestDamagedBindingConditionRejectsBadShapes(t *testing.T) {
	for _, line := range []string{
		"if opponent damaged extra { draw 1; }",
		"if damaged { draw 1; }",
		"if opponent damaged 1 { draw 1; }",
	} {
		if _, ds := compile(t, validCard("attack { "+line+" }")); len(ds) == 0 {
			t.Fatalf("%q must not compile", line)
		}
	}
}
