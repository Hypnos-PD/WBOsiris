package project

import (
	"testing"

	"wbo/internal/ir"
)

// 筛选器新增 `damaged` 与 `attack 比较 整数`；`add card … to hand` 产出 added 绑定。
func TestDamagedAndAttackFiltersCompile(t *testing.T) {
	body := compiledFanfare(t, `fanfare {
		choose target from oppo.field.followers where attack <= 4;
		banish target;
		destroy own.field.followers where damaged;
		add 1 card 12345678 to hand;
		buff added +3/+0;
	}`)
	if len(body) != 5 {
		t.Fatalf("effects = %d", len(body))
	}
	choose, ok := body[0].(ir.SelectionEffect)
	if !ok {
		t.Fatalf("unexpected first effect: %T", body[0])
	}
	filter, ok := choose.Source.(ir.FilterRef)
	if !ok {
		t.Fatalf("choose lost its filter: %#v", choose.Source)
	}
	predicate, ok := filter.Predicate.(ir.FieldPredicate)
	if !ok || predicate.Kind != "compare" || predicate.Field != "attack" || predicate.Op != "le" || predicate.Value != 4 {
		t.Fatalf("attack filter not preserved: %#v", filter.Predicate)
	}
	destroy := body[2].(ir.TargetEffect)
	if damaged, ok := destroy.Predicate.(ir.FieldPredicate); !ok || damaged.Kind != "is_damaged" {
		t.Fatalf("damaged filter not preserved: %#v", destroy.Predicate)
	}
	add := body[3].(ir.CardEffect)
	if add.Output != "added" {
		t.Fatalf("added binding missing: %#v", add)
	}
	buff := body[4].(ir.TargetEffect)
	if ref, ok := buff.Target.(ir.BindingRef); !ok || ref.Name != "added" {
		t.Fatalf("buff did not target the added card: %#v", buff.Target)
	}
}

func TestAddedBindingOnlyExposedByAdd(t *testing.T) {
	if _, ds := compile(t, validCard("fanfare { buff added +1/+1; }")); len(ds) == 0 {
		t.Fatal("added must not be visible without an add statement")
	}
	for _, line := range []string{
		"destroy own.field.followers where attack;",
		"destroy own.field.followers where damaged <= 1;",
	} {
		if _, ds := compile(t, validCard("fanfare { "+line+" }")); len(ds) == 0 {
			t.Fatalf("%q must not compile", line)
		}
	}
}
