package nativeprofile

import (
	"fmt"
	"sort"
	"strings"
)

// Handle 处理一条档案路由，返回该路由响应里的 data。
//
// 逐条对照 delta 的 tools/local_practice_profile.py。两个约定：
//   - 契约里没有的路由返回 (nil, nil)，由上层交回"未实现"——不猜；
//   - 请求不合法返回 ProfileError，同样如实拒绝。
//
// 对局入口 `/Practice/battleStart` 单独走 startPracticeBattle（见 battle.go）：它要
// 决定先后手、发牌，并把初始快照落盘；`/Practice/battleFinish` 还不在这里——那要等
// 对局层能真的打完一局。
func (p *Profile) Handle(path string, request map[string]any) (map[string]any, error) {
	if !p.HandledRoute(path) {
		return nil, nil
	}
	if path == "/Practice/battleStart" {
		var result map[string]any
		err := p.store.Current(func(state *State) error {
			data, err := p.startPracticeBattle(state, request)
			if err != nil {
				return err
			}
			result = data
			return nil
		})
		if err != nil {
			return nil, err
		}
		return result, nil
	}
	if path == "/Practice/battleFinish" {
		return nil, nil
	}
	var result map[string]any
	err := p.store.Current(func(state *State) error {
		data, changed, err := p.apply(state, path, request)
		if err != nil {
			return err
		}
		result = data
		if changed {
			state.Revision++
			return p.store.save()
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// apply 是 Handle 的本体，在锁里跑。changed 表示状态被改过（要落盘）。
func (p *Profile) apply(state *State, path string, request map[string]any) (map[string]any, bool, error) {
	data, err := p.copyRouteData(path)
	if err != nil {
		return nil, false, err
	}
	defaults, err := p.styles.Defaults(p.cards, state.Styles)
	if err != nil {
		return nil, false, err
	}
	changed := false
	switch {
	case path == "/LeaderSkin/getIdList":
		data["leader_skin_ids"] = toAnyList(sortedLeaderIDs(p.leaders))
	case path == "/Sleeve/getList":
		// 牌组详情在打开原生选择器之前会一起要这份列表，所以它不能是空的。
		// 只给那一个已核实的卡垫——这是本地持有，不是对官方库存的断言。
		sleeve, err := p.copyTemplate("UserSleeve")
		if err != nil {
			return nil, false, err
		}
		sleeve["sleeve_id"] = int64(100011205)
		data["sleeve_list"] = []any{sleeve}
	case path == "/LeaderSkin/getList":
		list := make([]any, 0, len(p.leaders))
		for _, id := range sortedLeaderIDs(p.leaders) {
			entry, err := p.copyTemplate("LeaderSkin")
			if err != nil {
				return nil, false, err
			}
			entry["leader_skin_id"] = id
			entry["class_id"] = int64(p.leaders[id].CardClass)
			list = append(list, entry)
		}
		data["leader_skin_list"] = list
	case path == "/LeaderSkin/getSettings":
		data["leader_skin_settings"] = deepCopyValue(state.LeaderSkinSettings)
	case path == "/LeaderSkin/change" || path == "/LeaderSkin/changeMulti":
		var entries []map[string]any
		if path == "/LeaderSkin/change" {
			entries = []map[string]any{request}
		} else {
			raw, ok := request["leader_skin_settings"]
			if !ok {
				return nil, false, fail("缺少 leader_skin_settings")
			}
			entries, err = asEntries(raw)
			if err != nil {
				return nil, false, err
			}
		}
		if len(entries) == 0 || len(entries) > 7 {
			return nil, false, fail("主战者设置条数不合法")
		}
		byClass := map[int64]map[string]any{}
		for _, entry := range entries {
			setting, err := p.leaderSetting(entry)
			if err != nil {
				return nil, false, err
			}
			class, _ := asInt(setting["class_id"])
			if _, duplicate := byClass[class]; duplicate {
				return nil, false, fail("同一职业出现了两条主战者设置")
			}
			byClass[class] = setting
		}
		updated := make([]map[string]any, 0, len(state.LeaderSkinSettings))
		for _, existing := range state.LeaderSkinSettings {
			class, _ := asInt(existing["class_id"])
			if replacement, ok := byClass[class]; ok {
				updated = append(updated, replacement)
				continue
			}
			updated = append(updated, existing)
		}
		state.LeaderSkinSettings = updated
		data["leader_skin_settings"] = deepCopyValue(updated)
		changed = true
	case path == "/Card/getList" || path == "/Card/getListForMypage":
		cards, err := p.ownedCardsPayload(defaults)
		if err != nil {
			return nil, false, err
		}
		data["cards"] = cards
		if _, ok := data["rotation_card_set_ids"]; ok {
			data["rotation_card_set_ids"] = ownedSetIDs(p.owned)
		}
	case path == "/Deck/getList" || path == "/Deck/getListForEdit" || path == "/Deck/getListForCopy":
		lists, err := p.deckLists(state)
		if err != nil {
			return nil, false, err
		}
		if strings.HasSuffix(path, "ForEdit") {
			edited := make([]any, 0, len(lists))
			for _, list := range lists {
				format, _ := asInt(list["deck_format"])
				entry, err := p.copyTemplate("EditDeckList")
				if err != nil {
					return nil, false, err
				}
				entry["deck_list"] = list
				entry["last_edit_deck_id"] = state.LastEdit[formatKey(format)]
				edited = append(edited, entry)
			}
			data["deck_lists"] = edited
		} else {
			entries := make([]any, 0, len(lists))
			for _, list := range lists {
				entries = append(entries, list)
			}
			data["deck_lists"] = entries
		}
		if _, ok := data["deck_config"]; ok {
			data["deck_config"] = deepCopyValue(state.DeckConfig)
		}
		if _, ok := data["leader_skin_settings"]; ok {
			data["leader_skin_settings"] = deepCopyValue(state.LeaderSkinSettings)
		}
		if _, ok := data["battle_settings"]; ok {
			data["battle_settings"] = deepCopyValue(state.BattleSettings)
		}
		if _, ok := data["last_edit_battle_format"]; ok {
			data["last_edit_battle_format"] = state.LastEditBattleFormat
		}
	case path == "/Deck/beginEdit" || path == "/Deck/beginEditForCopy":
		format, err := p.requestFormat(request, 1)
		if err != nil {
			return nil, false, err
		}
		identifier, err := asIntOr(request["deck_id"], 0)
		if err != nil {
			return nil, false, err
		}
		var deck map[string]any
		if identifier != 0 && path == "/Deck/beginEdit" {
			existing, err := lookupDeck(state, identifier)
			if err != nil {
				return nil, false, err
			}
			deck = deepCopyValue(existing).(map[string]any)
		} else {
			class, err := asIntOr(request["class_id"], 1)
			if err != nil {
				return nil, false, err
			}
			if err := bounded(class, 1, 7); err != nil {
				return nil, false, err
			}
			deck, err = p.newDeck(0, class, format, "")
			if err != nil {
				return nil, false, err
			}
			if raw, ok := request["cards"]; ok {
				entries, err := asEntries(raw)
				if err != nil {
					return nil, false, err
				}
				normalized, err := p.normalizedCards(entries, class)
				if err != nil {
					return nil, false, err
				}
				deck["cards"] = normalized
				if err := p.validateDeck(deck); err != nil {
					return nil, false, err
				}
			}
		}
		data["deck"] = deck
		class, _ := asInt(deck["class_id"])
		overrides, err := p.styles.Overrides(p.cards, deckEntries(deck))
		if err != nil {
			return nil, false, err
		}
		enabled, err := p.deckEnabledCards(class, overrides, defaults)
		if err != nil {
			return nil, false, err
		}
		data["cards"] = enabled
		tokens, err := p.tokenCards(defaults)
		if err != nil {
			return nil, false, err
		}
		data["token_cards"] = tokens
		state.LastEditBattleFormat = format
		state.LastEdit[formatKey(format)] = identifier
		changed = true
	case path == "/Deck/create":
		format, err := p.requestFormat(request, 1)
		if err != nil {
			return nil, false, err
		}
		class, err := asIntOr(request["class_id"], 1)
		if err != nil {
			return nil, false, err
		}
		if err := bounded(class, 1, 7); err != nil {
			return nil, false, err
		}
		identifier, err := asIntOr(request["deck_id"], 0)
		if err != nil {
			return nil, false, err
		}
		var deck map[string]any
		existing, lookupErr := lookupDeck(state, identifier)
		if identifier != 0 && lookupErr == nil {
			deck = existing
			if current, _ := asInt(deck["class_id"]); current != class {
				return nil, false, fail("牌组职业对不上")
			}
		} else {
			if identifier < state.NextID {
				identifier = state.NextID
			}
			state.NextID = identifier + 1
			deck, err = p.newDeck(identifier, class, format, "")
			if err != nil {
				return nil, false, err
			}
			state.Decks = append(state.Decks, deck)
		}
		name, err := deckName(request["name"])
		if err != nil {
			return nil, false, err
		}
		deck["name"] = name
		entries, err := asEntries(request["cards"])
		if err != nil {
			return nil, false, err
		}
		normalized, err := p.normalizedCards(entries, class)
		if err != nil {
			return nil, false, err
		}
		deck["cards"] = normalized
		if err := p.validateDeck(deck); err != nil {
			return nil, false, err
		}
		state.LastEdit[formatKey(format)] = identifier
		state.LastEditBattleFormat = format
		list, err := p.deckListPayload(state, format)
		if err != nil {
			return nil, false, err
		}
		data["deck_list"] = list
		changed = true
	case path == "/Deck/delete" || path == "/Deck/deleteList":
		format, err := p.requestFormat(request, 1)
		if err != nil {
			return nil, false, err
		}
		identifiers, err := deleteTargets(request)
		if err != nil {
			return nil, false, err
		}
		for _, identifier := range identifiers {
			deck, err := lookupDeck(state, identifier)
			if err != nil {
				return nil, false, err
			}
			if current, _ := asInt(deck["deck_format"]); current != format {
				return nil, false, fail("牌组格式对不上")
			}
		}
		removed := map[int64]bool{}
		for _, identifier := range identifiers {
			removed[identifier] = true
		}
		kept := make([]map[string]any, 0, len(state.Decks))
		for _, deck := range state.Decks {
			id, _ := asInt(deck["deck_id"])
			if !removed[id] {
				kept = append(kept, deck)
			}
		}
		state.Decks = kept
		for key, value := range state.LastEdit {
			if removed[value] {
				state.LastEdit[key] = 0
			}
		}
		settings := make([]map[string]any, 0, len(state.BattleSettings))
		for _, setting := range state.BattleSettings {
			id, _ := asInt(setting["deck_id"])
			if !removed[id] {
				settings = append(settings, setting)
			}
		}
		state.BattleSettings = settings
		list, err := p.deckListPayload(state, format)
		if err != nil {
			return nil, false, err
		}
		data["deck_list"] = list
		changed = true
	case path == "/Deck/validateDeckName":
		if _, err := deckName(request["name"]); err != nil {
			return nil, false, err
		}
	case strings.HasPrefix(path, "/Deck/update"):
		format, err := p.requestFormat(request, 1)
		if err != nil {
			return nil, false, err
		}
		if path == "/Deck/updateOrderList" {
			orderRaw, ok := request["order_list"]
			if !ok {
				return nil, false, fail("缺少 order_list")
			}
			order, err := asEntries(orderRaw)
			if err != nil {
				return nil, false, err
			}
			_ = order
			ordered, err := intList(orderRaw)
			if err != nil {
				return nil, false, err
			}
			if err := p.reorderDecks(state, format, ordered); err != nil {
				return nil, false, err
			}
			list, err := p.deckListPayload(state, format)
			if err != nil {
				return nil, false, err
			}
			data["deck_list"] = list
			changed = true
			break
		}
		identifier, err := asInt(request["deck_id"])
		if err != nil {
			return nil, false, err
		}
		deck, err := lookupDeck(state, identifier)
		if err != nil {
			return nil, false, err
		}
		if current, _ := asInt(deck["deck_format"]); current != format {
			return nil, false, fail("牌组格式对不上")
		}
		switch {
		case path == "/Deck/updateDeckInCard":
			entries, err := asEntries(request["cards"])
			if err != nil {
				return nil, false, err
			}
			class, _ := asInt(deck["class_id"])
			normalized, err := p.normalizedCards(entries, class)
			if err != nil {
				return nil, false, err
			}
			deck["cards"] = normalized
			if err := p.validateDeck(deck); err != nil {
				return nil, false, err
			}
		case path == "/Deck/updateName":
			name, err := deckName(request["name"])
			if err != nil {
				return nil, false, err
			}
			deck["name"] = name
		case path == "/Deck/updateLeaderSkin":
			leader, err := asIntOr(request["leader_skin_id"], 0)
			if err != nil {
				return nil, false, err
			}
			class, _ := asInt(deck["class_id"])
			if leader != 0 {
				if err := p.checkLeader(leader, class); err != nil {
					return nil, false, err
				}
			}
			deck["leader_skin_id"] = leader
			if leader == 0 {
				deck["leader_skin_id_list"] = []any{}
			} else {
				deck["leader_skin_id_list"] = []any{leader}
			}
			deck["is_random_leader_skin"] = false
		case path == "/Deck/updateRandomLeaderSkin":
			raw, ok := request["leader_skin_ids"]
			if !ok {
				return nil, false, fail("缺少 leader_skin_ids")
			}
			class, _ := asInt(deck["class_id"])
			choices, err := p.leaderList(raw, class)
			if err != nil {
				return nil, false, err
			}
			deck["leader_skin_id"] = choices[0]
			deck["leader_skin_id_list"] = toAnyList(choices)
			deck["is_random_leader_skin"] = true
		case path == "/Deck/updateSleeve":
			sleeve, err := asInt(request["sleeve_id"])
			if err != nil {
				return nil, false, err
			}
			if sleeve != 100011205 {
				return nil, false, fail("这个卡垫没有被核实过")
			}
		default:
			return nil, false, nil
		}
		if _, ok := data["deck_list"]; ok {
			list, err := p.deckListPayload(state, format)
			if err != nil {
				return nil, false, err
			}
			data["deck_list"] = list
		}
		changed = true
	case strings.HasPrefix(path, "/DeckConfig/"):
		key, argument, err := deckConfigField(path)
		if err != nil {
			return nil, false, err
		}
		value, err := asBool(request[argument])
		if err != nil {
			return nil, false, err
		}
		state.DeckConfig[key] = value
		changed = true
	case path == "/Card/setCardStyle":
		identifier, err := asInt(request["card_id"])
		if err != nil {
			return nil, false, err
		}
		card, ok := p.cards[identifier]
		if !ok {
			return nil, false, fail("不存在的卡 %d", identifier)
		}
		style, err := asInt(request["style_id"])
		if err != nil {
			return nil, false, err
		}
		if err := p.styles.Validate(card, style); err != nil {
			return nil, false, ProfileError{Reason: err.Error()}
		}
		// 一族一个默认样式：先清掉这一族原来的记录，再写新的。
		for key := range state.Styles {
			index, err := parseKey(key)
			if err != nil {
				continue
			}
			if existing, ok := p.cards[index]; ok && existing.Normal == card.Normal {
				delete(state.Styles, key)
			}
		}
		state.Styles[fmt.Sprint(card.Normal)] = style
		changed = true
	case path == "/CardStyle/getSettings":
		// 契约里的空壳就是答案（下面是统一填 card_style_settings）。
	case path == "/Battle/updateDeckId":
		// 原生练习发来的 battle_format 是 BattleFormat.None(0)，与 DeckFormat 是两套
		// 枚举，别用同一个 helper 校验（delta 的注释里踩过这个坑）。
		identifier, err := asInt(request["deck_id"])
		if err != nil {
			return nil, false, err
		}
		deck, err := lookupDeck(state, identifier)
		if err != nil {
			return nil, false, err
		}
		deckFormat, err := asInt(request["deck_format"])
		if err != nil {
			return nil, false, err
		}
		if current, _ := asInt(deck["deck_format"]); current != deckFormat {
			return nil, false, fail("牌组格式对不上")
		}
		battleFormat, err := asInt(request["battle_format"])
		if err != nil {
			return nil, false, err
		}
		if !p.enumHasValue("BattleFormat", battleFormat) {
			return nil, false, fail("不支持的对战格式 %d", battleFormat)
		}
		battleType, err := asInt(request["battle_type"])
		if err != nil {
			return nil, false, err
		}
		setting, err := p.copyTemplate("BattleSetting")
		if err != nil {
			return nil, false, err
		}
		setting["battle_type"] = battleType
		setting["battle_format"] = battleFormat
		setting["deck_id"] = identifier
		setting["deck_format"] = deckFormat
		kept := make([]map[string]any, 0, len(state.BattleSettings)+1)
		for _, existing := range state.BattleSettings {
			existingType, _ := asInt(existing["battle_type"])
			existingFormat, _ := asInt(existing["battle_format"])
			if existingType == battleType && existingFormat == battleFormat {
				continue
			}
			kept = append(kept, existing)
		}
		state.BattleSettings = append(kept, setting)
		changed = true
	case path == "/Practice/info":
		userInfo, err := p.copyTemplate("UserInfo")
		if err != nil {
			return nil, false, err
		}
		userInfo["client_id"] = int64(1)
		userInfo["name"] = "Offline Tester"
		userInfo["emblem_id"] = int64(100011100)
		data["user_info"] = userInfo
		practice, err := p.practiceList(state)
		if err != nil {
			return nil, false, err
		}
		data["practice_list"] = practice
	case path == "/Practice/deckInfo":
		kind, err := asInt(request["type"])
		if err != nil {
			return nil, false, err
		}
		if kind != 2 {
			return nil, false, fail("只支持本地自有牌组作为对手")
		}
		identifier, err := asInt(request["target_id"])
		if err != nil {
			return nil, false, err
		}
		deck, err := lookupDeck(state, identifier)
		if err != nil {
			return nil, false, err
		}
		data["deck_data"] = deepCopyValue(deck)
	default:
		return nil, false, nil
	}
	if _, ok := data["card_style_settings"]; ok {
		settings, err := p.cardStyleSettings(state)
		if err != nil {
			return nil, false, err
		}
		data["card_style_settings"] = settings
	}
	return data, changed, nil
}

// deckList 取某个格式的牌组列表（delta 的 deck_lists 里的一项）。
type deckList struct {
	Format int64
	Decks  []map[string]any
}

func (d deckList) decksAny() []any {
	out := make([]any, 0, len(d.Decks))
	for _, deck := range d.Decks {
		out = append(out, deepCopyValue(deck))
	}
	return out
}

// deckLists 返回四个格式各一份列表（客户端按格式分别展示）。
func (p *Profile) deckLists(state *State) ([]map[string]any, error) {
	lists := make([]map[string]any, 0, 4)
	for _, format := range []int64{1, 2, 3, 4} {
		list, err := p.deckList(state, format)
		if err != nil {
			return nil, err
		}
		entry, err := p.copyTemplate("DeckList")
		if err != nil {
			return nil, err
		}
		entry["deck_format"] = format
		entry["decks"] = list.decksAny()
		entry["is_new_deck_creatable"] = true
		lists = append(lists, entry)
	}
	return lists, nil
}

func (p *Profile) deckList(state *State, format int64) (deckList, error) {
	list := deckList{Format: format}
	for _, deck := range state.Decks {
		current, _ := asInt(deck["deck_format"])
		if current == format {
			list.Decks = append(list.Decks, deck)
		}
	}
	return list, nil
}

// deckListPayload 是响应里那个 DeckList 对象：格式、牌组数组、以及"能不能新建"。
func (p *Profile) deckListPayload(state *State, format int64) (map[string]any, error) {
	list, err := p.deckList(state, format)
	if err != nil {
		return nil, err
	}
	payload, err := p.copyTemplate("DeckList")
	if err != nil {
		return nil, err
	}
	payload["deck_format"] = format
	payload["decks"] = list.decksAny()
	payload["is_new_deck_creatable"] = true
	return payload, nil
}

// newDeck 造一副空牌组（照 delta 的 new_deck）。
func (p *Profile) newDeck(identifier, class, format int64, name string) (map[string]any, error) {
	deck, err := p.copyTemplate("DeckData")
	if err != nil {
		return nil, err
	}
	if name == "" {
		name = "离线测试牌组"
	}
	deck["deck_id"] = identifier
	deck["deck_format"] = format
	deck["class_id"] = class
	deck["name"] = name
	deck["sleeve_id"] = int64(100011205)
	deck["leader_skin_id"] = 1000 + class
	deck["leader_skin_id_list"] = []any{1000 + class}
	deck["error_state"] = p.enumValue("DeckError", "LessThanCardNum")
	return deck, nil
}

// deckEnabledCards 是牌组编辑界面里"这个职业能用的卡"。
func (p *Profile) deckEnabledCards(class int64, overrides, defaults map[int64]int64) ([]any, error) {
	cards := make([]any, 0, len(p.owned))
	for _, card := range sortedCards(p.owned) {
		if card.Class != 0 && int64(card.Class) != class {
			continue
		}
		entry, err := p.copyTemplate("DeckEnabledCard")
		if err != nil {
			return nil, err
		}
		entry["id"] = card.ID
		entry["num"] = int64(3)
		entry["styles"] = toAnyList(p.styles.Options(card.ID))
		if style, ok := overrides[card.Normal]; ok {
			entry["style_setting"] = style
		} else {
			entry["style_setting"] = int64(0)
		}
		entry["default_style_setting"] = defaults[card.Normal]
		cards = append(cards, entry)
	}
	return cards, nil
}

// tokenCards 是牌组编辑里那些"派生出来的 token 卡"。
func (p *Profile) tokenCards(defaults map[int64]int64) ([]any, error) {
	cards := make([]any, 0)
	for _, card := range sortedCards(p.cards) {
		if !card.Token || card.Foil != 0 || card.Type < 1 || card.Type > 4 {
			continue
		}
		entry, err := p.copyTemplate("DeckTokenCard")
		if err != nil {
			return nil, err
		}
		entry["id"] = card.ID
		entry["styles"] = toAnyList(p.styles.Options(card.ID))
		entry["default_style_setting"] = defaults[card.Normal]
		cards = append(cards, entry)
	}
	return cards, nil
}

// ownedCardsPayload 是收藏列表（/Card/getList）。
func (p *Profile) ownedCardsPayload(defaults map[int64]int64) ([]any, error) {
	cards := make([]any, 0, len(p.owned))
	for _, card := range sortedCards(p.owned) {
		entry, err := p.copyTemplate("Card")
		if err != nil {
			return nil, err
		}
		entry["id"] = card.ID
		entry["num"] = int64(3)
		entry["styles"] = toAnyList(p.styles.Options(card.ID))
		entry["style_setting"] = defaults[card.Normal]
		cards = append(cards, entry)
	}
	return cards, nil
}

func (p *Profile) cardStyleSettings(state *State) ([]any, error) {
	defaults, err := p.styles.Defaults(p.cards, state.Styles)
	if err != nil {
		return nil, err
	}
	keys := make([]int64, 0, len(defaults))
	for key := range defaults {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	settings := make([]any, 0, len(keys))
	for _, key := range keys {
		entry, err := p.copyTemplate("CardStyleSetting")
		if err != nil {
			return nil, err
		}
		entry["id"] = key
		entry["style"] = defaults[key]
		settings = append(settings, entry)
	}
	return settings, nil
}

// practiceList 是练习模式的对手列表：每个自有牌组都是"可以打的电脑"。
func (p *Profile) practiceList(state *State) ([]any, error) {
	list := make([]any, 0, len(state.Decks))
	for _, deck := range state.Decks {
		entry, err := p.copyTemplate("Practice")
		if err != nil {
			return nil, err
		}
		identifier, _ := asInt(deck["deck_id"])
		format, _ := asInt(deck["deck_format"])
		class, _ := asInt(deck["class_id"])
		entry["type"] = int64(2)
		entry["target_id"] = identifier
		entry["deck_format"] = format
		entry["battle_format"] = format
		entry["name"] = "练习电脑"
		entry["emblem_id"] = 210000001 + class*1000
		entry["deck_data"] = deepCopyValue(deck)
		// 奖励要留 null：客户端遇到 reward_type=0 会去加载不存在的资源然后抛异常。
		entry["is_reward_received"] = true
		entry["reward"] = nil
		list = append(list, entry)
	}
	return list, nil
}

func (p *Profile) validateDeck(deck map[string]any) error {
	entries := deckEntries(deck)
	class, _ := asInt(deck["class_id"])
	normalized, err := p.normalizedCards(entries, class)
	if err != nil {
		return err
	}
	deck["cards"] = normalized
	total := int64(0)
	for _, entry := range normalized {
		item, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		count, _ := asInt(item["num"])
		total += count
	}
	if total == 40 {
		deck["error_state"] = int64(0)
	} else {
		deck["error_state"] = p.enumValue("DeckError", "LessThanCardNum")
	}
	return nil
}

// normalizedCards 校验并整理一副牌里的卡片：每张 base 最多 3 张、总计最多 40 张、
// 职业对得上、样式被核实过。
func (p *Profile) normalizedCards(entries []map[string]any, class int64) ([]any, error) {
	if len(entries) > 40 {
		return nil, fail("卡片条目太多")
	}
	result := make([]any, 0, len(entries))
	seen := map[int64]bool{}
	groups := map[int64]int64{}
	total := int64(0)
	for _, entry := range entries {
		identifier, err := asInt(entry["id"])
		if err != nil {
			return nil, err
		}
		count, err := asInt(entry["num"])
		if err != nil {
			return nil, err
		}
		if err := bounded(count, 1, 3); err != nil {
			return nil, err
		}
		card, ok := p.owned[identifier]
		if !ok {
			return nil, fail("这张卡不在收藏里")
		}
		if card.Class != 0 && int64(card.Class) != class {
			return nil, fail("这张卡不能放进这个职业的牌组")
		}
		if seen[identifier] {
			return nil, fail("同一张卡重复出现")
		}
		if raw, ok := entry["style"]; ok && raw != nil {
			style, err := asInt(raw)
			if err != nil {
				return nil, err
			}
			if err := p.styles.Validate(card, style); err != nil {
				return nil, ProfileError{Reason: err.Error()}
			}
		}
		seen[identifier] = true
		groups[card.Base] += count
		if groups[card.Base] > 3 {
			return nil, fail("同一张 base 卡超过 3 张")
		}
		total += count
		out, err := p.copyTemplate("DeckInCard")
		if err != nil {
			return nil, err
		}
		out["id"] = identifier
		out["num"] = count
		out["style"] = entry["style"]
		result = append(result, out)
	}
	if total > 40 {
		return nil, fail("牌组超过 40 张")
	}
	if _, err := p.styles.Overrides(p.cards, resultAsEntries(result)); err != nil {
		return nil, ProfileError{Reason: err.Error()}
	}
	return result, nil
}

func (p *Profile) requestFormat(request map[string]any, fallback int64) (int64, error) {
	value := request["battle_format"]
	if value == nil {
		value = request["deck_format"]
	}
	format, err := asIntOr(value, fallback)
	if err != nil {
		return 0, err
	}
	switch format {
	case 1, 2, 3, 4:
		return format, nil
	}
	return 0, fail("不支持的对战格式 %d", format)
}

func (p *Profile) checkLeader(identifier, class int64) error {
	leader, ok := p.leaders[identifier]
	if !ok || int64(leader.CardClass) != class {
		return fail("这个职业没有这个主战者")
	}
	return nil
}

func (p *Profile) leaderList(raw any, class int64) ([]int64, error) {
	values, err := intList(raw)
	if err != nil {
		return nil, err
	}
	if len(values) == 0 || len(values) > len(p.leaders) {
		return nil, fail("至少要选一个可用的主战者")
	}
	seen := map[int64]bool{}
	for _, value := range values {
		if err := p.checkLeader(value, class); err != nil {
			return nil, err
		}
		if seen[value] {
			return nil, fail("主战者重复")
		}
		seen[value] = true
	}
	return values, nil
}

func (p *Profile) leaderSetting(entry map[string]any) (map[string]any, error) {
	class, err := asInt(entry["class_id"])
	if err != nil {
		return nil, err
	}
	if err := bounded(class, 1, 7); err != nil {
		return nil, err
	}
	random := false
	if raw, ok := entry["is_random_leader_skin"]; ok {
		random, err = asBool(raw)
		if err != nil {
			return nil, err
		}
	}
	identifier, err := asIntOr(entry["leader_skin_id"], 0)
	if err != nil {
		return nil, err
	}
	if identifier != 0 {
		if err := p.checkLeader(identifier, class); err != nil {
			return nil, err
		}
	}
	var choices []int64
	if raw, ok := entry["leader_skin_id_list"]; ok && raw != nil {
		choices, err = intList(raw)
		if err != nil {
			return nil, err
		}
	}
	if !random {
		if len(choices) == 0 {
			choices = []int64{identifier}
		}
	}
	validated, err := p.leaderList(toAnyList(choices), class)
	if err != nil {
		return nil, err
	}
	out, err := p.copyTemplate("LeaderSkinSetting")
	if err != nil {
		return nil, err
	}
	out["class_id"] = class
	out["leader_skin_id"] = identifier
	out["leader_skin_id_list"] = toAnyList(validated)
	out["is_random_leader_skin"] = random
	return out, nil
}

func (p *Profile) reorderDecks(state *State, format int64, order []int64) error {
	actual := []int64{}
	selected := map[int64]map[string]any{}
	for _, deck := range state.Decks {
		current, _ := asInt(deck["deck_format"])
		if current != format {
			continue
		}
		identifier, _ := asInt(deck["deck_id"])
		actual = append(actual, identifier)
		selected[identifier] = deck
	}
	if len(order) != len(actual) {
		return fail("牌组顺序与现有牌组对不上")
	}
	sortedActual := append([]int64(nil), actual...)
	sortedOrder := append([]int64(nil), order...)
	sort.Slice(sortedActual, func(i, j int) bool { return sortedActual[i] < sortedActual[j] })
	sort.Slice(sortedOrder, func(i, j int) bool { return sortedOrder[i] < sortedOrder[j] })
	for index := range sortedActual {
		if sortedActual[index] != sortedOrder[index] {
			return fail("牌组顺序与现有牌组对不上")
		}
	}
	kept := make([]map[string]any, 0, len(state.Decks))
	for _, deck := range state.Decks {
		current, _ := asInt(deck["deck_format"])
		if current != format {
			kept = append(kept, deck)
		}
	}
	for _, identifier := range order {
		kept = append(kept, selected[identifier])
	}
	state.Decks = kept
	return nil
}

func lookupDeck(state *State, identifier int64) (map[string]any, error) {
	if identifier <= 0 {
		return nil, fail("牌组 id 不合法")
	}
	for _, deck := range state.Decks {
		current, _ := asInt(deck["deck_id"])
		if current == identifier {
			return deck, nil
		}
	}
	return nil, fail("没有这个离线牌组")
}

// deckName 校验牌组名：非空、不超过 40 字、没有控制字符。
func deckName(raw any) (string, error) {
	name, err := asString(raw)
	if err != nil {
		return "", err
	}
	trimmed := strings.TrimSpace(name)
	if trimmed == "" || len([]rune(trimmed)) > 40 {
		return "", fail("牌组名不合法")
	}
	for _, r := range trimmed {
		if r < 32 {
			return "", fail("牌组名里有控制字符")
		}
	}
	return trimmed, nil
}

// deleteTargets 取要删的牌组 id 列表：/Deck/deleteList 给数组，/Deck/delete 给单个。
func deleteTargets(request map[string]any) ([]int64, error) {
	raw, ok := request["deck_id_list"]
	if !ok || raw == nil {
		raw = request["deck_id"]
	}
	if raw == nil {
		return nil, fail("缺少要删除的牌组 id")
	}
	if _, isSlice := raw.([]any); isSlice {
		return intList(raw)
	}
	identifier, err := asInt(raw)
	if err != nil {
		return nil, err
	}
	return []int64{identifier}, nil
}

func deckConfigField(path string) (string, string, error) {
	switch path {
	case "/DeckConfig/updateIsCopyCardStyle":
		return "is_copy_card_style", "is_copy", nil
	case "/DeckConfig/updateIsCopyLeaderSkin":
		return "is_copy_leader_skin", "is_copy", nil
	case "/DeckConfig/updateIsCopySleeve":
		return "is_copy_sleeve", "is_copy", nil
	case "/DeckConfig/updateIsFoilPreferred":
		return "is_foil_preferred", "is_preferred", nil
	}
	return "", "", fail("不认识的 DeckConfig 路由 %s", path)
}

func formatKey(format int64) string { return fmt.Sprintf("%d", format) }

func pathSuffix(path string) string {
	if index := strings.LastIndex(path, "/"); index >= 0 {
		return path[index+1:]
	}
	return path
}

// enumHasValue 判断某个枚举里有没有（名字到值的映射里的）这个取值。
func (p *Profile) enumHasValue(enum string, want int64) bool {
	for _, value := range p.contract.Enums[enum] {
		if int64(value) == want {
			return true
		}
	}
	return false
}
