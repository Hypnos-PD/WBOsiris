package project

import (
	"os"
	"path/filepath"
	"testing"
	"wbo/internal/ir"
	"wbo/internal/syntax"
)

func TestGrantCompilesIndependentRecipientScope(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "12345")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "12345678.wbo")
	source := validCard(`fanfare {
        choose recipient from own.hand;
        grant recipient {
            label eng "A final strike";
            lastwords { damage oppo.leader self.attack; }
        }
    }`)
	if err := os.WriteFile(path, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	loaded := LoadWithRoot([]string{path}, true, root)
	pack, _, err := BuildRuntimePacks(loaded)
	if err != nil {
		t.Fatal(err, loaded.Diagnostics)
	}
	data, err := ir.EncodeCardPack(*pack)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := ir.DecodeCardPack(data)
	if err != nil {
		t.Fatal(err)
	}
	grant := decoded.Cards[0].Abilities[0].Body[1].(ir.GrantEffect)
	if grant.Kind != "grant_ability" || grant.Target.(ir.BindingRef).Name != "recipient" || grant.Labels["eng"] != "A final strike" || ir.TriggerKind(grant.Ability.Trigger) != "lastwords" || grant.Ability.Body[0].(ir.TargetEffect).AmountExpr.(*ir.Scalar).Field != "attack" {
		t.Fatal(grant)
	}
}

func TestGrantRejectsCapturedBindingsAndUnsupportedTriggers(t *testing.T) {
	for _, body := range []string{
		`choose target from own.field; grant self { lastwords { destroy target; } }`,
		`grant missing { lastwords { draw 1; } }`,
		`grant self { lastwords; }`,
		`grant self { fanfare { draw 1; } }`,
		`grant self { when own follower summoned { draw 1; } }`,
		`grant self { lastwords { require target from own.hand; } }`,
		`grant self { lastwords { add 1 counter x; } }`,
		`grant self { lastwords { draw 1; } lastwords { draw 1; } }`,
		`grant self { label eng ""; lastwords { draw 1; } }`,
		`grant self { label eng "one"; label eng "two"; lastwords { draw 1; } }`,
	} {
		f, ds := syntax.Parse("12345678.wbo", []byte(validCard("fanfare { "+body+" }")))
		if len(ds) == 0 && !hasErrors(ValidateFile(f)) {
			t.Fatal("accepted invalid grant", body)
		}
	}
}
