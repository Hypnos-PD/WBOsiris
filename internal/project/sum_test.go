package project

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"wbo/internal/ir"
	"wbo/internal/syntax"
)

func TestCompileSumAndCurrentTurnHistory(t *testing.T) {
	source := validCard(`when self summoned {
    buff self +sum(own.destroyed.followers this turn where trait shikigami, base.attack)/+sum(own.destroyed.followers this turn where trait shikigami, base.life);
    damage oppo.leader count(oppo.destroyed this turn);
    choose held from own.hand;
    heal own.leader sum(held, base.cost);
}`)
	file, ds := syntax.Parse("12345678.wbo", []byte(source))
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	formatted := syntax.Format(file)
	file, ds = syntax.Parse("12345678.wbo", formatted)
	if len(ds) != 0 || !bytes.Equal(formatted, syntax.Format(file)) {
		t.Fatal("sum formatting is unstable")
	}
	root := t.TempDir()
	dir := filepath.Join(root, "12345")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "12345678.wbo")
	if err := os.WriteFile(path, formatted, 0600); err != nil {
		t.Fatal(err)
	}
	loaded := LoadWithRoot([]string{path}, true, root)
	pack, _, err := BuildRuntimePacks(loaded)
	if err != nil {
		t.Fatal(err, loaded.Diagnostics)
	}
	data, err := ir.EncodeCardPack(*pack)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := ir.DecodeCardPack(data)
	if err != nil {
		t.Fatal(err)
	}
	a := decoded.Cards[0].Abilities[0]
	trigger := a.Trigger.(ir.EventTrigger)
	if trigger.Event != "follower_summoned" || !trigger.SelfOnly || trigger.Side != "own" {
		t.Fatal(trigger)
	}
	buff := a.Body[0].(ir.TargetEffect)
	sum := buff.AttackExpr.(*ir.SumExpr)
	if sum.Field != "base_attack" || buff.LifeExpr.(*ir.SumExpr).Field != "base_life" || !reflect.DeepEqual(sum.Source.(ir.FilterRef).Source, ir.HistoryRef{Kind: "history", Side: "own", Member: "follower", Window: "this_turn"}) {
		t.Fatal(buff)
	}
	if !reflect.DeepEqual(a.Body[3].(ir.TargetEffect).AmountExpr.(*ir.SumExpr).Source, ir.BindingRef{Kind: "binding", Name: "held"}) {
		t.Fatal("sum lost its binding")
	}
}

func TestRejectMalformedSumAndHistoryWindows(t *testing.T) {
	for _, amount := range []string{
		"sum()", "sum(own.destroyed)", "sum(own.destroyed, attack)", "sum(own.destroyed, self.attack)",
		"sum(own.destroyed, base.missing)", "sum(own.destroyed, base.attack, base.life)",
		"sum(missing, base.attack)", "sum(own.hand this turn, base.attack)",
		"sum(own.destroyed this turn this turn, base.attack)", "sum(own.destroyed where trait shikigami this turn, base.attack)",
		"sum(own.destroyed this, base.attack)", "sum(own.destroyed this turn where, base.attack)",
		"count(own.destroyed this turn, base.attack)",
	} {
		file, ds := syntax.Parse("12345678.wbo", []byte(validCard("fanfare { damage oppo.leader "+amount+"; }")))
		if len(ds) == 0 && !hasErrors(ValidateFile(file)) {
			t.Fatal("accepted ambiguous aggregation", amount)
		}
	}
	for _, operation := range []string{"choose target from own.destroyed this turn;", "destroy own.destroyed this turn;", "when self summoned once per turn { draw 1; }"} {
		file, ds := syntax.Parse("12345678.wbo", []byte(validCard(operation)))
		if len(ds) == 0 && !hasErrors(ValidateFile(file)) {
			t.Fatal("accepted unsupported history target or trigger", operation)
		}
	}
}
