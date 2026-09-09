package project

import (
	"testing"

	"wbo/internal/syntax"
)

func TestRejectOutOfRangeAttackLimit(t *testing.T) {
	for _, operation := range []string{
		"set_attack_limit self 0;",
		"set_attack_limit self 65536;",
		"set_attack_limit self -1;",
	} {
		f, ds := syntax.Parse("12345678.wbo", []byte(validCard("fanfare { "+operation+" }")))
		if len(ds) == 0 && !hasErrors(ValidateFile(f)) {
			t.Fatalf("accepted invalid attack limit: %s", operation)
		}
	}
}
