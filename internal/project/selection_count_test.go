package project

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"wbo/internal/ir"
	"wbo/internal/syntax"
)

func TestCompileSelectionCountAndFilter(t *testing.T) {
	source := validCard(`fanfare {
        choose targets from own.field.followers other where trait pixie count 2;
        buff targets +1/+1;
        random enemies from oppo.field.followers count 3;
        damage enemies 2;
    }`)
	file, ds := syntax.Parse("12345678.wbo", []byte(source))
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	formatted := syntax.Format(file)
	file, ds = syntax.Parse("12345678.wbo", formatted)
	if len(ds) != 0 || !bytes.Equal(formatted, syntax.Format(file)) {
		t.Fatal("unstable selection formatting")
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
	body := pack.Cards[0].Abilities[0].Body
	chosen, random := body[0].(ir.SelectionEffect), body[2].(ir.SelectionEffect)
	if chosen.Count != 2 || random.Count != 3 || random.Kind != "random_choose" {
		t.Fatal("lost selection count")
	}
	filter := chosen.Source.(ir.FilterRef)
	if _, ok := filter.Source.(ir.ExcludeRef); !ok {
		t.Fatal("lost other filter")
	}
}

func TestRejectInvalidSelectionCounts(t *testing.T) {
	for _, suffix := range []string{"count", "count 0", "count -1", "count 65536", "count own.combo", "count 2 count 3", "count 2 where trait pixie"} {
		file, ds := syntax.Parse("12345678.wbo", []byte(validCard("fanfare { choose targets from own.field "+suffix+"; }")))
		if len(ds) == 0 && !hasErrors(ValidateFile(file)) {
			t.Fatalf("accepted %s", suffix)
		}
	}
}
