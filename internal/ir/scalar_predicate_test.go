package ir

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestPlayerScalarPredicateRoundTrip(t *testing.T) {
	for _, side := range []string{"own", "oppo"} {
		for _, field := range []string{"combo", "pp", "maxpp", "life", "ep", "sep", "shadows"} {
			p := FieldPredicate{Kind: "compare", Field: "cost", Op: "eq", ValueScalar: &Scalar{Kind: "scalar", Side: side, Field: field}}
			data, err := json.Marshal(p)
			if err != nil {
				t.Fatal(err)
			}
			decoded, err := decodePredicate(data)
			if err != nil || !reflect.DeepEqual(p, decoded) {
				t.Fatal(string(data), decoded, err)
			}
		}
	}
	// 绑定标量（含原始数值）同样可以参与筛选比较："X 为使用的卡牌的原始费用"。
	for _, field := range []string{"cost", "base_cost"} {
		p := FieldPredicate{Kind: "compare", Field: "base_cost", Op: "eq", ValueScalar: &Scalar{Kind: "binding_scalar", Side: "played", Field: field}}
		data, err := json.Marshal(p)
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := decodePredicate(data)
		if err != nil || !reflect.DeepEqual(p, decoded) {
			t.Fatal(string(data), decoded, err)
		}
	}
}

func TestPlayerScalarPredicateRejectsAmbiguousValues(t *testing.T) {
	for _, value := range []string{
		`null`, `true`, `1.5`, `"own.combo"`,
		`{"kind":"scalar","side":"both","field":"combo"}`,
		`{"kind":"scalar","side":"own","field":"cost"}`,
		`{"kind":"scalar","side":"own","field":"combo","extra":1}`,
		`{"kind":"self_scalar","field":"cost"}`,
		`{"kind":"binding_scalar","side":"played","field":"base_attack2"}`,
		`{"kind":"count","source":{"kind":"zone","side":"own","zone":"hand"}}`,
		`{"kind":"negate","value":{"kind":"scalar","side":"own","field":"combo"}}`,
	} {
		if _, err := decodePredicate([]byte(`{"kind":"compare","field":"cost","op":"eq","value":` + value + `}`)); err == nil {
			t.Fatal("accepted invalid predicate value", value)
		}
	}
	if _, err := decodePredicate([]byte(`{"kind":"compare","field":"cost","op":"eq"}`)); err == nil {
		t.Fatal("missing value silently became zero")
	}
	for _, p := range []FieldPredicate{
		{Kind: "compare", Field: "cost", Op: "eq", Value: 1, ValueScalar: &Scalar{Kind: "scalar", Side: "own", Field: "combo"}},
		{Kind: "compare", Field: "cost", Op: "eq", ValueScalar: &Scalar{Kind: "self_scalar", Field: "cost"}},
	} {
		if _, err := json.Marshal(p); err == nil {
			t.Fatal("encoded unsupported predicate", p)
		}
	}
}
