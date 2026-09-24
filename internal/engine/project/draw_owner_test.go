package project

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"wbo/internal/engine/ir"
	"wbo/internal/engine/syntax"
)

// 让对手抽牌：`draw N for oppo`。抽牌必须走牌组顶部并产生抽牌事件，所以它需要的是
// draw 的参与者，而不是另一条语句。
func TestDrawForOpponentCompilesAndKeepsOwnAsDefault(t *testing.T) {
	body := compiledFanfare(t, `fanfare {
		draw 2;
		draw 1 for oppo;
		draw 2 for oppo from deck where type spell;
	}`)
	if len(body) != 3 {
		t.Fatalf("expected three effects, got %d", len(body))
	}
	first, ok := body[0].(ir.DrawEffect)
	if !ok || first.Owner != "own" || first.Count != 2 {
		t.Fatalf("default draw must stay the controller's: %+v", body[0])
	}
	second, ok := body[1].(ir.DrawEffect)
	if !ok || second.Owner != "oppo" || second.Count != 1 {
		t.Fatalf("draw for oppo must carry the opponent owner: %+v", body[1])
	}
	third, ok := body[2].(ir.DrawEffect)
	if !ok || third.Owner != "oppo" || third.Predicate == nil {
		t.Fatalf("draw for oppo with a filter must keep both: %+v", body[2])
	}
}

func TestDrawRejectsUnknownParticipantsAndStrayClauses(t *testing.T) {
	for _, line := range []string{
		"draw 1 for self;",
		"draw 1 for;",
		"draw 1 for oppo from deck;",
		"draw 1 for oppo for own;",
		"draw all for oppo;",
	} {
		source := validCard("fanfare { " + line + " }")
		if _, ds := compile(t, source); len(ds) == 0 {
			t.Fatalf("%q must not compile", line)
		}
	}
}

// `draw all` 仍然必须带过滤器；`for` 子句不能把这条约束绕过去。
func TestDrawFilterStillRequiredForAll(t *testing.T) {
	if _, ds := compile(t, validCard("fanfare { draw all for oppo from deck where type spell; }")); len(ds) != 0 {
		t.Fatalf("filtered draw all must compile: %v", ds)
	}
}

func TestDrawForOpponentFormatsStably(t *testing.T) {
	source := validCard("fanfare { draw 1 for oppo; }")
	file, ds := syntax.Parse("12345678.wbo", []byte(source))
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	formatted := syntax.Format(file)
	if !strings.Contains(string(formatted), "draw 1 for oppo;") {
		t.Fatalf("draw owner clause must survive formatting: %s", formatted)
	}
}

// `draw N from deck where … distinct names`：抽取 N 种不同卡名的卡牌，
// 编码/解码必须保留这个标记（运行期靠它排除同名候选）。
func TestDrawDistinctNamesCompilesAndSurvivesDecoding(t *testing.T) {
	body := compiledFanfare(t, `fanfare {
		draw 2 from deck where type spell and cost == 1 distinct names;
	}`)
	if len(body) != 1 {
		t.Fatalf("expected one effect, got %d", len(body))
	}
	draw, ok := body[0].(ir.DrawEffect)
	if !ok || !draw.DistinctNames || draw.Count != 2 || draw.Predicate == nil {
		t.Fatalf("distinct names must survive compilation: %+v", body[0])
	}
}

func TestDrawRejectsDistinctNamesWithoutFilterOrWithAll(t *testing.T) {
	for _, line := range []string{
		"draw 2 distinct names;",
		"draw 2 from deck where type spell and distinct names;",
		"draw all from deck where type spell distinct names;",
		"draw 2 from deck where type spell distinct;",
		"draw 2 from deck where type spell names;",
	} {
		source := validCard("fanfare { " + line + " }")
		if _, ds := compile(t, source); len(ds) == 0 {
			t.Fatalf("%q must not compile", line)
		}
	}
}

// 解码器必须把 distinctNames 真的写回效果结构体——牌组召唤的同一处曾漏回填，
// 运行期因此恒为 false。
func TestDrawDistinctNamesSurvivesEncoding(t *testing.T) {
	pack, ds := compile(t, validCard("fanfare { draw 2 from deck where type spell and cost == 1 distinct names; }"))
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	data, err := ir.EncodeCardPack(*pack)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := ir.DecodeCardPack(data)
	if err != nil {
		t.Fatal(err)
	}
	draw, ok := decoded.Cards[0].Abilities[0].Body[0].(ir.DrawEffect)
	if !ok || !draw.DistinctNames || draw.Count != 2 || draw.Predicate == nil {
		t.Fatalf("distinct names lost in codec: %+v", decoded.Cards[0].Abilities[0].Body[0])
	}
}

func compiledFanfare(t *testing.T, effectBlock string) []ir.Effect {
	t.Helper()
	pack, ds := compile(t, validCard(effectBlock))
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	return pack.Cards[0].Abilities[0].Body
}

func compile(t *testing.T, source string) (*ir.CardPack, []string) {
	t.Helper()
	file, parseDiagnostics := syntax.Parse("12345678.wbo", []byte(source))
	if len(parseDiagnostics) != 0 {
		return nil, []string{parseDiagnostics[0].Message}
	}
	root := t.TempDir()
	dir := filepath.Join(root, "12345")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "12345678.wbo")
	if err := os.WriteFile(path, []byte(syntax.Format(file)), 0600); err != nil {
		t.Fatal(err)
	}
	loaded := LoadWithRoot([]string{path}, true, root)
	if loaded.HasErrors() {
		messages := []string{}
		for _, diagnostic := range loaded.Diagnostics {
			messages = append(messages, diagnostic.Message)
		}
		return nil, messages
	}
	pack, _, err := BuildRuntimePacks(loaded)
	if err != nil {
		return nil, []string{err.Error()}
	}
	return pack, nil
}
