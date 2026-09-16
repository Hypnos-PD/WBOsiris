package project

import (
	"strings"
	"testing"

	"wbo/internal/ir"
)

// 声明了 fusion 的卡牌可以在任意效果块里读取 fused.cost / fused.distinct：
// 法术的结算时判断（"若已与本卡牌融合，则改为抽取 2 张"）必须写在 effect 里，
// 而不是写在空的 fusion 块里。
func TestFusedScalarIsReadableOutsideTheFusionBlock(t *testing.T) {
	pack, ds := compile(t, fusionSpellCard(`effect {
		fusion material from own.hand where class forestcraft {
		}
		if fused.distinct >= 1 {
			draw 2;
		} else {
			draw 1;
		}
	}`))
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	if len(pack.Cards[0].FusionAbilities) != 1 {
		t.Fatalf("fusion ability missing: %#v", pack.Cards[0].FusionAbilities)
	}
	if len(pack.Cards[0].PlayEffects) != 1 {
		t.Fatalf("play effects = %d", len(pack.Cards[0].PlayEffects))
	}
	effect, ok := pack.Cards[0].PlayEffects[0].(ir.IfEffect)
	if !ok {
		t.Fatalf("unexpected effect: %T", pack.Cards[0].PlayEffects[0])
	}
	condition, ok := effect.Condition.(ir.CompareCondition)
	if !ok || condition.Left.Kind != "fusion_material_scalar" || condition.Left.Field != "distinct" || condition.Op != "ge" || condition.Right != 1 {
		t.Fatalf("fused scalar not preserved: %+v", effect.Condition)
	}
	if len(effect.Else) != 1 {
		t.Fatalf("else branch missing: %#v", effect.Else)
	}
}

// 没有声明融合的卡牌读到 fused 时仍然是错误，避免把笔误写成永远为假的条件。
func TestFusedScalarWithoutFusionIsRejected(t *testing.T) {
	for _, effect := range []string{
		`effect { if fused.cost >= 1 { draw 1; } }`,
		`effect { when own turn starts if fused.distinct >= 1 { draw 1; } }`,
	} {
		if _, ds := compile(t, fusionSpellCard(effect)); len(ds) == 0 {
			t.Fatalf("%q must not compile", effect)
		}
	}
}

func fusionSpellCard(effect string) string {
	body := effect
	if !strings.HasPrefix(strings.TrimSpace(body), "effect") {
		body = "effect { " + body + " }"
	}
	return "wbo 0.1.0; card 12345678 { type spell; cost 2; " + body +
		` meta { pack 12345; class neutral; rarity bronze; }` +
		` locale chs { name "x"; text ""; } locale eng { name "x"; text ""; }` +
		` locale jpn { name "x"; text ""; } locale kor { name "x"; text ""; } locale cht { name "x"; text ""; } }`
}
