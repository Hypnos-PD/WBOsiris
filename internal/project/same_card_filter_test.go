package project

import (
	"testing"

	"wbo/internal/ir"
)

// `where card <绑定>`：筛选出与绑定实例同一卡牌定义的对象（如"与其同名的所有随从"）。
// 配合 `superevolve extends evolve`，超进化沿用普通进化那次选择的目标。
func TestSameCardFilterCompiles(t *testing.T) {
	pack, ds := compile(t, validCard(`
		evolve {
			choose target from oppo.field.followers;
			banish target;
		}
		superevolve extends evolve {
			banish oppo.field.followers where card target;
		}`))
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	if len(pack.Cards[0].Abilities) != 2 {
		t.Fatalf("abilities = %d", len(pack.Cards[0].Abilities))
	}
	super := pack.Cards[0].Abilities[1]
	if super.Relation != "extends" {
		t.Fatalf("super evolve relation = %q", super.Relation)
	}
	banish, ok := super.Body[0].(ir.TargetEffect)
	if !ok || banish.Kind != "banish" {
		t.Fatalf("unexpected effect: %#v", super.Body[0])
	}
	// banish 的 where 筛选保存在效果自己的 Predicate 字段上。
	predicate, ok := banish.Predicate.(ir.FieldPredicate)
	if !ok || predicate.Kind != "same_card" {
		t.Fatalf("same card predicate not preserved: %#v", banish.Predicate)
	}
	binding, ok := predicate.CardRef.(ir.BindingRef)
	if !ok || binding.Name != "target" {
		t.Fatalf("same card predicate lost its binding: %#v", predicate.CardRef)
	}
	// 计划里超进化的第二步沿用第一步的绑定帧。
	plan := pack.Cards[0].ActionPlans
	if len(plan) != 2 || plan[1].Action != "superevolve" || len(plan[1].Steps) != 2 || plan[1].Steps[1].Frame != "continue" {
		t.Fatalf("super evolve plan does not share the evolve frame: %#v", plan)
	}
}

func TestSameCardFilterRejectsBadShapes(t *testing.T) {
	// `card` 后面既不是卡牌 ID 也不是标识符时才是非法形状。
	if _, ds := compile(t, validCard("fanfare { destroy oppo.field.followers where card 1 extra; }")); len(ds) == 0 {
		t.Fatal("malformed card filter must not compile")
	}
}
