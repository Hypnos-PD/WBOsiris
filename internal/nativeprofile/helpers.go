package nativeprofile

import (
	"encoding/json"
	"sort"
)

// deepCopyValue 复制一份结构，避免调用方改到存档里的同一个对象。
// JSON 往返是这里最省事也最不容易写错的做法——这些结构本来就都是 JSON 形状。
func deepCopyValue(value any) any {
	raw, err := json.Marshal(value)
	if err != nil {
		return value
	}
	var cloned any
	if err := decodeInto(raw, &cloned); err != nil {
		return value
	}
	// decodeInto 只是用 json.Number 解码（保住大整数），这里必须再归一化一次——
	// 副本最终要编给客户端，留着 json.Number 会变成字符串。
	normalizeNumbers(cloned)
	return cloned
}

// toAnyList 把 []int64 变成 []any（MessagePack 编码时两条路都行，但保持一种形状
// 免得同一份响应里出现两种数组）。
func toAnyList(values []int64) []any {
	out := make([]any, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	return out
}

// intList 取一个整数数组。
func intList(raw any) ([]int64, error) {
	items, ok := raw.([]any)
	if !ok {
		return nil, fail("需要一个整数数组，收到 %T", raw)
	}
	out := make([]int64, 0, len(items))
	for _, item := range items {
		value, err := asInt(item)
		if err != nil {
			return nil, err
		}
		out = append(out, value)
	}
	return out, nil
}

// sortedLeaderIDs 升序的主战者 id。
func sortedLeaderIDs(leaders map[int64]Leader) []int64 {
	ids := make([]int64, 0, len(leaders))
	for id := range leaders {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

// sortedCards 按 id 升序的卡片。响应的顺序要稳定，否则同样的请求两次拿到的字节不同，
// 回归测试就没法比。
func sortedCards(cards map[int64]Card) []Card {
	out := make([]Card, 0, len(cards))
	for _, card := range cards {
		out = append(out, card)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// ownedSetIDs 收藏里出现过的卡包 id。
func ownedSetIDs(owned map[int64]Card) []any {
	seen := map[int64]bool{}
	for _, card := range owned {
		if card.Set < 80000 {
			seen[card.Set] = true
		}
	}
	sets := make([]int64, 0, len(seen))
	for set := range seen {
		sets = append(sets, set)
	}
	sort.Slice(sets, func(i, j int) bool { return sets[i] < sets[j] })
	return toAnyList(sets)
}

// deckEntries 取一副牌里的卡片条目（已经是 map 形状）。
func deckEntries(deck map[string]any) []map[string]any {
	raw, ok := deck["cards"]
	if !ok {
		return nil
	}
	entries, err := asEntries(raw)
	if err != nil {
		return nil
	}
	return entries
}

// resultAsEntries 把响应里那份 []any（模板拷贝）当成条目列表读出来。
func resultAsEntries(values []any) []map[string]any {
	entries := make([]map[string]any, 0, len(values))
	for _, value := range values {
		if entry, ok := value.(map[string]any); ok {
			entries = append(entries, entry)
		}
	}
	return entries
}
