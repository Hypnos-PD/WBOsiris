package project

import (
	"testing"

	"wbo/internal/engine/ir"
)

// `other [绑定]`：默认排除来源自身，给出绑定名时改为排除该绑定指向的实例
// （例如"非交战对手的随从"）。
func TestOtherCanExcludeABinding(t *testing.T) {
	pack, ds := compile(t, validCard(`
		attack {
			random victim from oppo.field.followers other opponent;
			destroy victim;
		}
		fanfare {
			buff own.field.followers other +1/+1;
		}`))
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	attackBody := pack.Cards[0].Abilities[0].Body
	choose := attackBody[0].(ir.SelectionEffect)
	exclude, ok := choose.Source.(ir.ExcludeRef)
	if !ok {
		t.Fatalf("other opponent did not compile into an exclusion: %#v", choose.Source)
	}
	if binding, ok := exclude.Value.(ir.BindingRef); !ok || binding.Name != "opponent" {
		t.Fatalf("exclusion must point at the binding: %#v", exclude.Value)
	}
	if zone, ok := exclude.Source.(ir.ZoneRef); !ok || zone.Side != "oppo" {
		t.Fatalf("exclusion lost its source set: %#v", exclude.Source)
	}
	fanfare := pack.Cards[0].Abilities[1].Body
	plain := fanfare[0].(ir.TargetEffect)
	if plainExclude, ok := plain.Target.(ir.ExcludeRef); !ok {
		t.Fatalf("bare other stopped excluding self: %#v", plain.Target)
	} else if _, ok := plainExclude.Value.(ir.SelfRef); !ok {
		t.Fatalf("bare other must exclude self: %#v", plainExclude.Value)
	}
}
