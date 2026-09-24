package project

import (
	"os"
	"path/filepath"
	"testing"

	"wbo/internal/engine/ir"
	"wbo/internal/engine/syntax"
)

// S-70：`summon random N card A or card B [...] [for own|oppo]` 从几种指定卡中随机召唤。
func TestSummonPoolCompiles(t *testing.T) {
	pack, ds := compilePoolCard(t, `fanfare {
		summon random 1 card 12345678 or card 90099910;
	}`)
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	effect, ok := pack.Cards[0].Abilities[0].Body[0].(ir.SummonPoolEffect)
	if !ok || effect.Kind != "summon_random_pool" || effect.Count != 1 || effect.Owner != "own" {
		t.Fatalf("pool summon did not compile: %#v", pack.Cards[0].Abilities[0].Body[0])
	}
	if len(effect.Pool) != 2 || effect.Pool[0] != 12345678 || effect.Pool[1] != 90099910 {
		t.Fatalf("pool lost its card list: %#v", effect.Pool)
	}
	enemy, ds := compilePoolCard(t, `fanfare {
		summon random 1 card 12345678 or card 90099910 for oppo;
	}`)
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	if opponent := enemy.Cards[0].Abilities[0].Body[0].(ir.SummonPoolEffect); opponent.Owner != "oppo" {
		t.Fatalf("pool summon lost its owner: %#v", opponent)
	}
	if _, ds := compile(t, validCard(`fanfare { summon random 1 card 12345678; }`)); len(ds) == 0 {
		t.Fatal("a single-card pool was accepted")
	}
}

// compilePoolCard 额外写入一张 90099910 供随机池引用，然后连同它一起加载。
func compilePoolCard(t *testing.T, effect string) (*ir.CardPack, []string) {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "12345")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	pool := `wbo 0.1.0; card 90099910 { type follower; cost 1; stats 1/1; effect { } meta { pack 12345; class neutral; rarity bronze; } locale chs { name "x"; text ""; } locale eng { name "x"; text ""; } locale jpn { name "x"; text ""; } locale kor { name "x"; text ""; } locale cht { name "x"; text ""; } }`
	main := filepath.Join(dir, "12345678.wbo")
	other := filepath.Join(dir, "90099910.wbo")
	for path, source := range map[string]string{main: validCard(effect), other: pool} {
		file, ds := syntax.Parse(path, []byte(source))
		if len(ds) != 0 {
			return nil, []string{ds[0].Message}
		}
		if err := os.WriteFile(path, []byte(syntax.Format(file)), 0600); err != nil {
			t.Fatal(err)
		}
	}
	loaded := LoadWithRoot([]string{dir}, true, root)
	if loaded.HasErrors() {
		messages := []string{}
		for _, diagnostic := range loaded.Diagnostics {
			messages = append(messages, diagnostic.Message)
		}
		return nil, messages
	}
	pack, _, err := BuildRuntimePacks(loaded)
	if err != nil {
		return nil, []string{err.Error()}
	}
	return pack, nil
}
