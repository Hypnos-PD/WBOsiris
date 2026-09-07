package project

import (
	"bytes"
	"reflect"
	"testing"

	"wbo/internal/ir"
	"wbo/internal/syntax"
)

func TestFilterOrPrecedenceAndFormatting(t *testing.T) {
	file, ds := syntax.Parse("12345678.wbo", []byte(validCard(`
		fanfare { buff own.field.followers +1/+1 where type follower and life <= 2 or trait artifact and class portalcraft; }
	`)))
	if len(ds) != 0 || hasErrors(ValidateFile(file)) {
		t.Fatal("valid or filter rejected", ds, ValidateFile(file))
	}
	formatted := syntax.Format(file)
	again, ds := syntax.Parse("12345678.wbo", formatted)
	if len(ds) != 0 || !bytes.Equal(formatted, syntax.Format(again)) {
		t.Fatal("or formatting is not stable")
	}
	tokens, ds := syntax.Lex("filter", []byte("where type follower and life <= 2 or trait artifact and class portalcraft"))
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	got, _ := filterIR(tokens, 0)
	want := ir.OrPredicate{Kind: "or", Terms: []ir.Predicate{
		ir.AndPredicate{Kind: "and", Terms: []ir.Predicate{
			ir.FieldPredicate{Kind: "has_type", CardType: "follower"},
			ir.FieldPredicate{Kind: "compare", Field: "life", Op: "le", Value: 2},
		}},
		ir.AndPredicate{Kind: "and", Terms: []ir.Predicate{
			ir.FieldPredicate{Kind: "has_trait", Trait: "artifact"},
			ir.FieldPredicate{Kind: "has_class", Class: "portalcraft"},
		}},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("and must bind more tightly than or: %#v", got)
	}
	for _, filter := range []string{"or card 90073120", "card 90073120 or", "card 90073120 and", "card 90073120 or and card 90073130", "card 90073120 or card invalid"} {
		file, ds := syntax.Parse("12345678.wbo", []byte(validCard(`fusion material from own.hand where `+filter+` { draw 1; }`)))
		if len(ds) == 0 && !hasErrors(ValidateFile(file)) {
			t.Fatalf("accepted malformed filter %q", filter)
		}
	}
}
