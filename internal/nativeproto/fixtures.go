package nativeproto

import (
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// 引导阶段的响应夹具。
//
// 来源：delta 的 research/native-offline/startup-fixtures.json。它的 provenance 里写明
// 是"本地重建的响应"——字段布局取自原客户端 MessagePack 序列化器与 DTO 注册，编号与
// 成功码取自客户端代码；**不是抓到的官方响应**。
//
// 它们的用途是让客户端跨过启动阶段（版本查询、标题、账号初始化……）。真正需要引擎
// 参与的是之后的对局路由，不在这里。
//
//go:embed contracts/startup-fixtures.json
var fixtureData embed.FS

// Fixtures 是按路径索引的响应模板。
type Fixtures struct {
	provenance string
	responses  map[string]map[string]any
}

// LoadFixtures 读取内置夹具。
func LoadFixtures() (*Fixtures, error) {
	raw, err := fixtureData.ReadFile("contracts/startup-fixtures.json")
	if err != nil {
		return nil, err
	}
	return ParseFixtures(strings.NewReader(string(raw)))
}

// ParseFixtures 从任意来源读取夹具，便于换版本或做实验时替换。
func ParseFixtures(reader io.Reader) (*Fixtures, error) {
	var document struct {
		Provenance string                    `json:"provenance"`
		Responses  map[string]map[string]any `json:"responses"`
	}
	decoder := json.NewDecoder(reader)
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("nativeproto: 解析响应夹具失败: %w", err)
	}
	if len(document.Responses) == 0 {
		return nil, fmt.Errorf("nativeproto: 响应夹具里没有任何路由")
	}
	for route, response := range document.Responses {
		if _, ok := response["data_headers"]; !ok {
			return nil, fmt.Errorf("nativeproto: 夹具 %s 缺少 data_headers", route)
		}
		if _, ok := response["data"]; !ok {
			return nil, fmt.Errorf("nativeproto: 夹具 %s 缺少 data", route)
		}
	}
	return &Fixtures{provenance: document.Provenance, responses: document.Responses}, nil
}

// Provenance 返回夹具的来源说明，用于在日志里如实交代数据从哪来。
func (f *Fixtures) Provenance() string { return f.provenance }

// Routes 返回夹具覆盖的全部路径。
func (f *Fixtures) Routes() []string {
	routes := make([]string, 0, len(f.responses))
	for route := range f.responses {
		routes = append(routes, route)
	}
	return routes
}

// Lookup 取一条响应模板。客户端请求的路径带前缀（`/cygames/Version/info`），
// 而夹具的键是去掉前缀的路由名（`/Version/info`），所以这里要归一化。
//
// 返回的是深拷贝：调用方会往里塞运行期字段（比如 servertime），不能改到模板本身。
func (f *Fixtures) Lookup(path string) (map[string]any, bool) {
	route := NormalizeRoute(path)
	response, ok := f.responses[route]
	if !ok {
		return nil, false
	}
	cloned, err := deepCopy(response)
	if err != nil {
		return nil, false
	}
	return cloned, true
}

// CompleteHeaders 把 DataHeader 里夹具没覆盖到的字段补上默认值。
//
// 为什么必须补：夹具是从 delta 那份本地重建的数据来的，它比 1.9.5 的
// `Wizard2.Domain.DataHeader` 少了几个字段（maintenance / restriction /
// maintenance_notification / store_url / card_collection_notification）。客户端的
// MessagePack 反序列化是全字段的，少一个就整条响应解析失败，日志里只有一句
// "Failed to deserialize Wizard2.Domain.XxxResponse value."——表现是"客户端悄悄退回
// 标题"，极难查。默认值取"没有公告、没有维护、没有限制"。
func CompleteHeaders(headers map[string]any) {
	defaults := map[string]any{
		"result_code":                   1,
		"sid":                           "",
		"servertime":                    int64(0),
		"maintenance":                   nil,
		"maintenance_task_ids":          []any{},
		"restriction":                   nil,
		"achievement_notification":      []any{},
		"mission_notification":          []any{},
		"mission_complete_notification": []any{},
		"maintenance_notification":      nil,
		"store_url":                     "",
		"card_collection_notification":  nil,
		"owned_base_card_ids":           []any{},
		"festival_notification":         []any{},
		"is_streamer_mode":              false,
	}
	for key, value := range defaults {
		if _, ok := headers[key]; !ok {
			headers[key] = value
		}
	}
}

// NormalizeRoute 去掉客户端请求路径里的 API 前缀。
//
// 实测客户端请求的是 `/cygames/<route>`（例如 `/cygames/Version/info`），而契约与夹具
// 都按 `<route>` 记录。契约数据本身不带前缀，所以这一步是必需的而不是权宜之计。
func NormalizeRoute(path string) string {
	for _, prefix := range []string{"/cygames", "/api"} {
		if strings.HasPrefix(path, prefix) {
			trimmed := strings.TrimPrefix(path, prefix)
			if strings.HasPrefix(trimmed, "/") {
				return trimmed
			}
		}
	}
	return path
}

func deepCopy(value map[string]any) (map[string]any, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var cloned map[string]any
	// 用 UseNumber 读，避免大整数先被折成 float64；但必须在编码成 MessagePack 之前
	// 换回具体数值类型——json.Number 本质是字符串，直接编码出去会变成字符串，
	// 客户端拿到的就是 "1" 而不是 1。
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.UseNumber()
	if err := decoder.Decode(&cloned); err != nil {
		return nil, err
	}
	normalizeNumbers(cloned)
	return cloned, nil
}

// normalizeNumbers 就地把 json.Number 换成 int64（能整除时）或 float64。
func normalizeNumbers(value any) {
	switch typed := value.(type) {
	case map[string]any:
		for key, item := range typed {
			typed[key] = convertNumber(item)
			normalizeNumbers(typed[key])
		}
	case []any:
		for index, item := range typed {
			typed[index] = convertNumber(item)
			normalizeNumbers(typed[index])
		}
	}
}

func convertNumber(value any) any {
	number, ok := value.(json.Number)
	if !ok {
		return value
	}
	if integer, err := number.Int64(); err == nil {
		return integer
	}
	if float, err := number.Float64(); err == nil {
		return float
	}
	return value
}
