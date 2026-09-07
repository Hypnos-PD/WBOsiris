package project

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"wbo/internal/ir"
	"wbo/internal/syntax"
)

func TestDestructionBatchCompilesToOneEffect(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "12345", "12345678.wbo")
	if err := os.Mkdir(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	source := validCard("fanfare { choose ally from own.field.followers; choose enemy from oppo.field.followers; destroy ally, enemy; damage oppo.leader count(destroyed); }")
	if err := os.WriteFile(path, []byte(source), 0600); err != nil {
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
	body := decoded.Cards[0].Abilities[0].Body
	effect := body[2].(ir.TargetEffect)
	batch := effect.Target.(ir.DestructionBatchRef)
	if len(body) != 4 || effect.Kind != "destroy" || effect.Output != "destroyed" || len(batch.Targets) != 2 {
		t.Fatal(effect)
	}
	encoded, _ := json.Marshal(batch)
	if string(encoded) != `{"kind":"destruction_batch","targets":[{"kind":"binding","name":"ally"},{"kind":"binding","name":"enemy"}]}` {
		t.Fatal(string(encoded))
	}
}

func TestDestructionBatchRejectsAmbiguousTargets(t *testing.T) {
	for _, operation := range []string{
		"destroy ally, missing;", "destroy ally, ally;", "destroy ally,;", "destroy ,ally;",
		"destroy ally enemy;", "destroy ally, oppo.leader;", "destroy ally, enemy where cost >= 1;",
		"destroy ally, oppo.field.followers;", "banish ally, enemy;",
		"choose enemy from oppo.field.followers or oppo.leader; destroy ally, enemy;",
	} {
		f, ds := syntax.Parse("12345678.wbo", []byte(validCard("fanfare { choose ally from own.field.followers; choose enemy from oppo.field.followers; "+operation+" }")))
		if len(ds) == 0 && !hasErrors(ValidateFile(f)) {
			t.Fatal("accepted invalid batch", operation)
		}
	}
}
