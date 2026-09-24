package project

import (
	"testing"

	"wbo/internal/engine/ir"
)

// S-95：`add 1 counter step modulo 3` 加完取模，用于"按顺序循环发动"。
func TestCounterModuloCompiles(t *testing.T) {
	pack, ds := compile(t, validCard(`
		counter step 0;
		when self summoned {
			if self.counter.step == 0 {
				draw 1;
			}
			add 1 counter step modulo 3;
		}`))
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	body := pack.Cards[0].Abilities[0].Body
	step, ok := body[0].(ir.IfEffect)
	if !ok {
		t.Fatalf("if did not compile: %#v", body[0])
	}
	if condition, ok := step.Condition.(ir.CompareCondition); !ok || condition.Left.Field != "step" {
		t.Fatalf("counter condition lost: %#v", step.Condition)
	}
	adjust, ok := body[1].(ir.AdjustEffect)
	if !ok || adjust.Kind != "adjust_counter" || adjust.Field != "step" || adjust.Delta != 1 || adjust.Modulo != 3 {
		t.Fatalf("counter modulo lost: %#v", body[1])
	}
	plain, plainDiagnostics := compile(t, validCard(`counter step 0; fanfare { add 1 counter step; }`))
	if len(plainDiagnostics) != 0 {
		t.Fatal(plainDiagnostics)
	}
	if adjust, ok := plain.Cards[0].Abilities[0].Body[0].(ir.AdjustEffect); !ok || adjust.Modulo != 0 {
		t.Fatalf("plain counter adjustment changed: %#v", plain.Cards[0].Abilities[0].Body[0])
	}
	for _, invalid := range []string{
		`counter step 0; fanfare { add 1 counter step modulo 0; }`,
		`counter step 0; fanfare { add 1 counter step modulo 65536; }`,
	} {
		if _, ds := compile(t, validCard(invalid)); len(ds) == 0 {
			t.Fatalf("accepted invalid counter modulo: %s", invalid)
		}
	}
}
