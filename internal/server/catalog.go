package server

import (
	"maps"
	"net/http"
	"sort"

	"wbo/internal/runner"
)

type catalogCard struct {
	Counters          map[string]int `json:"counters,omitempty"`
	ID                int            `json:"id"`
	Name              string         `json:"name"`
	Text              string         `json:"text"`
	CardType          string         `json:"cardType"`
	Class             string         `json:"class"`
	Rarity            string         `json:"rarity"`
	Pack              int            `json:"pack"`
	Cost              int            `json:"cost"`
	Attack            *int           `json:"attack,omitempty"`
	Life              *int           `json:"life,omitempty"`
	Traits            []string       `json:"traits"`
	DeckLegal         bool           `json:"deckLegal"`
	UnavailableReason string         `json:"unavailableReason,omitempty"`
}

func (s *Server) cardCatalog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	items := make([]catalogCard, 0, len(s.cards.Cards))
	for n := range s.cards.Cards {
		card := &s.cards.Cards[n]
		locale := card.Locales["chs"]
		reason := runner.MatchCardUnavailableReason(card)
		item := catalogCard{ID: card.ID, Name: locale.Name, Text: locale.Text,
			CardType: card.CardType, Class: card.Meta.Class, Rarity: card.Meta.Rarity,
			Pack: card.Meta.Pack, Cost: card.Cost, Traits: append([]string{}, card.Traits...),
			DeckLegal: reason == "", UnavailableReason: reason, Counters: maps.Clone(card.Counters)}
		if card.Stats != nil {
			item.Attack, item.Life = &card.Stats.Attack, &card.Stats.Life
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, struct {
		Cards        []catalogCard `json:"cards"`
		PracticeDeck []int         `json:"practiceDeck"`
	}{items, practiceDeck()})
}
