package main

import (
	"path/filepath"
	"testing"

	"wbo/internal/project"
)

// 卡池要给出**具体**的结构化卡面特征：稀有度/费用/基础身材/效果语义标签。
// 这些标签是训练侧做结构特征与卡牌嵌入初始化的依据，缺了就只能靠"费用+身材"猜。
func TestCardPoolExposesStructuredFeatures(t *testing.T) {
	root := filepath.Join("..", "..")
	loaded := project.LoadWithRoot([]string{filepath.Join(root, "cards")}, false, root)
	if loaded.HasErrors() {
		t.Fatal(loaded.Diagnostics)
	}
	cards, _, err := project.BuildRuntimePacks(loaded)
	if err != nil {
		t.Fatal(err)
	}
	pool := cardPool(cards)
	if len(pool) == 0 {
		t.Fatal("卡池为空")
	}
	byID := map[int]envCardInfo{}
	tagKinds := map[string]int{}
	for _, item := range pool {
		byID[item.ID] = item
		if item.Rarity == "" {
			t.Fatalf("卡 %d 缺稀有度", item.ID)
		}
		for _, tag := range item.Tags {
			tagKinds[tag]++
		}
	}
	// 多情的唤灵师：4 费 5/4，进化时召唤、超进化给关键词
	card, ok := byID[10052120]
	if !ok {
		t.Fatal("卡池里没有 10052120")
	}
	if card.Cost != 4 || card.Attack != 5 || card.Life != 4 {
		t.Fatalf("10052120 基础数据不对：cost=%d %d/%d", card.Cost, card.Attack, card.Life)
	}
	want := map[string]bool{"summon": false, "evolve_effect": false, "superevolve_effect": false, "add_keyword": false}
	for _, tag := range card.Tags {
		if _, exists := want[tag]; exists {
			want[tag] = true
		}
	}
	for tag, found := range want {
		if !found {
			t.Fatalf("10052120 缺标签 %s（实际 %v）", tag, card.Tags)
		}
	}
	// 标签词表必须够丰富：真实卡池里至少有 30 种标签
	if len(tagKinds) < 30 {
		t.Fatalf("标签种类只有 %d 种，太贫瘠", len(tagKinds))
	}
	for _, tag := range []string{"damage", "destroy", "draw", "fanfare"} {
		if tagKinds[tag] == 0 {
			t.Fatalf("标签 %s 一张卡都没有，抽取可能有 bug", tag)
		}
	}
}
