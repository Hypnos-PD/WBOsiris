package runner

import "wbo/internal/ir"

func (g *game) distributeDamage(source *instance, targets []*instance, amount int, overflow ir.Ref) {
	remaining := max(amount, 0)
	allocations := make([]int, len(targets))
	// Allocate from current life before applying shields, reductions or death processing.
	for n, target := range targets {
		allocated := min(remaining, max(target.life, 0))
		if n == len(targets)-1 && overflow == nil {
			allocated = remaining
		}
		allocations[n] = allocated
		remaining -= allocated
	}
	for n, target := range targets {
		if allocations[n] > 0 {
			g.damageInstanceFrom(source, target, allocations[n], "effect")
		}
	}
	if leader, ok := overflow.(ir.LeaderRef); ok && remaining > 0 {
		player, side := g.playerForSide(source, leader.Side)
		g.damageLeaderFrom(source, player, side, remaining)
	}
	g.resolveDeathBatch(nil)
}
