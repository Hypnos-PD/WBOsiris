package project

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEarthSigilGainRequiresTokenDependency(t *testing.T) {
	for _, body := range []string{"fanfare { add 1 earthsigil; }", "fanfare { if overflow { add 4 earthsigil; } }", "fanfare { add 0 earthsigil; }"} {
		root := t.TempDir()
		dir := filepath.Join(root, "12345")
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, "12345678.wbo")
		if err := os.WriteFile(path, []byte(validCard(body)), 0600); err != nil {
			t.Fatal(err)
		}
		loaded := LoadWithRoot([]string{path}, true, root)
		_, missing := loaded.Unresolved["90031210"]
		wantMissing := body != "fanfare { add 0 earthsigil; }"
		if missing != wantMissing || loaded.HasErrors() != wantMissing {
			t.Fatalf("body=%s diagnostics=%v unresolved=%v", body, loaded.Diagnostics, loaded.Unresolved)
		}
	}
}
