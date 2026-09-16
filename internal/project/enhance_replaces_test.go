package project

import (
	"testing"
)

// 爆能强化的"改为"档会替换基础效果；这里锁定解析结果里带上了 replaces 关系，
// 运行时再据此跳过基础效果与入场曲（见 internal/runner/runner.go 的 commitPlay）。
func TestEnhanceReplacesKeepsTheRelation(t *testing.T) {
	pack, ds := compile(t, validCard(`
		fanfare {
			random target from oppo.field.followers;
			damage target 4;
		}
		enhance 4 replaces {
			random targets from oppo.field.followers count 3;
			damage targets 4;
		}`))
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	abilities := pack.Cards[0].Abilities
	if len(abilities) != 2 {
		t.Fatalf("expected fanfare and enhance abilities, got %d", len(abilities))
	}
	enhance := abilities[1]
	if enhance.Relation != "replaces" || len(enhance.Body) != 2 {
		t.Fatalf("enhance relation/body not preserved: %+v", enhance)
	}
	if abilities[0].Relation != "independent" {
		t.Fatalf("fanfare must not become a replacement: %+v", abilities[0])
	}
}

func TestEnhanceReplacesRejectsBadShapes(t *testing.T) {
	for _, line := range []string{
		"enhance 4 extends { draw 1; }",
		"enhance replaces { draw 1; }",
		"enhance 4 replaces;",
	} {
		source := validCard("fanfare { draw 1; } " + line)
		if _, ds := compile(t, source); len(ds) == 0 {
			t.Fatalf("%q must not compile", line)
		}
	}
}
