// Package deckcode 实现"官网卡组 hash"式的卡组分享码。
//
// 规则与 WBArts 的 js/deck.js（getDeckHash）一致：把卡组写成
// `格式.职业.卡牌编码.卡牌编码…`，卡牌编码是卡牌编号的 URL 安全 base64
// （字母表 0-9A-Za-z-_，高位在前、无补位），同一张卡有几张就出现几次。
// 因此我们的码可以直接贴到官方卡组详情页（?hash=…），也能解官方/ WBArts
// 生成出来的码。
package deckcode

import (
	"fmt"
	"sort"
	"strings"

	"wbo/internal/engine/runner"
)

// alphabet 是官网用的 base64 变体（与标准 base64 的 +/ 换成 -_）。
const alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz-_"

// 职业编号与游戏一致：0 中立、1 精灵、2 皇家护卫、3 巫师、4 龙族、5 梦魇、
// 6 主教、7 超越者。
var classIDs = map[string]int{
	"neutral":     0,
	"forestcraft": 1,
	"swordcraft":  2,
	"runecraft":   3,
	"dragoncraft": 4,
	"abysscraft":  5,
	"havencraft":  6,
	"portalcraft": 7,
}

var classNames = map[int]string{
	0: "neutral", 1: "forestcraft", 2: "swordcraft", 3: "runecraft",
	4: "dragoncraft", 5: "abysscraft", 6: "havencraft", 7: "portalcraft",
}

// MaxCards 限制解出来的张数上限，避免畸形码把内存撑爆（正式卡组是 40 张，
// 客端还可能保存未完成的草稿，所以留出余量）。
const MaxCards = 200

// ClassID 返回职业的官方编号。
func ClassID(name string) (int, bool) {
	id, ok := classIDs[name]
	return id, ok
}

// ClassName 返回官方编号对应的引擎职业名。
func ClassName(id int) (string, bool) {
	name, ok := classNames[id]
	return name, ok
}

// Deck 是解码结果。Cards 按升序排列，每张卡出现一次。
type Deck struct {
	Format string
	Class  string
	Cards  []int
}

// Encode 把卡组编成分享码。class 是引擎职业名（neutral / forestcraft / …）。
func Encode(format, class string, cards []int) (string, error) {
	if len(cards) > MaxCards {
		return "", fmt.Errorf("deck has %d cards, limit is %d", len(cards), MaxCards)
	}
	if class == "" {
		class = "neutral"
	}
	classID, ok := ClassID(class)
	if !ok {
		return "", fmt.Errorf("unknown class %q", class)
	}
	parts := []string{formatDigit(format), fmt.Sprint(classID)}
	sorted := append([]int(nil), cards...)
	sort.Ints(sorted)
	for _, id := range sorted {
		if id <= 0 {
			return "", fmt.Errorf("invalid card id %d", id)
		}
		parts = append(parts, encodeNumber(id))
	}
	return strings.Join(parts, "."), nil
}

// ClassForDeck 按牌组内容推断职业：取第一张非中立卡的职业，全是中立则为 neutral。
func ClassForDeck(cards []int, classOf func(int) string) string {
	class := "neutral"
	if classOf == nil {
		return class
	}
	for _, id := range cards {
		if name := classOf(id); name != "" && name != "neutral" {
			return name
		}
	}
	return class
}

// Decode 解析分享码。既接受裸码，也接受官方卡组详情链接（从中取 hash 参数）。
func Decode(raw string) (Deck, error) {
	code := strings.TrimSpace(raw)
	if code == "" {
		return Deck{}, fmt.Errorf("empty deck code")
	}
	if index := strings.Index(code, "hash="); index >= 0 {
		code = code[index+len("hash="):]
		if cut := strings.IndexAny(code, "&#"); cut >= 0 {
			code = code[:cut]
		}
	}
	parts := strings.Split(code, ".")
	if len(parts) < 2 {
		return Deck{}, fmt.Errorf("malformed deck code")
	}
	format, err := formatName(parts[0])
	if err != nil {
		return Deck{}, err
	}
	classID, err := decodeNumber(parts[1])
	if err != nil {
		return Deck{}, fmt.Errorf("class: %w", err)
	}
	class, ok := ClassName(int(classID))
	if !ok {
		return Deck{}, fmt.Errorf("unknown class id %d", classID)
	}
	cards := make([]int, 0, len(parts)-2)
	for _, part := range parts[2:] {
		value, err := decodeNumber(part)
		if err != nil {
			return Deck{}, fmt.Errorf("card %q: %w", part, err)
		}
		if value <= 0 {
			return Deck{}, fmt.Errorf("invalid card id %d", value)
		}
		cards = append(cards, int(value))
		if len(cards) > MaxCards {
			return Deck{}, fmt.Errorf("deck code has more than %d cards", MaxCards)
		}
	}
	sort.Ints(cards)
	return Deck{Format: format, Class: class, Cards: cards}, nil
}

func formatDigit(format string) string {
	if format == runner.FormatUnlimited {
		return "2"
	}
	return "1"
}

func formatName(digit string) (string, error) {
	switch digit {
	case "1", "":
		return runner.FormatRotation, nil
	case "2":
		return runner.FormatUnlimited, nil
	}
	return "", fmt.Errorf("unknown format digit %q", digit)
}

func encodeNumber(value int) string {
	if value == 0 {
		return string(alphabet[0])
	}
	encoded := ""
	for value > 0 {
		encoded = string(alphabet[value%64]) + encoded
		value >>= 6
	}
	return encoded
}

func decodeNumber(text string) (int64, error) {
	if text == "" {
		return 0, fmt.Errorf("empty number")
	}
	var value int64
	for _, r := range text {
		index := strings.IndexRune(alphabet, r)
		if index < 0 {
			return 0, fmt.Errorf("invalid character %q", r)
		}
		value = value*64 + int64(index)
		if value > 1<<31 {
			return 0, fmt.Errorf("number out of range")
		}
	}
	return value, nil
}
