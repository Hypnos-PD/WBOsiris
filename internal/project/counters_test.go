package project

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"wbo/internal/ir"
	"wbo/internal/syntax"
)

func TestCounterCompileAndRoundTrip(t *testing.T) {
	source := validCard(`counter x 2; spellboost { add 1 counter x; }
        fanfare { if self.counter.x >= 3 { repeat self.counter.x { damage oppo.leader self.counter.x; } }
        buff self +self.counter.x/-self.counter.x; }`)
	file, ds := syntax.Parse("12345678.wbo", []byte(source))
	if len(ds) != 0 || hasErrors(ValidateFile(file)) {
		t.Fatal(ds, ValidateFile(file))
	}
	formatted := syntax.Format(file)
	again, ds := syntax.Parse("12345678.wbo", formatted)
	if len(ds) != 0 || !bytes.Equal(formatted, syntax.Format(again)) {
		t.Fatal("unstable counter formatting")
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
	pack, _, err := BuildRuntimePacks(loaded)
	if err != nil {
		t.Fatal(err, loaded.Diagnostics)
	}
	if pack.Cards[0].Counters["x"] != 2 || len(pack.Cards[0].PlayEffects) != 0 {
		t.Fatal("counter declaration executed or lost")
	}
	data, err := ir.EncodeCardPack(*pack)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := ir.DecodeCardPack(data)
	if err != nil {
		t.Fatal(err)
	}
	data2, err := ir.EncodeCardPack(*decoded)
	if err != nil || !bytes.Equal(data, data2) {
		t.Fatal("counter IR roundtrip differs", err)
	}
}

func TestCounterRejectsInvalidDeclarationsAndReferences(t *testing.T) {
	for _, body := range []string{
		`counter X 0;`, `counter _x 0;`, `counter x -1;`, `counter x 2147483648;`,
		`counter x 1; counter x 2;`, `counter x 1 {}`, `fanfare { counter x 1; }`,
		`counter x 1; fanfare { add 1 counter y; }`, `counter x 1; fanfare { damage oppo.leader self.counter.y; }`,
		`counter x 1; fanfare { if self.counter.y >= 1 { draw 1; } }`,
		`counter x 1; fanfare { repeat self.counter.y { draw 1; } }`,
		`counter x 1; spellboost { add -1 counter x; }`, `counter x 1; spellboost { add 2147483648 counter x; }`,
	} {
		t.Run(body, func(t *testing.T) {
			file, ds := syntax.Parse("12345678.wbo", []byte(validCard(body)))
			if len(ds) == 0 && !hasErrors(ValidateFile(file)) {
				t.Fatal("accepted invalid counter")
			}
		})
	}
}

func TestScenarioCounterNamesAndOverrides(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "12345")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "12345678.wbo"), []byte(validCard("counter x 2;")), 0600); err != nil {
		t.Fatal(err)
	}
	base := `wbotest 0.1.0; use cards "12345";
scenario "counter" { seed 1; state { player own { pp 1/1; hand { follower source = 12345678 { counter x 7; } } } }
action { play source; } expect { legal; source.counter.x == 7; } }`
	for _, change := range []struct{ from, to string }{
		{"", ""}, {"counter x 7;", "counter y 7;"}, {"counter x 7;", "counter x 7; counter x 8;"},
		{"counter x 7;", "counter x -1;"}, {"counter x 7;", "counter x 2147483648;"},
		{"source.counter.x", "source.counter.y"},
	} {
		path := filepath.Join(root, "counter.wbotest")
		source := base
		if change.from != "" {
			source = strings.Replace(source, change.from, change.to, 1)
		}
		if err := os.WriteFile(path, []byte(source), 0600); err != nil {
			t.Fatal(err)
		}
		loaded := LoadWithRoot([]string{dir, path}, true, root)
		if change.from == "" {
			if loaded.HasErrors() {
				t.Fatal(loaded.Diagnostics)
			}
			_, pack, err := BuildRuntimePacks(loaded)
			if err != nil {
				t.Fatal(err)
			}
			if pack.Scenarios[0].InitialState.Players["own"].Zones["hand"][0].Overrides.Counters["x"] != 7 {
				t.Fatal("test override lost")
			}
		} else if !loaded.HasErrors() {
			t.Fatal("accepted invalid test counter", change)
		}
	}
}
