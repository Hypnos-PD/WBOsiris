package project

import (
	"testing"

	"wbo/internal/engine/ir"
)

// `count(集合 other)` 统计时排除来源自身；`set attack T N` 直接设置攻击力；
// `self.damage_taken` 读本实例已受伤害；`destroy 集合 other` 排除来源自身。
func TestCountOtherSetAttackAndDamageTaken(t *testing.T) {
	pack, ds := compile(t, validCard(`
		fanfare {
			buff self +count(own.field other)/+count(own.field other);
			destroy own.field other;
			set attack oppo.field.followers 4;
			heal self self.damage_taken;
		}`))
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	body := pack.Cards[0].Abilities[0].Body
	buff := body[0].(ir.TargetEffect)
	count, ok := buff.AttackExpr.(*ir.CountExpr)
	if !ok {
		t.Fatalf("count amount not preserved: %#v", buff.AttackExpr)
	}
	exclude, ok := count.Source.(ir.ExcludeRef)
	if !ok {
		t.Fatalf("count did not keep the exclusion: %#v", count.Source)
	}
	if _, ok := exclude.Value.(ir.SelfRef); !ok {
		t.Fatalf("count other must exclude self: %#v", exclude.Value)
	}
	destroy := body[1].(ir.TargetEffect)
	if _, ok := destroy.Target.(ir.ExcludeRef); !ok {
		t.Fatalf("destroy other did not compile: %#v", destroy.Target)
	}
	setAttack := body[2].(ir.TargetEffect)
	if setAttack.Kind != "set_attack" || setAttack.Amount != 4 {
		t.Fatalf("set attack not preserved: %#v", setAttack)
	}
	heal := body[3].(ir.TargetEffect)
	scalar, ok := heal.AmountExpr.(*ir.Scalar)
	if !ok || scalar.Kind != "self_scalar" || scalar.Field != "damage_taken" {
		t.Fatalf("damage taken scalar not preserved: %#v", heal.AmountExpr)
	}
}

func TestCountOtherAndSetAttackRejectBadShapes(t *testing.T) {
	for _, line := range []string{
		"buff self +count(own.field other)/+0 extra;",
		"set attack oppo.field.followers;",
		"if self.damage_taken { draw 1; }",
	} {
		if _, ds := compile(t, validCard("fanfare { "+line+" }")); len(ds) == 0 {
			t.Fatalf("%q must not compile", line)
		}
	}
}
