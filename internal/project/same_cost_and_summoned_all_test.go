package project

import (
	"testing"

	"wbo/internal/ir"
)

// S-58：`own.hand has 4 same cost` 判断手牌里是否有四张以上同费用卡牌。
func TestSameCostConditionCompiles(t *testing.T) {
	pack, ds := compile(t, validCard(`
		fanfare {
			if own.hand has 4 same cost {
				summon 1 card 12345678;
			}
		}`))
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	condition, ok := pack.Cards[0].Abilities[0].Body[0].(ir.IfEffect).Condition.(ir.SameCostCondition)
	if !ok || condition.Kind != "same_cost" || condition.Side != "own" || condition.Zone != "hand" || condition.Count != 4 {
		t.Fatalf("same cost condition did not compile: %#v", pack.Cards[0].Abilities[0].Body[0])
	}
	if _, ds := compile(t, validCard(`fanfare { if own.hand has 1 same cost { draw 1; } }`)); len(ds) == 0 {
		t.Fatal("same cost condition accepted a threshold below two")
	}
}

// S-61：`summoned_all` 累计本次结算里召唤的全部实例，`summoned` 仍然只保留最近一次。
func TestSummonedAllBindingIsVisible(t *testing.T) {
	pack, ds := compile(t, validCard(`
		fanfare {
			summon 1 card 12345678;
			summon 1 card 12345678;
		}
		enhance 7 {
			add storm to summoned_all;
		}`))
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	body := pack.Cards[0].Abilities[1].Body
	add, ok := body[0].(ir.TargetEffect)
	if !ok || add.Kind != "add_keyword" {
		t.Fatalf("enhance body = %#v", body[0])
	}
	if ref, ok := add.Target.(ir.BindingRef); !ok || ref.Name != "summoned_all" {
		t.Fatalf("summoned_all was not preserved: %#v", add.Target)
	}
	if _, ds := compile(t, validCard(`enhance 7 { add storm to summoned_all; }`)); len(ds) == 0 {
		t.Fatal("summoned_all was accepted without a summon")
	}
}
