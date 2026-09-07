package runner

import "wbo/internal/ir"

func (g *game) effectEndingSide(until, ownSide string) string {
	switch until {
	case "turn_end":
		if g.endingSide != "" {
			return g.endingSide
		}
		return g.turn.Active
	case "own_turn_end":
		return ownSide
	case "oppo_turn_end":
		return oppositeSide(ownSide)
	default:
		return ""
	}
}

func (i *instance) buffStats(attack, life int, endingSide string) {
	i.attack += attack
	i.life += life
	if endingSide == "" || attack == 0 && life == 0 {
		return
	}
	if i.temporaryStats == nil {
		i.temporaryStats = map[string]ir.Stats{}
	}
	delta := i.temporaryStats[endingSide]
	delta.Attack += attack
	delta.Life += life
	i.storeTemporaryStats(endingSide, delta)
}

func (i *instance) storeTemporaryStats(side string, delta ir.Stats) {
	if delta.Attack == 0 && delta.Life == 0 {
		delete(i.temporaryStats, side)
	} else {
		i.temporaryStats[side] = delta
	}
	if len(i.temporaryStats) == 0 {
		i.temporaryStats = nil
	}
}

func (i *instance) clearTemporaryLife() {
	for side, delta := range i.temporaryStats {
		delta.Life = 0
		i.storeTemporaryStats(side, delta)
	}
}
