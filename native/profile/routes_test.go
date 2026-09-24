package nativeprofile

import (
	"path/filepath"
	"testing"
)

func newProfile(t *testing.T) *Profile {
	t.Helper()
	profile, err := LoadWithState(filepath.Join(t.TempDir(), "profile.json"))
	if err != nil {
		t.Fatalf("加载档案层失败: %v", err)
	}
	return profile
}

func call(t *testing.T, profile *Profile, path string, request map[string]any) map[string]any {
	t.Helper()
	answer, err := profile.Handle(path, request)
	if err != nil {
		t.Fatalf("%s 应该成功，实际 %v", path, err)
	}
	if answer == nil {
		t.Fatalf("%s 应该有响应", path)
	}
	return answer
}

// 初始档案：七个职业各一副 40 张的基础牌组。牌组必须真的是 40 张、error_state 为 0，
// 否则客户端打开牌组编辑时会显示"张数不足"。
func TestSeedDecksAreComplete(t *testing.T) {
	profile := newProfile(t)
	answer := call(t, profile, "/Deck/getList", map[string]any{})
	lists, ok := answer["deck_lists"].([]any)
	if !ok || len(lists) != 4 {
		t.Fatalf("应该有 4 个格式的列表，实际 %v", answer["deck_lists"])
	}
	first, _ := lists[0].(map[string]any)
	decks, _ := first["decks"].([]any)
	if len(decks) != 7 {
		t.Fatalf("第一个格式应该有 7 副牌，实际 %d", len(decks))
	}
	for _, raw := range decks {
		deck, _ := raw.(map[string]any)
		cards, _ := deck["cards"].([]any)
		total := int64(0)
		for _, cardRaw := range cards {
			card, _ := cardRaw.(map[string]any)
			count, err := asInt(card["num"])
			if err != nil {
				t.Fatal(err)
			}
			total += count
		}
		if total != 40 {
			t.Errorf("牌组 %v 只有 %d 张", deck["name"], total)
		}
		if state, _ := asInt(deck["error_state"]); state != 0 {
			t.Errorf("牌组 %v 的 error_state 应该是 0，实际 %d", deck["name"], state)
		}
	}
}

// 收藏列表要给全（客户端据此渲染卡牌一览），而且每个条目都要是整数类型——
// 编成浮点客户端解析会失败。
func TestCardListPayload(t *testing.T) {
	profile := newProfile(t)
	answer := call(t, profile, "/Card/getList", map[string]any{})
	cards, _ := answer["cards"].([]any)
	if len(cards) != 1642 {
		t.Fatalf("收藏列表应该有 1642 张，实际 %d", len(cards))
	}
	first, _ := cards[0].(map[string]any)
	if _, ok := first["id"].(int64); !ok {
		t.Errorf("id 应该是 int64，实际 %T", first["id"])
	}
	if _, ok := first["num"].(int64); !ok {
		t.Errorf("num 应该是 int64，实际 %T", first["num"])
	}
	sets, _ := answer["rotation_card_set_ids"].([]any)
	if len(sets) == 0 {
		t.Error("rotation_card_set_ids 不该是空的")
	}
}

func TestLeaderAndSleeveRoutes(t *testing.T) {
	profile := newProfile(t)
	answer := call(t, profile, "/LeaderSkin/getIdList", map[string]any{})
	ids, _ := answer["leader_skin_ids"].([]any)
	if len(ids) != len(profile.leaders) {
		t.Fatalf("主战者数量不对：%d != %d", len(ids), len(profile.leaders))
	}
	sleeves := call(t, profile, "/Sleeve/getList", map[string]any{})
	list, _ := sleeves["sleeve_list"].([]any)
	if len(list) != 1 {
		t.Fatalf("应该只报一个已核实的卡垫，实际 %d", len(list))
	}
	skin := call(t, profile, "/LeaderSkin/getList", map[string]any{})
	if len(skin["leader_skin_list"].([]any)) != len(profile.leaders) {
		t.Error("主战者列表数量不一致")
	}
}

// 建牌组 → 出现在列表里 → 删掉 → 消失。这条链是客户端"新建牌组"的必经之路。
func TestCreateAndDeleteDeck(t *testing.T) {
	profile := newProfile(t)
	created := call(t, profile, "/Deck/create", map[string]any{
		"deck_format": int64(1),
		"class_id":    int64(1),
		"name":        "测试牌组",
		"cards":       fortyCardsForClass(t, profile, 1),
	})
	list, _ := created["deck_list"].(map[string]any)
	decks, _ := list["decks"].([]any)
	var createdID int64
	for _, raw := range decks {
		deck, _ := raw.(map[string]any)
		if name, _ := deck["name"].(string); name == "测试牌组" {
			createdID, _ = asInt(deck["deck_id"])
		}
	}
	if createdID == 0 {
		t.Fatalf("新建的牌组没有出现在列表里：%v", list)
	}
	after := call(t, profile, "/Deck/delete", map[string]any{
		"deck_format": int64(1),
		"deck_id":     createdID,
	})
	afterList, _ := after["deck_list"].(map[string]any)
	for _, raw := range afterList["decks"].([]any) {
		deck, _ := raw.(map[string]any)
		if id, _ := asInt(deck["deck_id"]); id == createdID {
			t.Fatalf("删除后牌组还在列表里")
		}
	}
}

// 不合法的东西必须被拒绝，而且不能把状态改坏。
func TestRejectsInvalidRequests(t *testing.T) {
	profile := newProfile(t)
	cases := []struct {
		name    string
		path    string
		request map[string]any
	}{
		{"带别职业的卡", "/Deck/create", map[string]any{
			"deck_format": int64(1), "class_id": int64(1), "name": "越界牌组",
			"cards": withForeignCard(t, profile, 1),
		}},
		{"空名字", "/Deck/create", map[string]any{
			"deck_format": int64(1), "class_id": int64(1), "name": "   ",
			"cards": fortyCardsForClass(t, profile, 1),
		}},
		{"未知格式", "/Deck/create", map[string]any{
			"deck_format": int64(9), "class_id": int64(1), "name": "格式不对",
			"cards": fortyCardsForClass(t, profile, 1),
		}},
		{"没核实过的样式", "/Card/setCardStyle", map[string]any{
			"card_id": int64(10001110), "style_id": int64(999999),
		}},
		{"没核实过的卡垫", "/Deck/updateSleeve", map[string]any{
			"deck_format": int64(1), "deck_id": int64(1), "sleeve_id": int64(123),
		}},
	}
	for _, item := range cases {
		if _, err := profile.Handle(item.path, item.request); err == nil {
			t.Errorf("%s：应该被拒绝", item.name)
		}
	}
}

// 状态要落盘：同一份档案再打开一次，牌组还是那些。
func TestStatePersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profile.json")
	first, err := LoadWithState(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := first.Handle("/Deck/create", map[string]any{
		"deck_format": int64(1), "class_id": int64(2), "name": "留下的牌组",
		"cards": fortyCardsForClass(t, first, 2),
	}); err != nil {
		t.Fatal(err)
	}
	second, err := LoadWithState(path)
	if err != nil {
		t.Fatal(err)
	}
	answer := call(t, second, "/Deck/getList", map[string]any{})
	found := false
	lists, _ := answer["deck_lists"].([]any)
	first_list, _ := lists[0].(map[string]any)
	for _, raw := range first_list["decks"].([]any) {
		deck, _ := raw.(map[string]any)
		if name, _ := deck["name"].(string); name == "留下的牌组" {
			found = true
		}
	}
	if !found {
		t.Fatal("重开之后牌组没了")
	}
}

// fortyCardsForClass 拼一副合法的 40 张牌：用该职业（含中立）的 base 卡各 3 张。
func fortyCardsForClass(t *testing.T, profile *Profile, class int64) []any {
	t.Helper()
	cards := []any{}
	total := int64(0)
	used := map[int64]bool{}
	for _, card := range sortedCards(profile.owned) {
		if card.Foil != 0 || card.Set != 10000 {
			continue
		}
		if card.Class != 0 && int64(card.Class) != class {
			continue
		}
		if used[card.Base] {
			continue
		}
		used[card.Base] = true
		count := int64(3)
		if remaining := 40 - total; remaining < count {
			count = remaining
		}
		entry := map[string]any{"id": card.ID, "num": count}
		cards = append(cards, entry)
		total += count
		if total >= 40 {
			break
		}
	}
	if total != 40 {
		t.Fatalf("拼不出 40 张（%d）", total)
	}
	return cards
}

// withForeignCard 把一副合法牌组的最后一张换成别的职业的卡：张数仍是 40，
// 但职业越界，必须被拒绝。
func withForeignCard(t *testing.T, profile *Profile, class int64) []any {
	t.Helper()
	cards := fortyCardsForClass(t, profile, class)
	var foreign int64
	for _, card := range sortedCards(profile.owned) {
		if card.Class != 0 && int64(card.Class) != class && card.Set == 10000 && card.Foil == 0 {
			foreign = card.ID
			break
		}
	}
	if foreign == 0 {
		t.Fatal("找不到别的职业的卡")
	}
	cards[len(cards)-1] = map[string]any{"id": foreign, "num": int64(3)}
	return cards
}
