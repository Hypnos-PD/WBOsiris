package server

import (
	"crypto/rand"
	"encoding/binary"
	"io"

	"wbo/internal/runner"
)

func randomMatchSetup() (uint64, string, error) {
	return readMatchSetup(rand.Reader)
}

func readMatchSetup(entropy io.Reader) (uint64, string, error) {
	var data [9]byte
	if _, err := io.ReadFull(entropy, data[:]); err != nil {
		return 0, "", err
	}
	first := "own"
	if data[8]&1 != 0 {
		first = "oppo"
	}
	return binary.LittleEndian.Uint64(data[:8]), first, nil
}

// Waiting rooms have no dealt cards, first player or simulator RNG yet.
func matchState(room *match, side string) (runner.StateView, error) {
	if room.session != nil {
		state, err := room.session.View(side)
		if !room.started {
			state.Phase = "mulligan"
		}
		return state, err
	}
	player := func() runner.PlayerView {
		return runner.PlayerView{LeaderLife: 20, LeaderMax: 20, EP: 2, SEP: 2,
			Field: []runner.EntityView{}, Crests: []runner.EntityView{}, Graveyard: []runner.EntityView{},
			Banished: []runner.EntityView{}, Destroyed: []runner.EntityView{}}
	}
	state := runner.StateView{Viewer: side, Phase: "waiting", Turn: runner.TurnView{Active: "own"}, Own: player(), Oppo: player()}
	state.Own.DeckCount = len(room.ownDeck)
	return state, nil
}
