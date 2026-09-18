package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"wbo/internal/runner"
)

// 指定模式不接受已经轮换出去的卡包；同一副牌组在无限制模式里可以建局。
func TestMatchCreateRespectsFormat(t *testing.T) {
	root := serverRoot(t)
	s, err := New(root, []string{filepath.Join(root, "cards"), filepath.Join(root, "tests")})
	if err != nil {
		t.Fatal(err)
	}
	rotation, err := runner.FormatByID(s.cards, runner.FormatRotation)
	if err != nil {
		t.Fatal(err)
	}
	rotated := 0
	for n := range s.cards.Cards {
		card := &s.cards.Cards[n]
		if card.Meta.Pack > 0 && !rotation.AllowsPack(card.Meta.Pack) {
			rotated = card.Meta.Pack
			break
		}
	}
	if rotated == 0 {
		t.Skip("card pool has no rotated-out pack")
	}
	var pool []int
	for n := range s.cards.Cards {
		card := &s.cards.Cards[n]
		if card.Meta.Pack != rotated || runner.MatchCardUnavailableReason(card) != "" {
			continue
		}
		if card.Meta.Class == "neutral" || card.Meta.Class == "swordcraft" {
			pool = append(pool, card.ID)
		}
	}
	if len(pool) == 0 {
		t.Skip("rotated pack has no class cards")
	}
	deck := make([]int, 0, 40)
	for len(deck) < 40 {
		deck = append(deck, pool[len(deck)%len(pool)])
	}
	post := func(format string) *httptest.ResponseRecorder {
		data, _ := json.Marshal(createRequest{Deck: deck, Format: format})
		req := httptest.NewRequest(http.MethodPost, "/api/matches", bytes.NewReader(data))
		res := httptest.NewRecorder()
		s.Handler().ServeHTTP(res, req)
		return res
	}
	if res := post(runner.FormatRotation); res.Code != http.StatusBadRequest {
		t.Fatalf("rotation accepted a rotated-out deck: status=%d body=%s", res.Code, res.Body.String())
	}
	if res := post(runner.FormatUnlimited); res.Code != http.StatusOK {
		t.Fatalf("unlimited rejected a published deck: status=%d body=%s", res.Code, res.Body.String())
	}
}

// 卡表要带上赛制信息，客户端才能按赛制过滤卡池。
func TestCatalogExposesFormats(t *testing.T) {
	root := serverRoot(t)
	s, err := New(root, []string{filepath.Join(root, "cards"), filepath.Join(root, "tests")})
	if err != nil {
		t.Fatal(err)
	}
	status, body := perform(t, s.Handler(), http.MethodGet, "/api/cards", "")
	if status != http.StatusOK {
		t.Fatalf("catalog status=%d", status)
	}
	var payload struct {
		Cards []struct {
			ID      int      `json:"id"`
			Pack    int      `json:"pack"`
			Formats []string `json:"formats"`
		} `json:"cards"`
		Formats []struct {
			ID    string `json:"id"`
			Name  string `json:"name"`
			Packs []int  `json:"packs"`
		} `json:"formats"`
	}
	if err := json.NewDecoder(bytes.NewReader(body)).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Formats) != 2 || payload.Formats[0].ID != runner.FormatRotation || payload.Formats[1].ID != runner.FormatUnlimited {
		t.Fatalf("catalog formats = %#v", payload.Formats)
	}
	if payload.Formats[0].Name != "指定模式" || len(payload.Formats[0].Packs) != 7 {
		t.Fatalf("rotation descriptor = %#v", payload.Formats[0])
	}
	rotation := map[int]bool{}
	for _, pack := range payload.Formats[0].Packs {
		rotation[pack] = true
	}
	checked := 0
	for _, card := range payload.Cards {
		if len(card.Formats) == 0 {
			continue
		}
		checked++
		wantRotation := rotation[card.Pack]
		hasRotation := false
		for _, id := range card.Formats {
			if id == runner.FormatRotation {
				hasRotation = true
			}
		}
		if wantRotation != hasRotation {
			t.Fatalf("card %d pack %d formats = %v", card.ID, card.Pack, card.Formats)
		}
	}
	if checked == 0 {
		t.Fatal("catalog exposed no deck-legal cards")
	}
}
