package project

import (
	"testing"

	"wbo/internal/engine/ir"
)

// S-52：`invoke self;` 与"在牌组中发动"的回合时点监听。
func TestInvokeCompiles(t *testing.T) {
	pack, ds := compile(t, validCard(`
		when own turn starts while self in deck {
			if own.evolutions >= 6 {
				invoke self;
			}
		}
		when self invoked {
			return self to hand;
		}`))
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	ability := pack.Cards[0].Abilities[0]
	trigger, ok := ability.Trigger.(ir.EventTrigger)
	if !ok || trigger.Event != "turn_started" || trigger.SourceZone != "deck" {
		t.Fatalf("in-deck trigger lost: %#v", ability.Trigger)
	}
	branch, ok := ability.Body[0].(ir.IfEffect)
	if !ok {
		t.Fatalf("if did not compile: %#v", ability.Body[0])
	}
	condition, ok := branch.Condition.(ir.CompareCondition)
	if !ok || condition.Left.Field != "evolutions" || condition.Op != "ge" || condition.Right != 6 {
		t.Fatalf("evolution condition lost: %#v", branch.Condition)
	}
	invoked, ok := branch.Then[0].(ir.InvokeEffect)
	if !ok || invoked.Target == nil {
		t.Fatalf("invoke did not compile: %#v", branch.Then[0])
	}
	invokedTrigger, ok := pack.Cards[0].Abilities[1].Trigger.(ir.EventTrigger)
	if !ok || invokedTrigger.Event != "card_invoked" || !invokedTrigger.SelfOnly {
		t.Fatalf("invoked trigger lost: %#v", pack.Cards[0].Abilities[1].Trigger)
	}
	// "在牌组中发动"只对回合开始/结束成立，其他事件仍不能写在牌组里。
	for _, invalid := range []string{
		`when own earthrite while self in deck { draw 1; }`,
		`when self summoned while self in deck { draw 1; }`,
	} {
		if _, ds := compile(t, validCard(invalid)); len(ds) == 0 {
			t.Fatalf("accepted invalid deck source zone: %s", invalid)
		}
	}
}
