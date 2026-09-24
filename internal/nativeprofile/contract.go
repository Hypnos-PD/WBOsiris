package nativeprofile

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// decodeInto 解析契约里的 JSON，并把数字归一化成 int64 / float64。
//
// 与 nativeproto 的夹具加载同一个道理：先用 json.Number 保住大整数，再换回具体类型。
// **不能**让 json.Number 溜到 MessagePack 编码那一步——它本质是字符串，客户端会收到
// "1" 而不是 1，然后解析失败。契约里的模板（DeckData、LeaderSkin 之类）全是数字字面量，
// 直接 json.Unmarshal 到 map[string]any 会得到 float64，编出去变成浮点，同样不对。
func decodeInto(raw []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	return nil
}

// decodeMap 解析成 map[string]any，并把数字归一化。
func decodeMap(raw []byte) (map[string]any, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value map[string]any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	normalizeNumbers(value)
	return value, nil
}

// normalizeNumbers 就地把 json.Number 换成 int64（整数）或 float64。
func normalizeNumbers(value any) {
	switch typed := value.(type) {
	case map[string]any:
		for key, item := range typed {
			typed[key] = numberToValue(item)
			normalizeNumbers(typed[key])
		}
	case []any:
		for index, item := range typed {
			typed[index] = numberToValue(item)
			normalizeNumbers(typed[index])
		}
	}
}

func numberToValue(value any) any {
	number, ok := value.(json.Number)
	if !ok {
		return value
	}
	if integer, err := number.Int64(); err == nil {
		return integer
	}
	if floating, err := number.Float64(); err == nil {
		return floating
	}
	return value
}

// copyTemplate 取一份模板的深拷贝。契约里的模板是"该响应的空壳"，每个请求都要一份
// 独立的副本——共享会被跨请求的修改污染。
func (p *Profile) copyTemplate(name string) (map[string]any, error) {
	template, ok := p.contract.Templates[name]
	if !ok {
		return nil, fmt.Errorf("nativeprofile: 契约里没有模板 %s", name)
	}
	raw, err := json.Marshal(template)
	if err != nil {
		return nil, err
	}
	return decodeMap(raw)
}

// copyRouteData 取一条路由的基线响应数据（契约里那条路由的 data 空壳）。
func (p *Profile) copyRouteData(path string) (map[string]any, error) {
	route, ok := p.contract.Routes[path]
	if !ok {
		return nil, fmt.Errorf("nativeprofile: 契约里没有路由 %s", path)
	}
	raw, err := json.Marshal(route.Data)
	if err != nil {
		return nil, err
	}
	return decodeMap(raw)
}

// HandledRoute 报告这条路径是不是档案层负责的路由。
func (p *Profile) HandledRoute(path string) bool {
	_, ok := p.contract.Routes[path]
	return ok
}

// RouteNames 列出档案层覆盖的全部路由（给启动日志用）。
func (p *Profile) RouteNames() []string {
	names := make([]string, 0, len(p.contract.Routes))
	for name := range p.contract.Routes {
		names = append(names, name)
	}
	return names
}
