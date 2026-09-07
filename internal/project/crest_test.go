package project

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"wbo/internal/ir"
	"wbo/internal/syntax"
)

func crestCardSource(body string) string {
	locales := ""
	for _, code := range []string{"chs", "eng", "jpn", "kor", "cht"} {
		locales += ` locale ` + code + ` { name "Crest"; text "Crest rules"; }`
	}
	return strings.Replace(validCard(`fanfare { gain own crest 12345678; }`), " meta {", " crest { "+body+locales+" } meta {", 1)
}

func TestCrestDefinitionRoundTripAndReferenceValidation(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "12345")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "12345678.wbo")
	source := crestCardSource(`counter x 1; countdown 2; lastwords { summon 1 card 12345678; } when own follower summoned { add storm to summoned; }`)
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
	c := decoded.Cards[0]
	if c.Crest == nil || c.Crest.Countdown != 2 || len(c.Crest.Abilities) != 2 || c.Crest.Counters["x"] != 1 || c.Abilities[0].ID == c.Crest.Abilities[0].ID {
		t.Fatal("crest definition or stable scope lost")
	}
	for _, invalid := range []string{
		validCard(`fanfare { gain own crest 12345678; }`),
		strings.Replace(source, "gain own crest 12345678", "gain own crest 99999999", 1),
	} {
		if err := os.WriteFile(path, []byte(invalid), 0600); err != nil {
			t.Fatal(err)
		}
		l := LoadWithRoot([]string{path}, true, root)
		if !l.HasErrors() {
			t.Fatal("accepted missing crest reference")
		}
	}
}

func TestCrestRejectsInapplicableTriggersAndScopes(t *testing.T) {
	for _, body := range []string{
		`fanfare { draw 1; }`, `evolve { draw 1; }`, `engage 0 { draw 1; }`,
		`when self summoned { draw 1; }`, `when own turn ends while self in hand { draw 1; }`,
		`storm;`, `draw 1;`, `countdown 0;`, `countdown 2; countdown 3;`,
	} {
		f, ds := syntax.Parse("12345678.wbo", []byte(crestCardSource(body)))
		if len(ds) == 0 && !hasErrors(ValidateFile(f)) {
			t.Fatal("accepted invalid crest", body)
		}
	}
}
