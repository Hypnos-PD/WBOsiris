package project

import (
	"testing"

	"wbo/internal/engine/ir"
)

// S-73：`mode random N { … }` 由引擎随机选出 N 个能力，不向玩家提问。
func TestRandomModeCompiles(t *testing.T) {
	pack, ds := compile(t, validCard(`
		fanfare {
			mode random 2 {
				option 1 { draw 1; }
				option 2 { heal own.leader 1; }
				option 3 { gain own.maxpp 1; }
			}
		}`))
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	effect, ok := pack.Cards[0].Abilities[0].Body[0].(ir.ModeEffect)
	if !ok || !effect.Random || effect.Count != 2 || len(effect.Options) != 3 {
		t.Fatalf("random mode did not compile: %#v", pack.Cards[0].Abilities[0].Body[0])
	}
	if _, ds := compile(t, validCard(`
		fanfare {
			mode random 0 {
				option 1 { draw 1; }
				option 2 { heal own.leader 1; }
			}
		}`)); len(ds) == 0 {
		t.Fatal("random mode accepted a zero count")
	}
	if _, ds := compile(t, validCard(`
		fanfare {
			mode random 1 {
				option 1 { draw 1; }
			}
		}`)); len(ds) == 0 {
		t.Fatal("random mode accepted a single option")
	}
}
