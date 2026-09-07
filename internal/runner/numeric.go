package runner

import "wbo/internal/ir"

func (g *game) numericValue(expr ir.NumericExpr, self *instance, bindings frame) int {
	switch e := expr.(type) {
	case *ir.SumExpr:
		total := 0
		for _, i := range g.fromRef(e.Source, self, bindings) {
			if !g.chargeQueryVisits(1) {
				return 0
			}
			if e.Field == "base_cost" {
				total += i.card.Cost
			} else if i.card.Stats != nil {
				if e.Field == "base_attack" {
					total += i.card.Stats.Attack
				} else if e.Field == "base_life" {
					total += i.card.Stats.Life
				}
			}
		}
		return total
	case *ir.CountExpr:
		if binding, ok := e.Source.(ir.BindingRef); ok {
			return len(bindings[binding.Name])
		}
		return len(g.fromRef(e.Source, self, bindings))
	case *ir.NegateExpr:
		return -g.numericValue(e.Value, self, bindings)
	case *ir.Scalar:
		if e.Kind == "self_counter" {
			if self == nil {
				return 0
			}
			return self.counters[e.Field]
		}
		if e.Kind == "self_scalar" {
			if self == nil {
				return 0
			}
			switch e.Field {
			case "attack":
				return self.attack
			case "life":
				return self.life
			case "cost":
				return max(0, self.cost)
			}
		}
		p, _ := g.playerForSide(self, e.Side)
		switch e.Field {
		case "hand_count":
			return len(p.hand)
		case "earthsigils":
			total := 0
			for _, i := range p.field {
				if !g.chargeQueryVisits(1) {
					return 0
				}
				total += i.earthsigil
			}
			return total
		case "combo":
			return p.combo
		case "pp":
			return p.pp
		case "maxpp":
			return p.maxpp
		case "life":
			return max(0, p.leaderLife)
		case "ep":
			return p.ep
		case "sep":
			return p.sep
		case "shadows":
			return p.shadows
		}
	}
	return 0
}
