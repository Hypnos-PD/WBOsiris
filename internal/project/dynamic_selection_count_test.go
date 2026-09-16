package project

import (
	"testing"

	"wbo/internal/ir"
)

// `random 绑定 from 集合 count X`：X 可以是数值表达式（纹章数、集合计数…），0 表示不选。
func TestDynamicSelectionCount(t *testing.T) {
	pack, ds := compile(t, validCard(`
		fanfare {
			random victims from oppo.field.followers count count(own.field other);
			destroy victims;
			random few from oppo.field.followers count 2;
		}`))
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	body := pack.Cards[0].Abilities[0].Body
	dynamic := body[0].(ir.SelectionEffect)
	if dynamic.CountExpr == nil || dynamic.Count != 0 || !dynamic.CountIsDynamic() {
		t.Fatalf("dynamic count not preserved: %#v", dynamic)
	}
	if _, ok := dynamic.CountExpr.(*ir.CountExpr); !ok {
		t.Fatalf("count expression lost: %#v", dynamic.CountExpr)
	}
	constant := body[2].(ir.SelectionEffect)
	if constant.CountExpr != nil || constant.Count != 2 || constant.CountIsDynamic() {
		t.Fatalf("constant count changed: %#v", constant)
	}
}

func TestDynamicSelectionCountRejectsBadShapes(t *testing.T) {
	for _, line := range []string{
		"random few from oppo.field.followers count;",
		"random few from oppo.field.followers count keyword;",
	} {
		if _, ds := compile(t, validCard("fanfare { "+line+" }")); len(ds) == 0 {
			t.Fatalf("%q must not compile", line)
		}
	}
}
