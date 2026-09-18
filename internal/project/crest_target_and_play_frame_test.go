package project

import "testing"

// S-43：`own.crests` / `oppo.crests` 可以作为效果目标集合（例如"破坏自己的纹章"、
// "使自己纹章的倒计数 -1"）。纹章不在战场上，运行时按纹章区域解析。
func TestCrestSetIsAnActionableTargetSet(t *testing.T) {
	if _, ds := compile(t, validCard(`fanfare { destroy own.crests where card 12345678; }`)); len(ds) != 0 {
		t.Fatal("destroy own.crests in a fanfare was rejected:", ds)
	}
	if _, ds := compile(t, validCard(`fanfare { reduce countdown oppo.crests 1; }`)); len(ds) != 0 {
		t.Fatal("reduce countdown oppo.crests was rejected:", ds)
	}
}

// 一次打出的入场曲与爆能强化共享同一个打出帧：爆能强化可以读取入场曲的输出，
// 但反过来（爆能强化声明在入场曲之前）不行。
func TestEnhanceCanReadEarlierFanfareOutputs(t *testing.T) {
	pack, ds := compile(t, validCard(`
		fanfare { summon 1 card 12345678; }
		enhance 6 { add bane to summoned; }`))
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	if len(pack.Cards[0].Abilities) != 2 {
		t.Fatalf("abilities = %#v", pack.Cards[0].Abilities)
	}
	if _, ds := compile(t, validCard(`
		enhance 6 { add bane to summoned; }
		fanfare { summon 1 card 12345678; }`)); len(ds) == 0 {
		t.Fatal("enhance read a fanfare output declared after it")
	}
}
