package runner

import (
	"sort"

	"wbo/internal/ir"
)

func (g *game) numericValue(expr ir.NumericExpr, self *instance, bindings frame) int {
	switch e := expr.(type) {
	case *ir.SumExpr:
		value := func(i *instance) int {
			switch e.Field {
			case "cost":
				return max(0, i.cost)
			case "attack":
				if i.card.Stats != nil {
					return i.currentAttack()
				}
			case "life":
				if i.card.Stats != nil {
					return i.life
				}
			case "base_cost":
				return i.card.Cost
			case "base_attack":
				if i.card.Stats != nil {
					return i.card.Stats.Attack
				}
			case "base_life":
				if i.card.Stats != nil {
					return i.card.Stats.Life
				}
			}
			return 0
		}
		items := g.fromRef(e.Source, self, bindings)
		if e.Limit > 0 && len(items) > e.Limit {
			// "原始费用最大的3张卡牌的费用合计"：先按字段取极值，再求和。
			sorted := append([]*instance(nil), items...)
			sort.SliceStable(sorted, func(a, b int) bool {
				if e.Direction == "lowest" {
					return value(sorted[a]) < value(sorted[b])
				}
				return value(sorted[a]) > value(sorted[b])
			})
			items = sorted[:e.Limit]
		}
		total := 0
		for _, i := range items {
			if !g.chargeQueryVisits(1) {
				return 0
			}
			total += value(i)
		}
		return total
	case *ir.CountExpr:
		if binding, ok := e.Source.(ir.BindingRef); ok {
			return len(bindings[binding.Name])
		}
		return len(g.fromRef(e.Source, self, bindings))
	case *ir.NegateExpr:
		return -g.numericValue(e.Value, self, bindings)
	case *ir.DifferenceExpr:
		return g.numericValue(e.Left, self, bindings) - g.numericValue(e.Right, self, bindings)
	case *ir.Scalar:
		if e.Kind == "self_counter" {
			if self == nil {
				return 0
			}
			return self.counters[e.Field]
		}
		if e.Kind == "fusion_material_scalar" {
			// fused.cost 是材料原始费用合计，fused.distinct 是按卡牌 ID 计算的种类数。
			if self == nil {
				return 0
			}
			total := 0
			seen := map[int]bool{}
			for _, material := range self.materials {
				total += material.card.Cost
				seen[material.card.ID] = true
			}
			if e.Field == "distinct" {
				return len(seen)
			}
			return total
		}
		if e.Kind == "self_scalar" {
			if self == nil {
				return 0
			}
			switch e.Field {
			case "attack":
				return self.currentAttack()
			case "life":
				return self.life
			case "cost":
				return max(0, self.cost)
			case "damage_taken":
				return self.damageTaken
			}
		}
		if e.Kind == "binding_scalar" {
			// `<绑定>.attack|life|cost`：读取绑定里第一个实例的当前数值；
			// `base_*` 读取它的卡牌定义（"使用的卡牌的原始费用"）。
			for _, bound := range bindings[e.Side] {
				item := g.instances[bound.InstanceID]
				if item == nil {
					continue
				}
				switch e.Field {
				case "attack":
					return item.currentAttack()
				case "life":
					return item.life
				case "cost":
					return max(0, item.cost)
				case "base_attack":
					if item.card.Stats != nil {
						return item.card.Stats.Attack
					}
				case "base_life":
					if item.card.Stats != nil {
						return item.card.Stats.Life
					}
				case "base_cost":
					return item.card.Cost
				}
			}
			return 0
		}
		p, _ := g.playerForSide(self, e.Side)
		switch e.Field {
		case "hand_count":
			return len(p.hand)
		case "crests":
			return len(p.crests)
		case "earthsigils":
			total := 0
			for _, i := range p.field {
				if !g.chargeQueryVisits(1) {
					return 0
				}
				total += i.earthsigil
			}
			return total
		case "entered_artifacts":
			return len(p.enteredArtifacts)
		case "evolutions":
			return p.evolutionsThisMatch
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
		case "rally":
			return p.rally
		}
	}
	return 0
}
