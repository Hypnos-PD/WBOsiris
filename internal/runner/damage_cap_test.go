package runner

import (
	"testing"
)

// 伤害上限（damage_cap N）：单次受到的伤害最多为 N，等价于卡面的「受到的 N+1 点或以上的伤害变为 N 点」。
func TestDamageCapClampsInstanceDamage(t *testing.T) {
	g := &game{}
	target := &instance{id: "target", zone: "field", life: 10, damageCap: 3, abilities: map[string]bool{}}
	g.own.field = []*instance{target}
	g.instances = map[string]*instance{"target": target}
	if got := g.damageInstance(target, 8); got != 3 || target.life != 7 {
		t.Fatalf("capped damage = %d, life=%d; want 3/7", got, target.life)
	}
	if got := g.damageInstance(target, 3); got != 3 || target.life != 4 {
		t.Fatalf("damage at the cap = %d, life=%d; want 3/4", got, target.life)
	}
	if got := g.damageInstance(target, 2); got != 2 || target.life != 2 {
		t.Fatalf("damage below the cap = %d, life=%d; want 2/2", got, target.life)
	}
}

// 主战者「受到的伤害 +1」：每次受到的伤害在其它修正之前先加一。
func TestLeaderDamageTakenUpAddsOne(t *testing.T) {
	g := &game{}
	g.oppo.leaderLife = 20
	g.setLeaderKeyword([]string{"oppo"}, "damage_taken_up", true, "")
	if got := g.damageLeaderFrom(nil, &g.oppo, "oppo", 5); got != 6 || g.oppo.leaderLife != 14 {
		t.Fatalf("raised leader damage = %d, life=%d; want 6/14", got, g.oppo.leaderLife)
	}
	if got := g.damageLeaderFrom(nil, &g.oppo, "oppo", 1); got != 2 || g.oppo.leaderLife != 12 {
		t.Fatalf("raised leader damage = %d, life=%d; want 2/12", got, g.oppo.leaderLife)
	}
}

// 屏障与「受到的伤害 +1」同时存在时，下一次伤害仍被整体降为 0（官方 QA）。
func TestLeaderDamageTakenUpIsNeutralizedByBarrier(t *testing.T) {
	g := &game{}
	g.own.leaderLife = 20
	g.setLeaderKeyword([]string{"own"}, "damage_taken_up", true, "")
	g.setLeaderKeyword([]string{"own"}, "barrier", true, "")
	if got := g.damageLeaderFrom(nil, &g.own, "own", 5); got != 0 || g.own.leaderLife != 20 {
		t.Fatalf("barrier did not absorb the raised damage: dealt=%d life=%d", got, g.own.leaderLife)
	}
	if got := g.damageLeaderFrom(nil, &g.own, "own", 5); got != 6 || g.own.leaderLife != 14 {
		t.Fatalf("second hit = %d, life=%d; want 6/14", got, g.own.leaderLife)
	}
}
