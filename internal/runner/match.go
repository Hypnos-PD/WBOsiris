package runner

import (
	"fmt"

	"wbo/internal/ir"
)

// ValidateMatchDeck validates constructed play against the loaded rule pack.
func ValidateMatchDeck(cards *ir.CardPack, deck []int) error {
	if cards == nil || len(deck) != 40 {
		return fmt.Errorf("deck must contain exactly 40 cards")
	}
	index := make(map[int]*ir.Card, len(cards.Cards))
	for n := range cards.Cards {
		index[cards.Cards[n].ID] = &cards.Cards[n]
	}
	counts := map[int]int{}
	class := ""
	for _, id := range deck {
		card := index[id]
		if card == nil {
			return fmt.Errorf("unknown card %d", id)
		}
		if reason := MatchCardUnavailableReason(card); reason != "" {
			return fmt.Errorf("card %d: %s", id, reason)
		}
		switch card.Meta.Class {
		case "neutral":
		case "forestcraft", "swordcraft", "runecraft", "dragoncraft", "abysscraft", "havencraft", "portalcraft":
			if class != "" && class != card.Meta.Class {
				return fmt.Errorf("deck must use one class and neutral cards")
			}
			class = card.Meta.Class
		default:
			return fmt.Errorf("card %d has no supported class", id)
		}
		counts[id]++
		if counts[id] > 3 {
			return fmt.Errorf("card %d exceeds three copies", id)
		}
	}
	return nil
}

// MatchCardUnavailableReason is shared by the public catalog and deck validation.
func MatchCardUnavailableReason(card *ir.Card) string {
	if card.Meta.Pack == 90000 || (card.CardType != "follower" && card.CardType != "spell" && card.CardType != "amulet") {
		return "cannot be included in a deck"
	}
	for _, restriction := range card.Restrictions {
		if restriction.Kind == "unplayable" {
			return "unavailable for constructed play"
		}
	}
	switch card.Meta.Class {
	case "neutral", "forestcraft", "swordcraft", "runecraft", "dragoncraft", "abysscraft", "havencraft", "portalcraft":
		return ""
	default:
		return "no supported class"
	}
}

// NewMatchSession fixes own as the first seat and deals both opening hands.
// The same RNG continues through mulligans and subsequent card effects.
func NewMatchSession(cards *ir.CardPack, ownDeck, oppoDeck []int, seed uint64) (*Session, error) {
	for _, deck := range [][]int{ownDeck, oppoDeck} {
		if err := ValidateMatchDeck(cards, deck); err != nil {
			return nil, err
		}
	}
	index := map[int]*ir.Card{}
	for n := range cards.Cards {
		index[cards.Cards[n].ID] = &cards.Cards[n]
	}
	state := ir.State{Turn: ir.Turn{Active: "own", Number: 1}, Phase: "main", FirstPlayer: "own", Players: map[string]ir.PlayerState{}}
	for seat, deck := range [][]int{ownDeck, oppoDeck} {
		side := []string{"own", "oppo"}[seat]
		player := ir.PlayerState{Leader: ir.Leader{Life: 20, MaxLife: 20}, EP: 2, SEP: 2, Zones: map[string][]ir.TestInstance{}}
		if seat == 0 {
			player.PP, player.MaxPP = 1, 1
		} else {
			player.ExtraPPEarly, player.ExtraPPLate = true, true
		}
		for n, id := range deck {
			player.Zones["deck"] = append(player.Zones["deck"], ir.TestInstance{
				InstanceID: fmt.Sprintf("%032x", seat*40+n+1), CardID: id, DeclaredType: index[id].CardType,
			})
		}
		state.Players[side] = player
	}
	session, err := NewSession(cards, state, seed)
	if err != nil {
		return nil, err
	}
	for _, player := range []*player{&session.g.own, &session.g.oppo} {
		for n := len(player.deck) - 1; n > 0; n-- {
			other := session.g.rng.Index(n + 1)
			player.deck[n], player.deck[other] = player.deck[other], player.deck[n]
		}
		for range 4 {
			session.g.move(player.deck[0], "hand")
		}
	}
	return session, nil
}
