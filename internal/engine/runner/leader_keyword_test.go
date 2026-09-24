package runner

import "testing"

// S-51：主战者级关键词的期限（"到对手的回合结束为止"）与"受到的伤害变为0"。
func TestLeaderTemporaryKeywordDeadlines(t *testing.T) {
	for _, deadline := range []string{"own", "oppo"} {
		s, _, _, _ := historySession(t, "own", 7)
		s.g.setLeaderKeyword([]string{"own"}, "damage_to_zero", true, deadline)
		if !s.g.own.leaderAbilities["damage_to_zero"] {
			t.Fatal("leader keyword was not granted")
		}
		// 另一个时点结束时不失效。
		s.g.expireTurnEffects(oppositeSide(deadline))
		if !s.g.own.leaderAbilities["damage_to_zero"] {
			t.Fatalf("leader keyword expired too early (%s)", deadline)
		}
		s.g.expireTurnEffects(deadline)
		if s.g.own.leaderAbilities["damage_to_zero"] || len(s.g.own.leaderTemporary) != 0 {
			t.Fatalf("leader keyword did not expire at %s", deadline)
		}
	}
}

func TestLeaderDamageToZeroNegatesDamage(t *testing.T) {
	s, _, _, _ := historySession(t, "own", 11)
	s.g.own.leaderLife = 20
	s.g.own.leaderAbilities = map[string]bool{"damage_to_zero": true}
	if actual := s.g.damageLeader(&s.g.own, "own", 5); actual != 0 || s.g.own.leaderLife != 20 {
		t.Fatalf("damage was not negated: actual=%d life=%d", actual, s.g.own.leaderLife)
	}
	// 0 点伤害保持 0；【屏障】不会被这次归零消耗。
	s.g.own.leaderAbilities["barrier"] = true
	if actual := s.g.damageLeader(&s.g.own, "own", 0); actual != 0 {
		t.Fatalf("zero damage changed: %d", actual)
	}
	if !s.g.own.leaderAbilities["barrier"] {
		t.Fatal("barrier was consumed by negated damage")
	}
	delete(s.g.own.leaderAbilities, "damage_to_zero")
	if actual := s.g.damageLeader(&s.g.own, "own", 5); actual != 0 || s.g.own.leaderLife != 20 || s.g.own.leaderAbilities["barrier"] {
		t.Fatalf("barrier did not absorb the follow-up damage: actual=%d", actual)
	}
}
