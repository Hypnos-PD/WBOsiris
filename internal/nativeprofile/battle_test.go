package nativeprofile

import (
	"path/filepath"
	"testing"
)

// firstDeckID 取第一副牌组（初始档案里七个职业各一副 40 张的）。
func firstDeckID(t *testing.T, profile *Profile) int64 {
	t.Helper()
	answer := call(t, profile, "/Deck/getList", map[string]any{})
	lists, _ := answer["deck_lists"].([]any)
	if len(lists) == 0 {
		t.Fatal("没有牌组列表")
	}
	first, _ := lists[0].(map[string]any)
	decks, _ := first["decks"].([]any)
	if len(decks) == 0 {
		t.Fatal("没有牌组")
	}
	deck, _ := decks[0].(map[string]any)
	id, err := asInt(deck["deck_id"])
	if err != nil {
		t.Fatalf("牌组 id 读不出来：%v", err)
	}
	return id
}

// 练习对局的响应形状：客户端拿 battle_id 去连 battle_url，拿 battle_info 决定先后手
// 与 stage，拿 battle_users 画两边的头像与职业。少任何一个都会卡在"准备中"。
func TestPracticeBattleStartShapesResponse(t *testing.T) {
	profile := newProfile(t)
	deckID := firstDeckID(t, profile)
	answer := call(t, profile, "/Practice/battleStart", map[string]any{
		"deck_id":            deckID,
		"deck_format":        int64(1),
		"practice_type":      int64(2),
		"target_id":          deckID,
		"target_deck_format": int64(1),
		"battle_order":       int64(0),
		"stage_select":       []any{int64(108)},
	})

	if answer["battle_url"] != DefaultBattleURL {
		t.Fatalf("battle_url 应该是 %s，实际 %v", DefaultBattleURL, answer["battle_url"])
	}
	battleID, err := asInt(answer["battle_id"])
	if err != nil || battleID == 0 {
		t.Fatalf("battle_id 应该是非零整数，实际 %v", answer["battle_id"])
	}
	info, ok := answer["battle_info"].(map[string]any)
	if !ok {
		t.Fatalf("battle_info 应该是对象，实际 %T", answer["battle_info"])
	}
	first, err := asInt(info["first_turn_client_id"])
	if err != nil || (first != 1 && first != 2) {
		t.Fatalf("先手应当是 1 或 2，实际 %v", info["first_turn_client_id"])
	}
	if stage, _ := asInt(info["stage_id"]); stage != practiceStageID {
		t.Fatalf("stage_id 应当是 %d，实际 %v", practiceStageID, info["stage_id"])
	}
	users, ok := answer["battle_users"].([]any)
	if !ok || len(users) != 2 {
		t.Fatalf("battle_users 应该是两条，实际 %v", answer["battle_users"])
	}
	for side, raw := range users {
		user, _ := raw.(map[string]any)
		if id, _ := asInt(user["client_id"]); id != int64(side+1) {
			t.Fatalf("第 %d 条的 client_id 应当是 %d，实际 %v", side, side+1, user["client_id"])
		}
		if name, _ := user["user_name"].(string); name != battleUserName(side) {
			t.Fatalf("第 %d 条的名字应当是 %s，实际 %v", side, battleUserName(side), user["user_name"])
		}
		if _, err := asInt(user["class_id"]); err != nil {
			t.Fatalf("第 %d 条缺 class_id：%v", side, err)
		}
	}
}

// 快照要在响应之前落盘，而且发牌是真的发了：两边的起手各 4 张、牌库各 36 张。
func TestPracticeBattleStartWritesSnapshot(t *testing.T) {
	profile := newProfile(t)
	deckID := firstDeckID(t, profile)
	call(t, profile, "/Practice/battleStart", map[string]any{
		"deck_id":            deckID,
		"deck_format":        int64(1),
		"practice_type":      int64(2),
		"target_id":          deckID,
		"target_deck_format": int64(1),
		"battle_order":       int64(2),
	})

	snapshot, err := profile.LoadBattleSnapshot()
	if err != nil {
		t.Fatalf("应当读得到快照：%v", err)
	}
	if snapshot.First != 2 {
		t.Fatalf("battle_order=2 表示对手先手，实际 first=%d", snapshot.First)
	}
	if len(snapshot.Hands) != 2 || len(snapshot.DrawPiles) != 2 {
		t.Fatalf("两边都应当有起手与牌库，实际 %d / %d", len(snapshot.Hands), len(snapshot.DrawPiles))
	}
	for side := 0; side < 2; side++ {
		if len(snapshot.Hands[side]) != handSize {
			t.Fatalf("第 %d 方起手应当是 %d 张，实际 %d", side, handSize, len(snapshot.Hands[side]))
		}
		if len(snapshot.Hands[side])+len(snapshot.DrawPiles[side]) != 40 {
			t.Fatalf("第 %d 方起手加牌库应当是 40 张", side)
		}
	}
	if snapshot.Stage != practiceStageID {
		t.Fatalf("stage 应当是 %d，实际 %d", practiceStageID, snapshot.Stage)
	}
}

// 用官方预设牌组的练习模式我们没有那份数据，猜一个只会给出看起来对、其实不对的对局。
func TestPracticeBattleStartRejectsUnknownPracticeType(t *testing.T) {
	profile := newProfile(t)
	deckID := firstDeckID(t, profile)
	if _, err := profile.Handle("/Practice/battleStart", map[string]any{
		"deck_id":            deckID,
		"deck_format":        int64(1),
		"practice_type":      int64(1),
		"target_id":          deckID,
		"target_deck_format": int64(1),
		"battle_order":       int64(0),
	}); err == nil {
		t.Fatal("practice_type=1（官方预设牌组）应当被拒绝")
	}
	if _, err := filepath.Glob(filepath.Join(filepath.Dir(profile.store.path), "battle", "*")); err != nil {
		t.Fatal(err)
	}
}
