package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"testing"

	"wbo/internal/runner"
)

func TestPublicCatalogMatchesConstructedDeckRules(t *testing.T) {
	root := serverRoot(t)
	s, err := New(root, []string{filepath.Join(root, "cards")})
	if err != nil {
		t.Fatal(err)
	}
	status, body := perform(t, s.Handler(), http.MethodGet, "/api/cards", "")
	if status != http.StatusOK {
		t.Fatalf("catalog status=%d", status)
	}
	var catalog struct {
		Cards        []catalogCard `json:"cards"`
		PracticeDeck []int         `json:"practiceDeck"`
	}
	if err := json.Unmarshal(body, &catalog); err != nil {
		t.Fatal(err)
	}
	if len(catalog.Cards) != len(s.cards.Cards) {
		t.Fatal("catalog omitted rule-pack cards")
	}
	for n, card := range catalog.Cards {
		if n > 0 && catalog.Cards[n-1].ID >= card.ID {
			t.Fatal("catalog is not stably sorted")
		}
		rule := cardByID(s.cards, card.ID)
		if !reflect.DeepEqual(card.Counters, rule.Counters) {
			t.Fatalf("catalog lost initial counters for %d", card.ID)
		}
		if card.Name != rule.Locales["chs"].Name || card.Text != rule.Locales["chs"].Text || card.Cost != rule.Cost || card.Class != rule.Meta.Class || card.DeckLegal != (runner.MatchCardUnavailableReason(rule) == "") {
			t.Fatalf("catalog diverged from card %d", card.ID)
		}
		if card.ID == 10121120 && !card.DeckLegal || card.ID == 90021120 && card.DeckLegal || card.ID == 10104120 && !card.DeckLegal {
			t.Fatalf("incorrect availability for %d", card.ID)
		}
	}
	if err := validateDeck(s.cards, catalog.PracticeDeck); err != nil {
		t.Fatalf("catalog supplied invalid practice deck: %v", err)
	}
	if status, _ := perform(t, s.Handler(), http.MethodPost, "/api/cards", ""); status != http.StatusMethodNotAllowed {
		t.Fatal("catalog accepted a write")
	}
}

func TestJoinInstallsGuestDeckWithoutChangingHostOpeningHand(t *testing.T) {
	root := serverRoot(t)
	s, err := New(root, []string{filepath.Join(root, "cards")})
	if err != nil {
		t.Fatal(err)
	}
	h := s.Handler()
	_, body := perform(t, h, http.MethodPost, "/api/matches", "")
	var owner response
	if err := json.Unmarshal(body, &owner); err != nil {
		t.Fatal(err)
	}
	room := s.matches[owner.MatchID]
	var identities []int
	for n := range s.cards.Cards {
		card := &s.cards.Cards[n]
		if card.Meta.Class == "swordcraft" && runner.MatchCardUnavailableReason(card) == "" {
			identities = append(identities, card.ID)
		}
	}
	if len(identities) < 14 {
		t.Fatal("not enough distinct cards for guest fixture")
	}
	guestDeck := make([]int, 40)
	for n := range guestDeck {
		guestDeck[n] = identities[n%14]
	}
	joinPath := "/api/matches/" + owner.MatchID + "/join?code=" + owner.JoinCode
	post := func(path string, value any) *httptest.ResponseRecorder {
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(data))
		res := httptest.NewRecorder()
		h.ServeHTTP(res, req)
		return res
	}
	previousSession := room.session
	previousView, err := previousSession.View("own")
	if err != nil {
		t.Fatal(err)
	}
	for _, invalid := range [][]int{{}, guestDeck[:39]} {
		res := post(joinPath, map[string]any{"deck": invalid})
		if res.Code != http.StatusBadRequest || room.joined || room.session != previousSession {
			t.Fatal("invalid guest deck changed room or consumed invitation")
		}
	}
	malformed := httptest.NewRecorder()
	h.ServeHTTP(malformed, httptest.NewRequest(http.MethodPost, joinPath, bytes.NewBufferString(`{"deck":`)))
	if malformed.Code != http.StatusBadRequest || room.joined || room.session != previousSession {
		t.Fatal("malformed guest request changed room or consumed invitation")
	}
	if res := post("/api/matches", map[string]any{"deck": []int{}}); res.Code != http.StatusBadRequest {
		t.Fatal("explicit empty deck silently used a default")
	}
	res := post(joinPath, map[string]any{"deck": guestDeck})
	if res.Code != http.StatusOK {
		t.Fatalf("join status=%d body=%s", res.Code, res.Body.String())
	}
	var guest response
	if err := json.Unmarshal(res.Body.Bytes(), &guest); err != nil {
		t.Fatal(err)
	}
	view, err := room.session.View("own")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(view.Own, previousView.Own) {
		t.Fatal("joining changed host state")
	}
	if guest.State.Own.HandCount != 4 || guest.State.Own.DeckCount != 36 {
		t.Fatalf("guest hand=%d deck=%d", guest.State.Own.HandCount, guest.State.Own.DeckCount)
	}
	for _, card := range guest.State.Own.Hand {
		if cardByID(s.cards, card.CardID).Meta.Class != "swordcraft" {
			t.Fatal("guest received default deck cards")
		}
	}
	if room.mulligan["own"] || room.started {
		t.Fatal("custom deck bypassed mulligan")
	}
	guestView, err := room.session.View("oppo")
	if err != nil {
		t.Fatal(err)
	}
	if len(room.replay) != 1 || !reflect.DeepEqual(room.replay[0].Oppo.Own.Hand, guestView.Own.Hand) {
		t.Fatal("replay retained the placeholder guest hand")
	}
	// Draw through both decks to verify every submitted identity and copy count.
	if step := room.session.StartMatch(); step.Status != runner.StatusCompleted {
		t.Fatal(step)
	}
	for n := 0; n < 72; n++ {
		current, _ := room.session.View("own")
		if current.Own.DeckCount == 0 && current.Oppo.DeckCount == 0 {
			break
		}
		step := room.session.SubmitAs(fmt.Sprintf("%032x", n+1), current.Turn.Active, runner.SimulatorCommand{Kind: "end_turn"})
		if step.Status != runner.StatusCompleted {
			t.Fatal(step)
		}
	}
	for side, deck := range map[string][]int{"own": room.ownDeck, "oppo": guestDeck} {
		current, _ := room.session.View(side)
		got, want := map[int]int{}, map[int]int{}
		for _, card := range append(current.Own.Hand, current.Own.Graveyard...) {
			got[card.CardID]++
		}
		for _, id := range deck {
			want[id]++
		}
		if current.Own.DeckCount != 0 || !reflect.DeepEqual(got, want) {
			t.Fatalf("%s full deck differs: got=%v want=%v", side, got, want)
		}
	}
}
