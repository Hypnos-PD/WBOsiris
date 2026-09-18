package runner

import (
	"fmt"

	"wbo/internal/ir"
)

// PracticeDeck 是练习/压测用的固定 40 张卡组：第一弹的中立随从与法术轮转，
// 每张至多 3 张，天然满足构筑规则。客户端、练习模式与自对弈都从这里取，
// 保证"大家玩的是同一副默认卡组"。
func PracticeDeck() []int {
	ids := []int{10001110, 10001120, 10001130, 10001210, 10002110, 10002120, 10002210,
		10011110, 10011120, 10011130, 10011210, 10012110, 10012120, 10012310}
	deck := make([]int, 0, 40)
	for len(deck) < 40 {
		for _, id := range ids {
			if len(deck) == 40 {
				break
			}
			deck = append(deck, id)
		}
	}
	return deck
}

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

// NewMatchSession provides a deterministic match with own playing first.
func NewMatchSession(cards *ir.CardPack, ownDeck, oppoDeck []int, seed uint64) (*Session, error) {
	return NewMatchSessionWithFirstPlayer(cards, ownDeck, oppoDeck, seed, "own")
}

// NewMatchSessionWithFirstPlayer deals both opening hands with an explicit first player.
// The same RNG continues through mulligans and subsequent card effects.
func NewMatchSessionWithFirstPlayer(cards *ir.CardPack, ownDeck, oppoDeck []int, seed uint64, firstPlayer string) (*Session, error) {
	if firstPlayer != "own" && firstPlayer != "oppo" {
		return nil, fmt.Errorf("first player must be own or oppo")
	}
	for _, deck := range [][]int{ownDeck, oppoDeck} {
		if err := ValidateMatchDeck(cards, deck); err != nil {
			return nil, err
		}
	}
	index := map[int]*ir.Card{}
	for n := range cards.Cards {
		index[cards.Cards[n].ID] = &cards.Cards[n]
	}
	state := ir.State{Turn: ir.Turn{Active: firstPlayer, Number: 1}, Phase: "main", FirstPlayer: firstPlayer, Players: map[string]ir.PlayerState{}}
	for seat, deck := range [][]int{ownDeck, oppoDeck} {
		side := []string{"own", "oppo"}[seat]
		player := ir.PlayerState{Leader: ir.Leader{Life: 20, MaxLife: 20}, EP: 2, SEP: 2, Zones: map[string][]ir.TestInstance{}}
		if side == firstPlayer {
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
