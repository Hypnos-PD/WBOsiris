package runner

import "wbo/internal/ir"

func (g *game) numericValue(expr ir.NumericExpr, self *instance, bindings frame) int {
	switch e := expr.(type) {
	case *ir.CountExpr:
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
