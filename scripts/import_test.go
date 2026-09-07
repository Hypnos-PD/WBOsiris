package scripts_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"wbo/internal/project"
)

func TestCardImportPreservesTraits(t *testing.T) {
	for _, program := range []string{"bash", "jq"} {
		if _, err := exec.LookPath(program); err != nil {
			t.Skipf("card importer requires %s", program)
		}
	}
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	temp := t.TempDir()
	source, output := filepath.Join(temp, "source"), filepath.Join(temp, "cards")
	if err := os.MkdirAll(filepath.Join(source, "data"), 0700); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		tribe int
		trait string
	}{
		{0, ""}, {2, "officer"}, {3, "luminous"}, {4, "levin"}, {5, "pixie"},
		{6, "departed"}, {8, "earthsigil"}, {11, "mysteria"}, {12, "golem"},
		{13, "shikigami"}, {14, "artifact"}, {15, "puppetry"}, {17, "marine"},
		{18, "loot"}, {19, "encroacher"}, {20, "anathema"},
	}
	var rows []map[string]any
	for n, tc := range cases {
		rows = append(rows, map[string]any{
			"card_id": 10201110 + n*10, "card_set_id": 10002, "type": 1,
			"cost": 1, "atk": 1, "life": 2, "class": 0, "rarity": 1, "tribe": tc.tribe,
			"name_chs": "test", "name_eng": "test", "name_jpn": "test", "name_kor": "test", "name_cht": "test",
			"skill_texts": []any{},
		})
	}
	writeRows := func(rows []map[string]any) {
		t.Helper()
		data, err := json.Marshal(rows)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(source, "data", "cards.json"), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	run := func() ([]byte, error) {
		return exec.Command("bash", filepath.Join(root, "scripts", "import_wbarts_packs.sh"), source, output, "10002").CombinedOutput()
	}
	writeRows(rows)
	if log, err := run(); err != nil {
		t.Fatalf("import failed: %v\n%s", err, log)
	}
	loaded := project.LoadWithRoot([]string{output}, true, temp)
	if loaded.HasErrors() {
		t.Fatal(loaded.Diagnostics)
	}
	pack, _, err := project.BuildRuntimePacks(loaded)
	if err != nil {
		t.Fatal(err)
	}
	if len(pack.Cards) != len(cases) {
		t.Fatalf("imported %d cards, want %d", len(pack.Cards), len(cases))
	}
	for n, card := range pack.Cards {
		want := cases[n].trait
		if want == "" && len(card.Traits) != 0 || want != "" && (len(card.Traits) != 1 || card.Traits[0] != want) {
			t.Errorf("tribe %d imported as %v, want %q", cases[n].tribe, card.Traits, want)
		}
	}
	// Re-importing must preserve authored effects and local edits.
	firstPath := filepath.Join(output, "10002", "10201110.wbo")
	marker := []byte("authored content must remain untouched\n")
	if err := os.WriteFile(firstPath, marker, 0600); err != nil {
		t.Fatal(err)
	}
	if log, err := run(); err != nil {
		t.Fatalf("repeat import failed: %v\n%s", err, log)
	}
	got, err := os.ReadFile(firstPath)
	if err != nil || string(got) != string(marker) {
		t.Fatal("import overwrote an existing definition")
	}
	for _, field := range []string{"type", "tribe"} {
		t.Run("unknown_"+field, func(t *testing.T) {
			bad := map[string]any{}
			for k, v := range rows[0] {
				bad[k] = v
			}
			bad["card_id"], bad[field] = 10209990, 999
			writeRows([]map[string]any{bad})
			if _, err := run(); err == nil {
				t.Fatalf("unknown %s was silently imported", field)
			}
			if _, err := os.Stat(filepath.Join(output, "10002", "10209990.wbo")); !os.IsNotExist(err) {
				t.Fatalf("unknown %s left a card definition", field)
			}
		})
	}
}
