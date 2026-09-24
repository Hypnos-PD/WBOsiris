package project

import "testing"

// S-55：护符可以声明【灵气】（月影指环这类卡），其它固有关键词仍然只允许随从。
func TestAmuletAllowsAuraOnly(t *testing.T) {
	source := func(effect string) string {
		return `wbo 0.1.0; card 12345678 { type amulet; cost 1; effect { ` + effect + ` } meta { pack 12345; class havencraft; rarity legendary; } locale chs { name "x"; text ""; } locale eng { name "x"; text ""; } locale jpn { name "x"; text ""; } locale kor { name "x"; text ""; } locale cht { name "x"; text ""; } }`
	}
	pack, ds := compile(t, source(`countdown 1; aura;`))
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	if len(pack.Cards[0].Intrinsic) != 1 || pack.Cards[0].Intrinsic[0] != "aura" {
		t.Fatalf("aura did not compile into an intrinsic keyword: %#v", pack.Cards[0].Intrinsic)
	}
	if _, ds := compile(t, source(`ward;`)); len(ds) == 0 {
		t.Fatal("ward was accepted on an amulet")
	}
}
