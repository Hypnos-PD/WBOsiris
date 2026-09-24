package runner

import (
	"fmt"
	"sort"

	"wbo/internal/engine/ir"
)

// Format 描述一个构筑赛制：可用的卡包范围，以及（预留的）禁限卡表。
//
// 规则与 WBArts 的 js/deck.js 保持一致：
//   - 指定模式（rotation）= 基础卡牌（10000）+ 最新的 6 个已发布扩展包；
//   - 无限制模式（unlimited）= 全部已发布卡包；
//   - 附属卡包（90000）属于衍生卡，任何赛制都不能编入卡组。
//
// 官方目前没有禁限卡表，Banlist 先留空；结构留在这里是为了以后加"禁止/限制 1 张/
// 准限制 2 张"时不必再改调用方。
type Format struct {
	ID      string         `json:"id"`
	Name    string         `json:"name"`
	Packs   []int          `json:"packs"`
	Banlist map[int]string `json:"banlist,omitempty"`
}

const (
	FormatRotation     = "rotation"
	FormatUnlimited    = "unlimited"
	corePack           = 10000
	rotationExpansions = 6
	expansionLimit     = 20000
)

// PublishedPacks 返回卡池里已发布的卡包编号（10000 起、20000 以下），升序。
// 附属卡包（90000）与其它特殊编号不算"已发布卡包"。
func PublishedPacks(cards *ir.CardPack) []int {
	if cards == nil {
		return nil
	}
	seen := map[int]bool{}
	for n := range cards.Cards {
		pack := cards.Cards[n].Meta.Pack
		if pack >= corePack && pack < expansionLimit {
			seen[pack] = true
		}
	}
	packs := make([]int, 0, len(seen))
	for pack := range seen {
		packs = append(packs, pack)
	}
	sort.Ints(packs)
	return packs
}

// Formats 返回当前卡池下的赛制列表：指定模式在前（默认赛制）。
func Formats(cards *ir.CardPack) []Format {
	published := PublishedPacks(cards)
	expansions := make([]int, 0, len(published))
	for _, pack := range published {
		if pack != corePack {
			expansions = append(expansions, pack)
		}
	}
	sort.Sort(sort.Reverse(sort.IntSlice(expansions)))
	rotation := []int{corePack}
	for _, pack := range expansions {
		if len(rotation) > rotationExpansions {
			break
		}
		rotation = append(rotation, pack)
	}
	sort.Ints(rotation)
	return []Format{
		{ID: FormatRotation, Name: "指定模式", Packs: rotation},
		{ID: FormatUnlimited, Name: "无限制模式", Packs: published},
	}
}

func FormatByID(cards *ir.CardPack, id string) (Format, error) {
	formats := Formats(cards)
	if id == "" {
		// 与官方客户端一致：不指定时按指定模式处理。
		return formats[0], nil
	}
	for _, format := range formats {
		if format.ID == id {
			return format, nil
		}
	}
	return Format{}, fmt.Errorf("unknown format %q", id)
}

// AllowsPack 报告赛制是否允许某个卡包。
func (f Format) AllowsPack(pack int) bool {
	for _, allowed := range f.Packs {
		if allowed == pack {
			return true
		}
	}
	return false
}

// ValidateDeckForFormat 在构筑结构之外再检查卡包与（预留的）禁限卡表。
func ValidateDeckForFormat(cards *ir.CardPack, deck []int, format Format) error {
	if err := ValidateMatchDeck(cards, deck); err != nil {
		return err
	}
	index := make(map[int]*ir.Card, len(cards.Cards))
	for n := range cards.Cards {
		index[cards.Cards[n].ID] = &cards.Cards[n]
	}
	counts := map[int]int{}
	for _, id := range deck {
		card := index[id]
		if card == nil {
			return fmt.Errorf("unknown card %d", id)
		}
		if !format.AllowsPack(card.Meta.Pack) {
			return fmt.Errorf("card %d (%s) is not available in %s", id, format.Name, format.ID)
		}
		switch format.Banlist[id] {
		case "forbidden":
			return fmt.Errorf("card %d is forbidden in %s", id, format.ID)
		case "limited":
			counts[id]++
			if counts[id] > 1 {
				return fmt.Errorf("card %d is limited to one copy in %s", id, format.ID)
			}
		case "semi":
			counts[id]++
			if counts[id] > 2 {
				return fmt.Errorf("card %d is limited to two copies in %s", id, format.ID)
			}
		}
	}
	return nil
}

// FormatsForCard 返回某张卡（按卡包）可以使用的赛制。
func FormatsForCard(cards *ir.CardPack, card *ir.Card) []string {
	if card == nil {
		return nil
	}
	ids := []string{}
	for _, format := range Formats(cards) {
		if format.AllowsPack(card.Meta.Pack) {
			ids = append(ids, format.ID)
		}
	}
	return ids
}
