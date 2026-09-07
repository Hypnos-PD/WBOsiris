package runner

func (g *game) evolutionUnlocked(side string, super bool) bool {
	firstPlayer := g.firstPlayer
	if firstPlayer == "" {
		firstPlayer = "own"
	}
	turn := 5
	if super {
		turn = 7
	}
	if side != firstPlayer {
		turn--
	}
	return g.turn.Number >= turn
}
