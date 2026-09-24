package nativeprofile

import (
	"fmt"
)

// CardStyles 是"卡面样式"这一层：同一张卡的不同印刷体/样式资源。
//
// 照 delta 的 tools/local_native_styles.py 搬：样式与 CardId、闪卡类型是两件事，
// 也不能改变卡的规则。校验用的是**已核实过资源存在**的样式表——客户端拿一个没有
// 对应资源的样式去渲染会出问题，所以这里宁可拒绝。
type CardStyles struct {
	// byCard 是"这张印刷体可用的样式"；byNormal 是"这一族（normal id）可用的样式"。
	byCard   map[int64][]int64
	byNormal map[int64][]int64
}

type stylesFile struct {
	Schema               int                `json:"schema"`
	OriginalClientSHA256 string             `json:"originalClientSHA256"`
	CardOptions          map[string][]int64 `json:"cardOptions"`
	FamilyOptions        map[string][]int64 `json:"familyOptions"`
}

func loadStyles(raw []byte, clientSHA string) (*CardStyles, error) {
	var file stylesFile
	if err := decodeInto(raw, &file); err != nil {
		return nil, fmt.Errorf("nativeprofile: 解析卡面样式表失败: %w", err)
	}
	if file.Schema != 1 {
		return nil, fmt.Errorf("nativeprofile: 卡面样式表 schema 是 %d，不认识", file.Schema)
	}
	if file.OriginalClientSHA256 != clientSHA {
		return nil, fmt.Errorf("nativeprofile: 卡面样式表不是这份客户端提取的（%s != %s）",
			file.OriginalClientSHA256, clientSHA)
	}
	styles := &CardStyles{
		byCard:   map[int64][]int64{},
		byNormal: map[int64][]int64{},
	}
	for key, values := range file.CardOptions {
		index, err := parseKey(key)
		if err != nil {
			return nil, err
		}
		styles.byCard[index] = values
	}
	for key, values := range file.FamilyOptions {
		index, err := parseKey(key)
		if err != nil {
			return nil, err
		}
		styles.byNormal[index] = values
	}
	return styles, nil
}

// Options 返回这张印刷体可用的样式；没有任何记录时按 delta 的写法回 [0]（默认样式）。
func (s *CardStyles) Options(id int64) []int64 {
	if values, ok := s.byCard[id]; ok {
		return values
	}
	return []int64{0}
}

func (s *CardStyles) family(normal int64) []int64 {
	if values, ok := s.byNormal[normal]; ok {
		return values
	}
	return []int64{0}
}

// Validate 判断"给这张卡设这个样式"是不是站得住。
func (s *CardStyles) Validate(card Card, style int64) error {
	if _, known := s.byCard[card.ID]; !known {
		return fmt.Errorf("未知的卡面")
	}
	if style < 0 {
		return fmt.Errorf("样式号不合法")
	}
	if !containsInt(s.family(card.Normal), style) {
		return fmt.Errorf("这个样式没有被核实过")
	}
	// 高闪（foil==2）的印刷体本来就没有独立的样式资源行，它用自己的 master 资源；
	// delta 的证据里，客户端仍然会把这一族的偏好套到它身上，所以这一支放行。
	if !containsInt(s.Options(card.ID), style) && card.Foil != 2 {
		return fmt.Errorf("这个印刷体没有对应的样式资源")
	}
	return nil
}

// Defaults 把存档里的样式偏好（按 normal id 归并）整理出来。
func (s *CardStyles) Defaults(cards map[int64]Card, saved map[string]int64) (map[int64]int64, error) {
	result := map[int64]int64{}
	for key, value := range saved {
		index, err := parseKey(key)
		if err != nil {
			return nil, err
		}
		card, ok := cards[index]
		if !ok {
			return nil, fmt.Errorf("存档里的样式指向不存在的卡 %d", index)
		}
		if err := s.Validate(card, value); err != nil {
			return nil, err
		}
		if existing, ok := result[card.Normal]; ok && existing != value {
			return nil, fmt.Errorf("同一族里有两个互相矛盾的样式偏好")
		}
		result[card.Normal] = value
	}
	return result, nil
}

// Overrides 整理一副牌里逐张指定过的样式；同一族里出现两个不同样式就是错的。
func (s *CardStyles) Overrides(cards map[int64]Card, entries []map[string]any) (map[int64]int64, error) {
	result := map[int64]int64{}
	for _, entry := range entries {
		style, present := entry["style"]
		if !present || style == nil {
			continue
		}
		value, err := asInt(style)
		if err != nil {
			return nil, err
		}
		id, err := asInt(entry["id"])
		if err != nil {
			return nil, err
		}
		card, ok := cards[id]
		if !ok {
			return nil, fmt.Errorf("牌组里的样式指向不存在的卡 %d", id)
		}
		if err := s.Validate(card, value); err != nil {
			return nil, err
		}
		if existing, ok := result[card.Normal]; ok && existing != value {
			return nil, fmt.Errorf("同一族里有两个互相矛盾的牌组样式")
		}
		result[card.Normal] = value
	}
	return result, nil
}

func containsInt(values []int64, want int64) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
