package project

import (
	"testing"

	"wbo/internal/ir"
)

// S-86：条件的右值可以是数值表达式（"若自己的主战者的生命值大于对手的主战者的生命值"）。
func TestNumericConditionRightSideCompiles(t *testing.T) {
	pack, ds := compile(t, validCard(`
		fanfare {
			if own.life > oppo.life {
				summon 1 card 12345678;
			}
		}`))
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	effect, ok := pack.Cards[0].Abilities[0].Body[0].(ir.IfEffect)
	if !ok {
		t.Fatalf("if did not compile: %#v", pack.Cards[0].Abilities[0].Body[0])
	}
	condition, ok := effect.Condition.(ir.CompareCondition)
	if !ok || condition.Op != "gt" || condition.Left.Field != "life" || condition.Left.Side != "own" {
		t.Fatalf("condition lost its left scalar: %#v", effect.Condition)
	}
	if condition.Right != 0 {
		t.Fatalf("expression condition kept a literal right value: %#v", condition)
	}
	right, ok := condition.RightExpr.(*ir.Scalar)
	if !ok || right.Kind != "scalar" || right.Side != "oppo" || right.Field != "life" {
		t.Fatalf("condition lost its right expression: %#v", condition.RightExpr)
	}
}

// S-41：数值相减（"X 为对手的战场上的随从数减去自己的战场上的随从数"）。
func TestDifferenceCountCompiles(t *testing.T) {
	pack, ds := compile(t, validCard(`
		fanfare {
			random victims from oppo.field.followers count count(oppo.field.followers) - count(own.field.followers);
			destroy victims;
		}`))
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	selection, ok := pack.Cards[0].Abilities[0].Body[0].(ir.SelectionEffect)
	if !ok {
		t.Fatalf("random selection did not compile: %#v", pack.Cards[0].Abilities[0].Body[0])
	}
	difference, ok := selection.CountExpr.(*ir.DifferenceExpr)
	if !ok || difference.Kind != "difference" {
		t.Fatalf("count expression is not a difference: %#v", selection.CountExpr)
	}
	if left, ok := difference.Left.(*ir.CountExpr); !ok || left.Source.(ir.ZoneRef).Side != "oppo" {
		t.Fatalf("difference left operand lost: %#v", difference.Left)
	}
	if right, ok := difference.Right.(*ir.CountExpr); !ok || right.Source.(ir.ZoneRef).Side != "own" {
		t.Fatalf("difference right operand lost: %#v", difference.Right)
	}
	// 字面量不能当相减的操作数：无法用数值表达式表示，只能直接写数字。
	if _, ds := compile(t, validCard(`
		fanfare {
			damage oppo.leader count(own.field.followers) - 1;
		}`)); len(ds) == 0 {
		t.Fatal("count(...) - 1 was accepted")
	}
}

// S-90：`count(own.entered other where card X)` 读取"本场对战中进入战场的其他同名卡"。
func TestEnteredCountCompiles(t *testing.T) {
	pack, ds := compile(t, validCard(`
		when self summoned {
			if count(own.entered other where card 12345678) >= 5 {
				buff self +3/+3;
			}
		}`))
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	effect, ok := pack.Cards[0].Abilities[0].Body[0].(ir.IfEffect)
	if !ok {
		t.Fatalf("if did not compile: %#v", pack.Cards[0].Abilities[0].Body[0])
	}
	condition, ok := effect.Condition.(ir.CountCondition)
	if !ok || condition.Op != "ge" || condition.Right != 5 {
		t.Fatalf("entered condition lost: %#v", effect.Condition)
	}
	filter, ok := condition.Source.(ir.FilterRef)
	if !ok {
		t.Fatalf("entered count lost its filter: %#v", condition.Source)
	}
	exclude, ok := filter.Source.(ir.ExcludeRef)
	if !ok {
		t.Fatalf("entered count lost the self exclusion: %#v", filter.Source)
	}
	zone, ok := exclude.Source.(ir.ZoneRef)
	if !ok || zone.Zone != "entered" || zone.Side != "own" {
		t.Fatalf("entered count lost its zone: %#v", exclude.Source)
	}
	predicate, ok := filter.Predicate.(ir.FieldPredicate)
	if !ok || predicate.Kind != "has_card" || predicate.CardID != 12345678 {
		t.Fatalf("entered count lost its card filter: %#v", filter.Predicate)
	}
	// 集合计数之间的比较使用两侧都是表达式的通用比较节点。
	compare, ds := compile(t, validCard(`
		fanfare {
			if count(own.hand) > count(oppo.hand) {
				draw 1;
			}
		}`))
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	both, ok := compare.Cards[0].Abilities[0].Body[0].(ir.IfEffect)
	if !ok {
		t.Fatalf("if did not compile: %#v", compare.Cards[0].Abilities[0].Body[0])
	}
	comparison, ok := both.Condition.(ir.CompareCondition)
	if !ok || comparison.LeftExpr == nil || comparison.RightExpr == nil || comparison.Op != "gt" {
		t.Fatalf("expression comparison lost: %#v", both.Condition)
	}
}
