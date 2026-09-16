package project

import (
	"testing"

	"wbo/internal/ir"
)

// remove lastwords from … / remove all abilities from …：让实例失去触发能力。
func TestAbilityRemovalCompilesIntoItsOwnKind(t *testing.T) {
	body := compiledFanfare(t, `fanfare {
		summon 1 card 12345678;
		remove lastwords from summoned;
		choose targets from oppo.field.followers count 2;
		remove all abilities from targets;
	}`)
	if len(body) != 4 {
		t.Fatalf("expected four effects, got %d", len(body))
	}
	lastwords, ok := body[1].(ir.TargetEffect)
	if !ok || lastwords.Kind != "remove_ability" || lastwords.Keyword != "lastwords" {
		t.Fatalf("last words removal not preserved: %#v", body[1])
	}
	if _, ok := lastwords.Target.(ir.BindingRef); !ok {
		t.Fatalf("removal target must stay a binding: %#v", lastwords.Target)
	}
	all, ok := body[3].(ir.TargetEffect)
	if !ok || all.Kind != "remove_ability" || all.Keyword != "all" {
		t.Fatalf("full removal not preserved: %#v", body[3])
	}
}

func TestAbilityRemovalRejectsBadShapes(t *testing.T) {
	for _, line := range []string{
		"remove lastwords from;",
		"remove lastwords summoned;",
		"remove abilities from targets;",
		"remove all abilities targets;",
		"remove fanfare from self;",
		"add lastwords to self;",
		"remove lastwords from self until own turn ends;",
	} {
		if _, ds := compile(t, validCard("fanfare { "+line+" }")); len(ds) == 0 {
			t.Fatalf("%q must not compile", line)
		}
	}
}
