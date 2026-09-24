package project

import (
	"testing"

	"wbo/internal/engine/ir"
)

// S-76：`replay fanfare self;` 重新发动本随从的【入场曲】。
func TestReplayFanfareCompiles(t *testing.T) {
	pack, ds := compile(t, validCard(`
		fanfare {
			mode random 2 {
				option 1 { draw 1; }
				option 2 { replay fanfare self; }
			}
		}
		superevolve {
			replay fanfare self;
		}`))
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	fanfare, ok := pack.Cards[0].Abilities[0].Body[0].(ir.ModeEffect)
	if !ok || !fanfare.Random || fanfare.Count != 2 {
		t.Fatalf("random mode lost: %#v", pack.Cards[0].Abilities[0].Body[0])
	}
	replay, ok := fanfare.Options[1].Body[0].(ir.ReplayFanfareEffect)
	if !ok || replay.Target == nil {
		t.Fatalf("replay did not compile: %#v", fanfare.Options[1].Body[0])
	}
	if _, ok := replay.Target.(ir.SelfRef); !ok {
		t.Fatalf("replay target should be self: %#v", replay.Target)
	}
	// 只支持 `replay fanfare <目标>`。
	for _, invalid := range []string{
		`fanfare { replay self; }`,
		`fanfare { replay fanfare; }`,
		`fanfare { replay evolve self; }`,
	} {
		if _, ds := compile(t, validCard(invalid)); len(ds) == 0 {
			t.Fatalf("accepted invalid replay: %s", invalid)
		}
	}
}
