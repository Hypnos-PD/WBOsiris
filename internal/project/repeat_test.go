package project

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"wbo/internal/ir"
	"wbo/internal/syntax"
)

func TestCompileRepeatScopeAndDynamicCount(t *testing.T) {
	source := validCard(`fanfare {
        choose target from own.field.followers;
        repeat count(own.hand.followers where trait pixie) {
            repeat 2 { choose target from oppo.field.followers; damage target 1; }
        }
        buff target +1/+1;
    }`)
	file, ds := syntax.Parse("12345678.wbo", []byte(source))
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	formatted := syntax.Format(file)
	file, ds = syntax.Parse("12345678.wbo", formatted)
	if len(ds) != 0 || !bytes.Equal(formatted, syntax.Format(file)) {
		t.Fatal("repeat format unstable")
	}
	root := t.TempDir()
	dir := filepath.Join(root, "12345")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "12345678.wbo")
	if err := os.WriteFile(path, formatted, 0600); err != nil {
		t.Fatal(err)
	}
	loaded := LoadWithRoot([]string{path}, true, root)
	if loaded.HasErrors() {
		t.Fatal(loaded.Diagnostics)
	}
	pack, _, err := BuildRuntimePacks(loaded)
	if err != nil {
		t.Fatal(err)
	}
	outer := pack.Cards[0].Abilities[0].Body[1].(ir.RepeatEffect)
	if outer.TimesExpr == nil || outer.Body[0].(ir.RepeatEffect).Times != 2 {
		t.Fatal("repeat count or nested body lost")
	}
}

func TestRejectInvalidRepeatAndEscapedBindings(t *testing.T) {
	for _, body := range []string{
		`repeat { draw 1; }`, `repeat -1 { draw 1; }`, `repeat own.combo + 1 { draw 1; }`, `repeat 2;`,
		`repeat 2 { draw 1; };`, `repeat count(target) { draw 1; }`, `repeat 2 { require t from own.field; }`,
		`repeat 2 { if combo >= 1 { require t from own.field; } }`,
		`repeat 2 { choose inner from own.field; } buff inner +1/+1;`,
		`repeat 2 { summon 1 card 90011110; } buff summoned +1/+1;`,
	} {
		t.Run(body, func(t *testing.T) {
			file, ds := syntax.Parse("12345678.wbo", []byte(validCard("fanfare { "+body+" }")))
			if len(ds) == 0 && !hasErrors(ValidateFile(file)) {
				t.Fatal("accepted invalid repetition")
			}
		})
	}
}
