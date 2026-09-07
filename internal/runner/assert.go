package runner

import (
	"fmt"
	"reflect"

	"wbo/internal/ir"
)

func (g *game) assert(a ir.Assertion) string {
	switch x := a.(type) {
	case ir.BasicAssertion:
		switch x.Kind {
		case "legal":
			if !g.legal {
				return "expected legal, got illegal " + g.illegal
			}
		case "illegal":
			if g.legal {
				return "expected illegal " + x.Code + ", got legal"
			}
			if x.Code != g.illegal {
				return fmt.Sprintf("expected illegal %s, got %s", x.Code, g.illegal)
			}
		case "unchanged":
			if !g.unchanged {
				return "expected unchanged state"
			}
		}
	case ir.CompareAssertion:
		return g.assertCompare(x)
	case ir.KeywordAssertion:
		return g.assertKeyword(x)
	case ir.ZoneAssertion:
		return g.assertZone(x)
	case ir.EventsAssertion:
		return g.assertEvents(x)
	}
	return ""
}
func (g *game) assertCompare(a ir.CompareAssertion) string {
	var got any
	switch a.Left.Kind {
	case "player_pp_pair":
		p := g.player(a.Left.Side)
		got = [2]int{p.pp, p.maxpp}
	case "player_field":
		p := g.player(a.Left.Side)
		switch a.Left.Field {
		case "leader.life":
			got = p.leaderLife
		case "leader.maxlife":
			got = p.leaderMax
		case "pp":
			got = p.pp
		case "maxpp":
			got = p.maxpp
		case "ep":
			got = p.ep
		case "sep":
			got = p.sep
		case "combo":
			got = p.combo
		case "shadows":
			got = p.shadows
		}
	case "instance_counter":
		i := g.instances[a.Left.InstanceID]
		if i == nil {
			return "unknown instance"
		}
		value, exists := i.counters[a.Left.Field]
		if !exists {
			return "unknown instance counter"
		}
		got = value
	case "instance_field":
		i := g.instances[a.Left.InstanceID]
		if i == nil {
			return "unknown instance"
		}
		switch a.Left.Field {
		case "zone":
			got = i.zone
		case "cost":
			got = i.cost
		case "stats":
			got = [2]int{i.attack, i.life}
		case "evolved":
			got = i.evolved
		case "super_evolved":
			got = i.superEvolved
		case "earthsigil":
			got = i.earthsigil
		case "countdown":
			got = i.countdown
		case "engaged":
			got = i.engaged
		}
	case "rng_consumed":
		got = int(g.rng.Consumed())
	}
	var want any
	switch a.Right.Kind {
	case "integer":
		want = a.Right.Integer
	case "boolean":
		want = a.Right.Boolean
	case "zone":
		want = a.Right.Value
	case "stats":
		want = [2]int{a.Right.Attack, a.Right.Life}
	case "pp":
		want = [2]int{a.Right.Current, a.Right.Maximum}
	}
	if !reflect.DeepEqual(got, want) {
		return fmt.Sprintf("expected %v, got %v", want, got)
	}
	return ""
}
func (g *game) assertKeyword(a ir.KeywordAssertion) string {
	if a.Kind == "has_keyword" {
		i := g.instances[a.Target]
		if i == nil {
			return "unknown instance"
		}
		if i.abilities[a.Keyword] != a.Expected {
			return fmt.Sprintf("expected %s %s=%t", i.alias, a.Keyword, a.Expected)
		}
		return ""
	}
	for _, i := range g.fromRef(a.Source, nil, frame{}) {
		if !i.abilities[a.Keyword] {
			return fmt.Sprintf("expected all matching cards to have %s", a.Keyword)
		}
	}
	return ""
}
func (g *game) assertZone(a ir.ZoneAssertion) string {
	if a.Kind == "rng_consumed" {
		return compareInt("rng.consumed", int(g.rng.Consumed()), a.Count)
	}
	if a.Kind == "zone_count" {
		p := g.player(a.Side)
		n := 0
		for _, i := range g.zone(p, a.Zone) {
			if i.card.ID == a.CardID {
				n++
			}
		}
		return compareInt(a.Side+"."+a.Zone+" count", n, a.Count)
	}
	if a.Kind == "ordered_instances" {
		items := g.fromRef(a.Source, nil, frame{})
		n := 0
		for _, i := range items {
			if n < len(a.Expected) && i.id == a.Expected[n] {
				n++
			}
		}
		if n != len(a.Expected) {
			return fmt.Sprintf("expected ordered instances %v", a.Expected)
		}
		if a.Containment == "exact" && len(items) != len(a.Expected) {
			return fmt.Sprintf("expected exact instances %v", a.Expected)
		}
	}
	return ""
}
func (g *game) assertEvents(a ir.EventsAssertion) string {
	matches := func(m ir.EventMatcher, e ir.RuntimeEvent) bool {
		if m.Kind != e.Kind {
			return false
		}
		if m.Side != "" && m.Side != e.Side {
			return false
		}
		if m.Count != 0 && m.Count != e.Count {
			return false
		}
		if m.CardID != 0 && m.CardID != e.CardID {
			return false
		}
		if m.InstanceID != "" && m.InstanceID != e.InstanceID {
			return false
		}
		if m.Actual != 0 && m.Actual != e.Actual {
			return false
		}
		if m.Target != nil && !targetEqual(m.Target, e.Target) {
			return false
		}
		if m.Subject != nil && !targetEqual(m.Subject, e.Subject) {
			return false
		}
		if m.Attacker != nil && !targetEqual(m.Attacker, e.Attacker) {
			return false
		}
		if m.Defender != nil && !targetEqual(m.Defender, e.Defender) {
			return false
		}
		return true
	}
	if a.Mode == "excludes" {
		for _, m := range a.Expected {
			for _, e := range g.events {
				if matches(m, e) {
					return "unexpected matching event"
				}
			}
		}
		return ""
	}
	n := 0
	for _, e := range g.events {
		if n < len(a.Expected) && matches(a.Expected[n], e) {
			n++
		}
	}
	if n != len(a.Expected) {
		return fmt.Sprintf("expected ordered events %v, got %v", a.Expected, g.events)
	}
	if a.Mode == "exact" && len(g.events) != len(a.Expected) {
		return fmt.Sprintf("expected exact events %v, got %v", a.Expected, g.events)
	}
	return ""
}
func targetEqual(a, b *ir.EventTarget) bool {
	return a != nil && b != nil && a.Kind == b.Kind && a.InstanceID == b.InstanceID && a.Side == b.Side && (a.CardID == 0 || a.CardID == b.CardID)
}
func (g *game) player(side string) *player {
	if side == "oppo" {
		return &g.oppo
	}
	return &g.own
}
func compareInt(name string, got, want int) string {
	if got != want {
		return fmt.Sprintf("expected %s %d, got %d", name, want, got)
	}
	return ""
}
