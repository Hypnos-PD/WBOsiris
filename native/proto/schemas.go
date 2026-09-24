package nativeproto

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// 客户端 DTO 的序列化布局。
//
// 原客户端的 MessagePack 对象**不是**按字段名编码的字典，而是**按槽位排列的数组**：
// 每个 DTO 的每个字段在数组里都有固定位置，位置由客户端自己的序列化器决定。所以给
// 客户端造响应时，必须严格按这个布局填值——填错了它解析出来的就是错位的结构。
//
// 这份布局来自 delta 的 research/protocol-contract/object-schemas.json：字段偏移取自
// 原生注册表，槽位顺序取自生成的序列化代码实际读写的顺序（见文件里的 limits/provenance）。
//
//go:embed contracts/object-schemas.json
var schemaData embed.FS

// Field 是 DTO 里的一个字段。
type Field struct {
	Slot     int    `json:"slot"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	Offset   string `json:"offset"`
	Reserved bool   `json:"reserved"`
}

// Schema 是一个 DTO 的布局。
type Schema struct {
	Type        string `json:"type"`
	Status      string `json:"status"`
	Limitation  string `json:"limitation"`
	ArrayLength int    `json:"-"`
	ArrayHeader struct {
		Length int `json:"length"`
	} `json:"arrayHeader"`
	Fields []Field `json:"fields"`
}

// Schemas 是按类型名索引的布局集合，支持用短名（去掉命名空间）查找。
type Schemas struct {
	sourceSHA256 string
	byFull       map[string]*Schema
	byShort      map[string]string
}

// LoadSchemas 读取内置布局数据。
func LoadSchemas() (*Schemas, error) {
	raw, err := schemaData.ReadFile("contracts/object-schemas.json")
	if err != nil {
		return nil, err
	}
	return ParseSchemas(bytes.NewReader(raw))
}

// ParseSchemas 从任意来源读取布局数据。
func ParseSchemas(reader *bytes.Reader) (*Schemas, error) {
	var document struct {
		SourceSHA256 string             `json:"sourceSHA256"`
		Schemas      map[string]*Schema `json:"schemas"`
	}
	if err := json.NewDecoder(reader).Decode(&document); err != nil {
		return nil, fmt.Errorf("nativeproto: 解析 DTO 布局失败: %w", err)
	}
	if len(document.Schemas) == 0 {
		return nil, fmt.Errorf("nativeproto: DTO 布局为空")
	}
	schemas := &Schemas{
		sourceSHA256: document.SourceSHA256,
		byFull:       document.Schemas,
		byShort:      make(map[string]string, len(document.Schemas)),
	}
	for full, schema := range document.Schemas {
		schema.ArrayLength = schema.ArrayHeader.Length
		if schema.ArrayLength <= 0 {
			return nil, fmt.Errorf("nativeproto: %s 的槽位数量无效（%d）", full, schema.ArrayLength)
		}
		for _, field := range schema.Fields {
			if field.Slot < 0 || field.Slot >= schema.ArrayLength {
				return nil, fmt.Errorf("nativeproto: %s 的字段 %s 槽位越界（%d）", full, field.Name, field.Slot)
			}
		}
		short := full
		if index := strings.LastIndex(full, "."); index >= 0 {
			short = full[index+1:]
		}
		if existing, clash := schemas.byShort[short]; clash && existing != full {
			// 短名撞车不算错——调用方可以用全名。但不要静默覆盖，否则按短名取值
			// 会随加载顺序变化。
			schemas.byShort[short] = ""
			continue
		}
		schemas.byShort[short] = full
	}
	return schemas, nil
}

// SourceSHA256 是这批布局所依据的 GameAssembly.dll 哈希。
func (s *Schemas) SourceSHA256() string { return s.sourceSHA256 }

// Names 返回全部类型名（全名），便于诊断与覆盖度统计。
func (s *Schemas) Names() []string {
	names := make([]string, 0, len(s.byFull))
	for name := range s.byFull {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Lookup 按全名或短名取布局。
func (s *Schemas) Lookup(name string) (*Schema, bool) {
	if schema, ok := s.byFull[name]; ok {
		return schema, true
	}
	if full, ok := s.byShort[name]; ok && full != "" {
		return s.byFull[full], true
	}
	return nil, false
}

// Obj 按布局把一个"字段名 → 值"的映射填成**槽位数组**。
//
// 未提供的字段按类型给默认值；提供了布局里没有的字段则报错——静默忽略会让客户端
// 收到一个缺字段的结构，排查起来远比直接报错痛苦。
func (s *Schemas) Obj(name string, values map[string]any) ([]any, error) {
	schema, ok := s.Lookup(name)
	if !ok {
		return nil, fmt.Errorf("nativeproto: 没有 %s 的布局", name)
	}
	result := make([]any, schema.ArrayLength)
	used := make(map[string]bool, len(values))
	for _, field := range schema.Fields {
		if value, ok := values[field.Name]; ok {
			result[field.Slot] = value
			used[field.Name] = true
			continue
		}
		result[field.Slot] = DefaultValue(field)
	}
	if len(used) != len(values) {
		var unknown []string
		for key := range values {
			if !used[key] {
				unknown = append(unknown, key)
			}
		}
		sort.Strings(unknown)
		return nil, fmt.Errorf("nativeproto: %s 没有这些字段: %s", name, strings.Join(unknown, ", "))
	}
	return result, nil
}

// DefaultValue 按字段类型给出默认值。语义照抄 delta 的 NativeSchema.default：
// 保留位与可空类型是 nil，容器是空容器，数值/枚举是 0，时间是当前时间戳。
//
// 这套取值的意义在于"结构与真实响应同形"——字段缺省时客户端拿到的是空值而不是
// 缺位，不会因为数组长度不对而解析失败。
func DefaultValue(field Field) any {
	if field.Reserved {
		return nil
	}
	kind := field.Type
	switch {
	case kind == "bool":
		return false
	case kind == "string":
		return ""
	case kind == "System.DateTime":
		return time.Now().UTC()
	}
	outer := kind
	if index := strings.IndexByte(outer, '<'); index >= 0 {
		outer = outer[:index]
	}
	switch {
	case strings.Contains(outer, "Nullable`"):
		return nil
	case strings.Contains(outer, "Dictionary`"):
		return map[string]any{}
	case strings.HasSuffix(kind, "[]"),
		strings.Contains(outer, "List`"),
		strings.Contains(outer, "Collection`"),
		strings.Contains(outer, "Set`"):
		return []any{}
	}
	switch kind {
	case "int", "long", "uint", "ulong", "byte", "sbyte", "short", "ushort", "float", "double":
		return 0
	}
	if strings.Contains(kind, ".Primitives.") || strings.Contains(kind, ".Enums.") {
		return 0
	}
	return nil
}
