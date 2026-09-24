package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"wbo/internal/engine/ir"
	"wbo/internal/engine/project"
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

// 效果程序 token 化：必须能表达"mode 二选一 + 每支的具体效果 + 触发时机"。
// 例子：禁牙的变貌·诺玛格达拉（7 费 5/6）——入场曲【模式】二选一：
// (1) 抽 1 张、回复自己主战者 3 点；(2) 使对手全场随从 -0/-4。进化时重复同一段。
func TestCardEffectTokensPreserveModeStructure(t *testing.T) {
	root := filepath.Join("..", "..")
	loaded := project.LoadWithRoot([]string{filepath.Join(root, "cards")}, false, root)
	if loaded.HasErrors() {
		t.Fatal(loaded.Diagnostics)
	}
	cards, _, err := project.BuildRuntimePacks(loaded)
	if err != nil {
		t.Fatal(err)
	}
	var target *ir.Card
	for n := range cards.Cards {
		if cards.Cards[n].ID == 10944120 {
			target = &cards.Cards[n]
			break
		}
	}
	if target == nil {
		t.Fatal("卡池里没有 10944120")
	}
	tokens := cardEffectTokens(target, maxEffectTokens)

	// 按 (trigger, kind, option) 找 token，并检查数值/字段名与目标
	find := func(trigger, kind string, option int) *envEffectToken {
		for index := range tokens {
			token := &tokens[index]
			if token.Trigger == trigger && token.Kind == kind && token.Option == option {
				return token
			}
		}
		return nil
	}
	numOf := func(token *envEffectToken, key string) (float64, bool) {
		for index, name := range token.NumKeys {
			if name == key {
				return token.Nums[index], true
			}
		}
		return 0, false
	}
	mode := find("fanfare", "mode", 0)
	if mode == nil || mode.Parent < 0 {
		t.Fatalf("入场曲缺少 mode 节点：%+v", mode)
	}
	draw := find("fanfare", "draw", 1)
	if draw == nil {
		t.Fatal("入场曲分支 1 缺少 draw")
	}
	if value, ok := numOf(draw, "count"); !ok || value != 1 {
		t.Fatalf("draw 的 count 应为 1：%+v", draw)
	}
	if draw.Parent != 1 {
		t.Fatalf("draw 的父节点应是 mode（下标 1），实际 %d", draw.Parent)
	}
	heal := find("fanfare", "heal", 1)
	if heal == nil {
		t.Fatal("入场曲分支 1 缺少 heal")
	}
	if value, ok := numOf(heal, "amount"); !ok || value != 3 {
		t.Fatalf("heal 的 amount 应为 3：%+v", heal)
	}
	if heal.TargetKind != "leader" || heal.TargetSide != "own" {
		t.Fatalf("heal 目标应是 own leader：%+v", heal)
	}
	buff := find("fanfare", "buff_stats", 2)
	if buff == nil {
		t.Fatal("入场曲分支 2 缺少 buff_stats")
	}
	if value, ok := numOf(buff, "lifeDelta"); !ok || value != -4 {
		t.Fatalf("buff_stats 的 lifeDelta 应为 -4：%+v", buff)
	}
	if buff.TargetSide != "oppo" || buff.TargetZone != "field" || buff.TargetMember != "follower" {
		t.Fatalf("buff_stats 目标应是 oppo.field.follower：%+v", buff)
	}
	if find("evolve", "buff_stats", 2) == nil {
		t.Fatal("进化时应重复同一段（含 buff_stats 分支 2）")
	}
	// 字符串字段也要带全：draw 的 owner/sourceZone 是判断"从哪抽"的依据
	if len(draw.StrKeys) == 0 || len(draw.Strs) == 0 {
		t.Fatalf("draw 丢了字符串字段：%+v", draw)
	}
	// 分支编号必须真的把两支分开：不能出现"没有 option 的 buff_stats"（那会让模型以为两支都能做）
	for _, token := range tokens {
		if token.Kind == "buff_stats" && token.Option == 0 {
			t.Fatalf("mode 内的效果丢了分支编号：%+v", token)
		}
	}
}

// 离线导出：训练/客户端不必起引擎进程，但必须与运行时**同一套解析器**、同一个卡池指纹。
func TestCardFeaturesExportMatchesRuntimePool(t *testing.T) {
	root := filepath.Join("..", "..")
	out := filepath.Join(t.TempDir(), "card_features.json")
	if code := runCardFeatures([]string{"--out", out, "--source-root", root, filepath.Join(root, "cards")}); code != 0 {
		t.Fatalf("导出失败，退出码 %d", code)
	}
	blob, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	var payload cardFeaturesPayload
	if err := json.Unmarshal(blob, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Schema != "wbo-card-features/1" {
		t.Fatalf("schema 不对：%s", payload.Schema)
	}
	if payload.Count != len(payload.Cards) || payload.Count < 900 {
		t.Fatalf("卡数与列表不一致：count=%d cards=%d", payload.Count, len(payload.Cards))
	}
	// 指纹必须与运行时握手一致：否则特征表与引擎版本可能对不上
	loaded := project.LoadWithRoot([]string{filepath.Join(root, "cards")}, false, root)
	cards, _, err := project.BuildRuntimePacks(loaded)
	if err != nil {
		t.Fatal(err)
	}
	if payload.PoolHash != cardPoolHash(cards) {
		t.Fatalf("卡池指纹不一致：导出 %s vs 运行时 %s", payload.PoolHash, cardPoolHash(cards))
	}
	// 效果 token 必须带结构：每个 buff_stats 的父节点都应是同触发的 mode
	for index := range payload.Cards {
		card := &payload.Cards[index]
		if card.ID != 10944120 {
			continue
		}
		modes := map[string]int{}
		for tokenIndex := range card.Effects {
			token := &card.Effects[tokenIndex]
			if token.Kind == "mode" {
				modes[token.Trigger] = tokenIndex
			}
		}
		if _, ok := modes["fanfare"]; !ok {
			t.Fatalf("缺少入场曲 mode：%+v", modes)
		}
		buffs := 0
		for tokenIndex := range card.Effects {
			token := &card.Effects[tokenIndex]
			if token.Kind != "buff_stats" {
				continue
			}
			buffs++
			if token.Option != 2 {
				t.Fatalf("mode 内的 buff_stats 应带分支号 2：%+v", token)
			}
			parent := modes[token.Trigger]
			if token.Parent != parent {
				t.Fatalf("buff_stats 的父节点应是同触发的 mode（%d），实际 %d", parent, token.Parent)
			}
			if len(token.NumKeys) == 0 || len(token.Nums) == 0 {
				t.Fatalf("buff_stats 丢了数值：%+v", token)
			}
		}
		if buffs < 2 { // 入场曲 + 进化时各一次
			t.Fatalf("buff_stats 数量不对：%d", buffs)
		}
	}
}
