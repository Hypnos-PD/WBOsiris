package project

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"wbo/internal/syntax"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func TestProjectCorpusAndReferenceStrictness(t *testing.T) {
	root := repoRoot(t)
	paths := []string{filepath.Join(root, "cards"), filepath.Join(root, "tests")}
	l := LoadWithRoot(paths, false, root)
	if l.HasErrors() {
		t.Fatalf("default check errors: %#v", l.Diagnostics)
	}
	if len(l.Cards) != 232 {
		t.Fatalf("cards=%d", len(l.Cards))
	}
	sc := 0
	for _, tf := range l.Tests {
		sc += len(tf.Scenarios)
	}
	if sc != 77 {
		t.Fatalf("scenarios=%d", sc)
	}
	if len(l.Unresolved) != 0 {
		t.Fatalf("unresolved=%v", l.Unresolved)
	}
	strict := LoadWithRoot(paths, true, root)
	if strict.HasErrors() {
		t.Fatalf("strict references failed: %#v", strict.Diagnostics)
	}
}

func TestCompileDeterministicAndStructured(t *testing.T) {
	root := repoRoot(t)
	l := LoadWithRoot([]string{filepath.Join(root, "cards")}, false, root)
	if l.HasErrors() {
		t.Fatal(l.Diagnostics)
	}
	typed, err := compileCardsTyped(l)
	if err != nil {
		t.Fatal(err)
	}
	if len(typed.Cards) != 232 || len(typed.Sources) != 232 {
		t.Fatalf("typed card pack has cards=%d sources=%d", len(typed.Cards), len(typed.Sources))
	}
	a, err := Compile(l, true)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Compile(l, true)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a, b) {
		t.Fatal("compile output is not deterministic")
	}
	var pack map[string]any
	if err := json.Unmarshal(a, &pack); err != nil {
		t.Fatal(err)
	}
	if pack["format"] != "wbos" || pack["kind"] != "card-pack" || pack["contentHash"] == "" {
		t.Fatalf("bad pack header: %v", pack)
	}
	cards := pack["cards"].([]any)
	first := cards[0].(map[string]any)
	if _, ok := first["abilities"].([]any); !ok {
		t.Fatalf("abilities are not structured: %v", first)
	}
	assertNoRawTokens(t, pack)
	if len(pack["unresolvedReferences"].([]any)) != 0 {
		t.Fatal("unexpected unresolved references")
	}
}

func TestCompileTestPackStructured(t *testing.T) {
	root := repoRoot(t)
	l := LoadWithRoot([]string{filepath.Join(root, "tests")}, false, root)
	if l.HasErrors() {
		t.Fatal(l.Diagnostics)
	}
	if len(l.Unresolved) != 0 {
		t.Fatalf("test dependencies unresolved=%v", l.Unresolved)
	}
	typed, err := compileTestsTyped(l)
	if err != nil {
		t.Fatal(err)
	}
	if len(typed.Scenarios) != 77 || typed.Scenarios[0].Name != "无敌方目标时零费启动仍先破坏自身" {
		t.Fatalf("bad typed test pack: scenarios=%d", len(typed.Scenarios))
	}
	b, err := Compile(l, true)
	if err != nil {
		t.Fatal(err)
	}
	var pack map[string]any
	if err := json.Unmarshal(b, &pack); err != nil {
		t.Fatal(err)
	}
	if pack["format"] != "wbos" || pack["kind"] != "test-pack" {
		t.Fatalf("bad test container: %v %v", pack["format"], pack["kind"])
	}
	if _, ok := pack["ruleset"].(map[string]any); !ok {
		t.Fatal("test pack is missing ruleset dependency")
	}
	scenarios := pack["scenarios"].([]any)
	if len(scenarios) != 77 {
		t.Fatalf("scenarios=%d", len(scenarios))
	}
	first := scenarios[0].(map[string]any)
	if first["name"] != "无敌方目标时零费启动仍先破坏自身" {
		t.Fatalf("scenarios are not in source order: %v", first["name"])
	}
	if _, ok := first["initialState"].(map[string]any); !ok {
		t.Fatal("initialState is not materialized")
	}
	for _, field := range []string{"actions", "assertions"} {
		if _, ok := first[field].([]any); !ok {
			t.Fatalf("%s is not typed", field)
		}
	}
	assertNoRawTokens(t, pack)
}

func assertNoRawTokens(t *testing.T, v any) {
	t.Helper()
	switch x := v.(type) {
	case map[string]any:
		for k, child := range x {
			if k == "arguments" || k == "fields" {
				t.Fatalf("raw token field %q found", k)
			}
			assertNoRawTokens(t, child)
		}
	case []any:
		for _, child := range x {
			assertNoRawTokens(t, child)
		}
	}
}

func TestSourceIDStableWithFixedRoot(t *testing.T) {
	root := repoRoot(t)
	file := filepath.Join(root, "cards", "10000", "10001110.wbo")
	all := LoadWithRoot([]string{filepath.Join(root, "cards")}, false, root)
	one := LoadWithRoot([]string{file}, false, root)
	a, err := Compile(all, true)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Compile(one, true)
	if err != nil {
		t.Fatal(err)
	}
	var pa, pb map[string]any
	if json.Unmarshal(a, &pa) != nil || json.Unmarshal(b, &pb) != nil {
		t.Fatal("invalid JSON")
	}
	want := ""
	for _, src := range pa["sources"].([]any) {
		m := src.(map[string]any)
		if m["path"] == "cards/10000/10001110.wbo" {
			want = m["sourceId"].(string)
		}
	}
	got := pb["sources"].([]any)[0].(map[string]any)["sourceId"].(string)
	if want == "" || want != got {
		t.Fatalf("source ids differ: %q %q", want, got)
	}
}

func TestStrictEffectReviewCounterexamples(t *testing.T) {
	cases := []string{
		`when own banana summoned { draw 1; }`,
		`fusion material from own.hand where type amulet garbage { draw 1; }`,
		`if combo banana 3 { draw 1; }`,
		`mode { option 0 { draw 1; } option 0 { draw 1; } }`,
		`superevolve replaces evolve { draw 1; }`,
		`transform self into card 12345678 preserving materials;`,
		`fanfare { buff self; }`,
		`fanfare { buff own.hand other; }`,
		`fanfare { buff self +1; }`,
		`fanfare { buff self +1/+1 where; }`,
		`trait shikigamii;`,
		`fanfare { buff own.hand +1/+0 where trait puppetryy; }`,
		`when own follower summoned where trait bat { draw 1; }`,
	}
	for _, effect := range cases {
		f, pds := syntax.Parse("12345678.wbo", []byte(validCard(effect)))
		if len(pds) != 0 {
			t.Fatalf("parse %q: %v", effect, pds)
		}
		if got := ValidateFile(f); !hasErrors(got) {
			t.Errorf("accepted malformed effect: %s", effect)
		}
	}
}

func TestDiscardEventTriggerIsAccepted(t *testing.T) {
	f, pds := syntax.Parse("12345678.wbo", []byte(validCard(`when own card discarded { draw 1; }`)))
	if len(pds) != 0 {
		t.Fatalf("parse discard event: %v", pds)
	}
	if ds := ValidateFile(f); hasErrors(ds) {
		t.Fatalf("discard event rejected: %v", ds)
	}
}

func TestStrictTestReviewCounterexamples(t *testing.T) {
	cases := []string{
		`state { player own { nonsense 1; } }`,
		`state { player own { hand { follower a = 12345678 garbage; } } }`,
		`action { play missing extra; }`,
		`expect { legal garbage; }`,
		`expect { legal; events contains ordered { damage own.leader 1 garbage; } }`,
	}
	for _, part := range cases {
		src := `wbotest 0.1.0; use cards "../../cards"; scenario "x" { seed 1; state {} action { end_turn; } expect { legal; } }`
		switch {
		case strings.HasPrefix(part, "state"):
			src = strings.Replace(src, "state {}", part, 1)
		case strings.HasPrefix(part, "action"):
			src = strings.Replace(src, "action { end_turn; }", part, 1)
		default:
			src = strings.Replace(src, "expect { legal; }", part, 1)
		}
		f, pds := syntax.Parse("x.wbotest", []byte(src))
		if len(pds) != 0 {
			continue
		}
		if got := ValidateFile(f); !hasErrors(got) {
			t.Errorf("accepted malformed test construct: %s", part)
		}
	}
}

func validCard(effect string) string {
	return `wbo 0.1.0; card 12345678 { type follower; cost 1; stats 1/1; effect { ` + effect + ` } meta { pack 12345; class neutral; rarity bronze; } locale chs { name "x"; text ""; } locale eng { name "x"; text ""; } locale jpn { name "x"; text ""; } locale kor { name "x"; text ""; } locale cht { name "x"; text ""; } }`
}

func TestInvalidCardDiagnostic(t *testing.T) {
	root := repoRoot(t)
	path := filepath.Join(root, "internal", "project", "testdata", "invalid.wbo")
	src := []byte("wbo 0.1.0; card 12345678 { type follower; cost 1; effect {} meta { pack 1; class neutral; rarity bronze; } }")
	f, ds := syntax.Parse(path, src)
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	v := ValidateFile(f)
	if len(v) == 0 || v[0].Code == "" {
		t.Fatalf("expected semantic diagnostic: %#v", v)
	}
}
