package ai

import (
	"fmt"
	"sort"

	"wbo/internal/ir"
	"wbo/internal/ruleset"
	"wbo/internal/runner"
)

// RandomDeck 在指定赛制里随机生成一副合法的 40 张卡组：单一职业 + 中立，
// 同名至多 3 张。它的用途是让自对弈能覆盖到整个卡池——固定练习卡组只能碰到
// 十来张卡，随机卡组才可能撞出冷门卡的规则 bug。
func RandomDeck(cards *ir.CardPack, format runner.Format, rng *ruleset.RNG) ([]int, error) {
	if cards == nil {
		return nil, fmt.Errorf("card pack is required")
	}
	if rng == nil {
		rng = ruleset.NewRNG(1)
	}
	type candidate struct {
		id    int
		class string
	}
	pool := make([]candidate, 0, len(cards.Cards))
	classes := map[string]bool{}
	for n := range cards.Cards {
		card := &cards.Cards[n]
		if runner.MatchCardUnavailableReason(card) != "" {
			continue
		}
		if !format.AllowsPack(card.Meta.Pack) {
			continue
		}
		if card.Meta.Class == "" || card.Meta.Class == "neutral" {
			pool = append(pool, candidate{id: card.ID, class: "neutral"})
			continue
		}
		pool = append(pool, candidate{id: card.ID, class: card.Meta.Class})
		classes[card.Meta.Class] = true
	}
	if len(pool) == 0 {
		return nil, fmt.Errorf("format %s has no deck-legal cards", format.ID)
	}
	chosen := "neutral"
	if len(classes) > 0 {
		names := make([]string, 0, len(classes))
		for name := range classes {
			names = append(names, name)
		}
		sort.Strings(names)
		chosen = names[rng.Index(len(names))]
	}
	deckPool := make([]int, 0, len(pool))
	for _, item := range pool {
		if item.class == "neutral" || item.class == chosen {
			deckPool = append(deckPool, item.id)
		}
	}
	if len(deckPool)*3 < 40 {
		return nil, fmt.Errorf("class %s only has %d deck-legal cards in %s", chosen, len(deckPool), format.ID)
	}
	// Fisher-Yates 洗牌，保证同名卡在卡组里分散。
	for n := len(deckPool) - 1; n > 0; n-- {
		other := rng.Index(n + 1)
		deckPool[n], deckPool[other] = deckPool[other], deckPool[n]
	}
	counts := map[int]int{}
	deck := make([]int, 0, 40)
	for len(deck) < 40 {
		progress := false
		for _, id := range deckPool {
			if len(deck) == 40 {
				break
			}
			if counts[id] >= 3 {
				continue
			}
			counts[id]++
			deck = append(deck, id)
			progress = true
		}
		if !progress {
			return nil, fmt.Errorf("class %s cannot fill a 40 card deck in %s", chosen, format.ID)
		}
	}
	if err := runner.ValidateDeckForFormat(cards, deck, format); err != nil {
		return nil, err
	}
	return deck, nil
}
