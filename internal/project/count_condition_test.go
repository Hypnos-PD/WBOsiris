package project

import (
	"testing"

	"wbo/internal/ir"
)

// `if count(集合 [where …]) 比较 整数`：条件可以直接比较集合里满足筛选的对象数量。
func TestCountConditionCompilesIntoItsOwnKind(t *testing.T) {
	body := compiledFanfare(t, `fanfare {
		if count(own.field.followers where form super_evolved) >= 1 {
			draw 1;
		}
		if count(own.field.amulets) >= 2 {
			draw 2;
		}
	}`)
	if len(body) != 2 {
		t.Fatalf("expected two if effects, got %d", len(body))
	}
	first, ok := body[0].(ir.IfEffect)
	if !ok {
		t.Fatalf("unexpected effect: %T", body[0])
	}
	condition, ok := first.Condition.(ir.CountCondition)
	if !ok || condition.Op != "ge" || condition.Right != 1 {
		t.Fatalf("count condition not preserved: %+v", first.Condition)
	}
	if _, ok := condition.Source.(ir.FilterRef); !ok {
		t.Fatalf("count condition must keep the filter: %+v", condition.Source)
	}
	second := body[1].(ir.IfEffect).Condition.(ir.CountCondition)
	if _, filtered := second.Source.(ir.FilterRef); filtered {
		t.Fatalf("unfiltered count must stay unfiltered: %+v", second.Source)
	}
}

func TestCountConditionRejectsBadShapes(t *testing.T) {
	for _, line := range []string{
		"if count(own.field.amulets) { draw 1; }",
		"if count(own.field.amulets) >= ;",
		"if count() >= 1 { draw 1; }",
		"if count(own.field.amulets) >= 1 extra { draw 1; }",
	} {
		if _, ds := compile(t, validCard("fanfare { "+line+" }")); len(ds) == 0 {
			t.Fatalf("%q must not compile", line)
		}
	}
}
