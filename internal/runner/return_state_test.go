package runner

import (
	"strings"
	"testing"
	"wbo/internal/ir"
)

func TestReturningRemovedCardsRestoresPrintedState(t *testing.T) {
	pack := &ir.CardPack{Cards: []ir.Card{{ID: 12345678, CardType: "follower", Cost: 2, Stats: &ir.Stats{Attack: 2, Life: 2}, Intrinsic: []string{"ward"}}}}
	for _, owner := range []string{"own", "oppo"} {
		for _, from := range []string{"graveyard", "banished"} {
			for _, to := range []string{"hand", "deck"} {
				state := testState()
				id := strings.Repeat("a", 32)
				state.Players[owner] = withInstance(state.Players[owner], from, ir.TestInstance{InstanceID: id, CardID: 12345678, DeclaredType: "follower"})
				s, err := NewSession(pack, state, 1)
				if err != nil {
					t.Fatal(err)
				}
				i := s.g.instances[id]
				i.attack, i.life, i.cost = 9, -2, 0
				i.evolved = true
				i.removeKeyword("ward")
				i.addKeyword("storm", owner)
				s.g.returnCard(i, to)
				if i.zone != to || s.g.owner(i) != s.g.player(owner) || i.attack != 2 || i.life != 2 || i.cost != 2 || i.evolved || !i.abilities["ward"] || i.abilities["storm"] || len(i.temporaryKeywords) != 0 {
					t.Fatalf("%s %s -> %s did not reset card: %#v", owner, from, to, i)
				}
			}
		}
	}
}
