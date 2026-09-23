package nativeproto

import (
	"reflect"
	"testing"

	"github.com/vmihailenco/msgpack/v5"
)

func TestLoadSchemas(t *testing.T) {
	schemas, err := LoadSchemas()
	if err != nil {
		t.Fatalf("内置布局应当能加载: %v", err)
	}
	if len(schemas.Names()) < 100 {
		t.Fatalf("布局数量太少：%d", len(schemas.Names()))
	}
	// 布局来自哪份二进制必须对得上，否则字段布局可能整体错位。
	if schemas.SourceSHA256() != "363d45dd94d9ad940bfcd8ead534657430f10ce8097170b129dfcde6c0cab3a3" {
		t.Fatalf("布局的 sourceSHA256 与预期构建不符：%s", schemas.SourceSHA256())
	}
}

// 布局的结构完整性：每个字段都必须落在自己的槽位范围内，槽位不能重复。
// 这类错误在运行期表现是"客户端解析出乱码"，很难定位，所以在这里挡住。
func TestEverySchemaIsStructurallySane(t *testing.T) {
	schemas, err := LoadSchemas()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range schemas.Names() {
		schema, ok := schemas.Lookup(name)
		if !ok {
			t.Fatalf("%s 按全名查不到", name)
		}
		seen := map[int]string{}
		for _, field := range schema.Fields {
			if field.Slot < 0 || field.Slot >= schema.ArrayLength {
				t.Errorf("%s.%s 槽位 %d 越界（长度 %d）", name, field.Name, field.Slot, schema.ArrayLength)
			}
			if previous, clash := seen[field.Slot]; clash {
				t.Errorf("%s 槽位 %d 被 %s 与 %s 同时占用", name, field.Slot, previous, field.Name)
			}
			seen[field.Slot] = field.Name
		}
	}
}

func TestObjFillsSlotsAndDefaults(t *testing.T) {
	schemas, err := LoadSchemas()
	if err != nil {
		t.Fatal(err)
	}
	// BattleHandCardMpo 的槽位在 dump.cs 里核对过：0=UniqueId 1=CardId 2=Cost …
	obj, err := schemas.Obj("BattleHandCardMpo", map[string]any{"UniqueId": 7, "CardId": 10001110})
	if err != nil {
		t.Fatalf("应当能按布局构造: %v", err)
	}
	schema, _ := schemas.Lookup("BattleHandCardMpo")
	if len(obj) != schema.ArrayLength {
		t.Fatalf("构造出来的长度应当是 %d，实际 %d", schema.ArrayLength, len(obj))
	}
	slots := map[string]int{}
	for _, field := range schema.Fields {
		slots[field.Name] = field.Slot
	}
	if obj[slots["UniqueId"]] != 7 {
		t.Errorf("UniqueId 没落在自己的槽位上：%v", obj[slots["UniqueId"]])
	}
	if obj[slots["CardId"]] != 10001110 {
		t.Errorf("CardId 没落在自己的槽位上：%v", obj[slots["CardId"]])
	}
	// 没给的字段应当是"同形的空值"，而不是缺位。
	if cost := obj[slots["Cost"]]; cost != 0 {
		t.Errorf("未提供的 int 字段应当是 0，实际 %v", cost)
	}
	if _, isSlice := obj[slots["EnhanceCosts"]].([]any); !isSlice {
		t.Errorf("未提供的数组字段应当是空数组，实际 %v", obj[slots["EnhanceCosts"]])
	}
}

func TestObjRejectsUnknownFields(t *testing.T) {
	schemas, err := LoadSchemas()
	if err != nil {
		t.Fatal(err)
	}
	// 字段名写错时必须报错：静默忽略会让客户端收到缺字段的结构。
	if _, err := schemas.Obj("BattleHandCardMpo", map[string]any{"CardID": 1}); err == nil {
		t.Fatal("未知字段应当被拒绝")
	}
	if _, err := schemas.Obj("NoSuchType", nil); err == nil {
		t.Fatal("未知类型应当被拒绝")
	}
}

func TestObjSurvivesMessagePackRoundTrip(t *testing.T) {
	schemas, err := LoadSchemas()
	if err != nil {
		t.Fatal(err)
	}
	values := map[string]any{"UniqueId": 1, "CardId": 42, "Cost": 3}
	obj, err := schemas.Obj("BattleHandCardMpo", values)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := msgpack.Marshal(obj)
	if err != nil {
		t.Fatal(err)
	}
	var back []any
	if err := msgpack.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	if len(back) != len(obj) {
		t.Fatalf("往返后长度变了：%d != %d", len(back), len(obj))
	}
	// 逐槽位核对：MessagePack 往返之后不能错位。
	for index, value := range back {
		if !sameValue(value, obj[index]) {
			t.Fatalf("槽位 %d 往返后不一致：%v != %v", index, value, obj[index])
		}
	}
}

// sameValue 比较两份"槽位值"。MessagePack 往返会把具体数值类型归一化
// （例如 int → int8），所以只比较数值与结构，不比较静态类型。
func sameValue(a, b any) bool {
	toFloat := func(value any) (float64, bool) {
		switch typed := value.(type) {
		case int:
			return float64(typed), true
		case int8:
			return float64(typed), true
		case int16:
			return float64(typed), true
		case int32:
			return float64(typed), true
		case int64:
			return float64(typed), true
		case uint8:
			return float64(typed), true
		case uint16:
			return float64(typed), true
		case uint32:
			return float64(typed), true
		case uint64:
			return float64(typed), true
		case float32:
			return float64(typed), true
		case float64:
			return typed, true
		}
		return 0, false
	}
	if left, ok := toFloat(a); ok {
		right, ok := toFloat(b)
		return ok && left == right
	}
	switch left := a.(type) {
	case nil:
		return b == nil
	case []any:
		right, ok := b.([]any)
		if !ok || len(left) != len(right) {
			return false
		}
		for index := range left {
			if !sameValue(left[index], right[index]) {
				return false
			}
		}
		return true
	case map[string]any:
		right, ok := b.(map[string]any)
		if !ok || len(left) != len(right) {
			return false
		}
		for key, value := range left {
			other, ok := right[key]
			if !ok || !sameValue(value, other) {
				return false
			}
		}
		return true
	case []byte:
		right, ok := b.([]byte)
		if !ok || len(left) != len(right) {
			return false
		}
		for index := range left {
			if left[index] != right[index] {
				return false
			}
		}
		return true
	}
	// 剩下的只可能是可比较的标量（string、bool、具体数值未命中 toFloat 的情况等）。
	// 用反射兜底，避免遇到切片/映射类不可比较类型时直接 panic。
	if reflect.TypeOf(a) != reflect.TypeOf(b) {
		return false
	}
	if a == nil {
		return b == nil
	}
	if !reflect.TypeOf(a).Comparable() {
		return false
	}
	return a == b
}
