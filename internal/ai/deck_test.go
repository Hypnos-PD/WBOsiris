package ai

import (
	"path/filepath"
	"testing"

	"wbo/internal/project"
	"wbo/internal/ruleset"
	"wbo/internal/runner"
)

// 随机卡组必须是该赛制下合法的 40 张：这是"用随机卡组压测整个卡池"的前提。
func TestRandomDeckIsLegalInBothFormats(t *testing.T) {
	root := filepath.Join("..", "..")
	loaded := project.LoadWithRoot([]string{filepath.Join(root, "cards")}, true, root)
	if loaded.HasErrors() {
		t.Fatal(loaded.Diagnostics)
	}
	cards, _, err := project.BuildRuntimePacks(loaded)
	if err != nil {
		t.Fatal(err)
	}
	for seed := uint64(1); seed <= 20; seed++ {
		for _, id := range []string{runner.FormatRotation, runner.FormatUnlimited} {
			format, err := runner.FormatByID(cards, id)
			if err != nil {
				t.Fatal(err)
			}
			deck, err := RandomDeck(cards, format, ruleset.NewRNG(seed))
			if err != nil {
				t.Fatalf("seed %d format %s: %v", seed, id, err)
			}
			if len(deck) != 40 {
				t.Fatalf("seed %d format %s: %d cards", seed, id, len(deck))
			}
			if err := runner.ValidateDeckForFormat(cards, deck, format); err != nil {
				t.Fatalf("seed %d format %s: %v", seed, id, err)
			}
		}
	}
}

// 无限制能用的卡包比指定模式多，随机卡组应当能碰到轮换出去的卡。
func TestUnlimitedRandomDecksReachRotatedOutPacks(t *testing.T) {
	root := filepath.Join("..", "..")
	loaded := project.LoadWithRoot([]string{filepath.Join(root, "cards")}, true, root)
	if loaded.HasErrors() {
		t.Fatal(loaded.Diagnostics)
	}
	cards, _, err := project.BuildRuntimePacks(loaded)
	if err != nil {
		t.Fatal(err)
	}
	index := map[int]int{}
	for n := range cards.Cards {
		index[cards.Cards[n].ID] = cards.Cards[n].Meta.Pack
	}
	unlimited, _ := runner.FormatByID(cards, runner.FormatUnlimited)
	rotation, _ := runner.FormatByID(cards, runner.FormatRotation)
	seenRotated := false
	for seed := uint64(1); seed <= 40 && !seenRotated; seed++ {
		deck, err := RandomDeck(cards, unlimited, ruleset.NewRNG(seed))
		if err != nil {
			t.Fatal(err)
		}
		for _, id := range deck {
			if !rotation.AllowsPack(index[id]) {
				seenRotated = true
				break
			}
		}
	}
	if !seenRotated {
		t.Fatal("unlimited random decks never used a rotated-out pack")
	}
	// 反向：指定模式生成的卡组里不允许出现轮换出去的卡包。
	for seed := uint64(1); seed <= 20; seed++ {
		deck, err := RandomDeck(cards, rotation, ruleset.NewRNG(seed))
		if err != nil {
			t.Fatal(err)
		}
		for _, id := range deck {
			if !rotation.AllowsPack(index[id]) {
				t.Fatalf("rotation deck contains rotated-out pack %d", index[id])
			}
		}
	}
}
