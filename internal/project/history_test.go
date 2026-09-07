package project

import (
	"testing"
	"wbo/internal/syntax"
)

func TestDestroyedHistoryIsReadOnly(t *testing.T) {
	for _, operation := range []string{
		"choose old from own.destroyed;", "require old from oppo.destroyed.followers;",
		"random old from own.destroyed where cost <= 4;", "return own.destroyed to hand;",
		"buff own.destroyed +1/+1;", "damage own.destroyed 2;", "destroy own.destroyed;",
		"banish own.destroyed;", "reduce cost own.destroyed 1 minimum 0;",
	} {
		file, ds := syntax.Parse("12345678.wbo", []byte(validCard("fanfare { "+operation+" }")))
		if len(ds) == 0 && !hasErrors(ValidateFile(file)) {
			t.Fatal("accepted history as an effect target", operation)
		}
	}
	for _, operation := range []string{
		"damage oppo.leader count(own.destroyed);",
		"damage oppo.leader count(own.destroyed where type follower);",
		"choose target from own.graveyard; return target to hand;",
		"destroy own.field; damage oppo.leader count(destroyed);",
	} {
		file, ds := syntax.Parse("12345678.wbo", []byte(validCard("fanfare { "+operation+" }")))
		if len(ds) != 0 || hasErrors(ValidateFile(file)) {
			t.Fatal("rejected historical count or live binding", operation, ds, ValidateFile(file))
		}
	}
}
