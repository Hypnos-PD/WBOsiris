package nativeprofile

import (
	"fmt"
	"strconv"
)

// ProfileError 表示请求本身不合法（字段缺失、取值越界、牌组校验不过……）。
// 与"我们还没实现这条路由"区分开：前者要如实拒绝，后者交回未实现。
type ProfileError struct{ Reason string }

func (e ProfileError) Error() string { return e.Reason }

func fail(format string, values ...any) error {
	return ProfileError{Reason: fmt.Sprintf(format, values...)}
}

// asInt 取一个整数。MessagePack/JSON 解出来的整数都是具体类型，这里统一收口，
// 免得每个调用点各写一遍类型断言。
func asInt(value any) (int64, error) {
	switch typed := value.(type) {
	case int:
		return int64(typed), nil
	case int8:
		return int64(typed), nil
	case int16:
		return int64(typed), nil
	case int32:
		return int64(typed), nil
	case int64:
		return typed, nil
	case uint8:
		return int64(typed), nil
	case uint16:
		return int64(typed), nil
	case uint32:
		return int64(typed), nil
	case uint64:
		return int64(typed), nil
	case float64:
		if typed == float64(int64(typed)) {
			return int64(typed), nil
		}
	}
	return 0, fail("需要一个整数，收到 %T", value)
}

// asIntOr 取整数，缺省时用 fallback。
func asIntOr(value any, fallback int64) (int64, error) {
	if value == nil {
		return fallback, nil
	}
	return asInt(value)
}

// asBool 取布尔值。只认真布尔——把 0/1 当 false/true 是客户端那边的事，合约里
// 这些字段就是 bool。
func asBool(value any) (bool, error) {
	typed, ok := value.(bool)
	if !ok {
		return false, fail("需要一个布尔值，收到 %T", value)
	}
	return typed, nil
}

func asString(value any) (string, error) {
	typed, ok := value.(string)
	if !ok {
		return "", fail("需要一个字符串，收到 %T", value)
	}
	return typed, nil
}

// asMap 取一个对象。
func asMap(value any) (map[string]any, error) {
	typed, ok := value.(map[string]any)
	if !ok {
		return nil, fail("需要一个对象，收到 %T", value)
	}
	return typed, nil
}

// asEntries 取一个对象数组（牌组里的 cards 字段）。
func asEntries(value any) ([]map[string]any, error) {
	items, ok := value.([]any)
	if !ok {
		return nil, fail("需要一个数组，收到 %T", value)
	}
	entries := make([]map[string]any, 0, len(items))
	for _, item := range items {
		entry, err := asMap(item)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// parseKey 把 "10001110" 这样的字符串键转成整数。
func parseKey(key string) (int64, error) {
	value, err := strconv.ParseInt(key, 10, 64)
	if err != nil {
		return 0, fail("键 %q 不是整数", key)
	}
	return value, nil
}

// bounded 检查一个整数落在 [low, high] 里。
func bounded(value, low, high int64) error {
	if value < low || value > high {
		return fail("取值 %d 越界（应在 %d..%d）", value, low, high)
	}
	return nil
}
