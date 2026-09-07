package project

import (
	"testing"

	"wbo/internal/syntax"
)

func TestSpellboostTransformSyntax(t *testing.T) {
	for _, body := range []string{
		`spellboost { transform self into card 12345678; }`,
		`counter x 0; spellboost { add 1 counter x; if self.counter.x >= 5 { transform self into card 12345678; } }`,
		`spellboost { repeat 1 { transform self into card 12345678; } }`,
		`spellboost { mode { option 1 { transform self into card 12345678; } option 2 {} } }`,
		`fusion material from own.hand { transform self into card 12345678 preserving materials; }`,
	} {
		file, ds := syntax.Parse("12345678.wbo", []byte(validCard(body)))
		if len(ds) != 0 || hasErrors(ValidateFile(file)) {
			t.Fatal(body, ds, ValidateFile(file))
		}
	}
	for _, body := range []string{
		`transform self into card 12345678;`,
		`fanfare { transform self into card 12345678; }`,
		`spellboost { transform target into card 12345678; }`,
		`spellboost { transform self into card 12345678 preserving; }`,
		`fusion material from own.hand { transform self into card 12345678; }`,
	} {
		file, ds := syntax.Parse("12345678.wbo", []byte(validCard(body)))
		if len(ds) == 0 && !hasErrors(ValidateFile(file)) {
			t.Fatal("accepted invalid transform", body)
		}
	}
}
