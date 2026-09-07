package ir

func validDamageDistribution(kind, distribution string, target, overflow Ref) bool {
	if distribution == "" {
		return overflow == nil
	}
	zone, ok := target.(ZoneRef)
	if kind != "damage" || distribution != "field_entry_order" || !ok || !validSide(zone.Side) || zone.Zone != "field" || zone.Member != "follower" {
		return false
	}
	if overflow == nil {
		return true
	}
	leader, ok := overflow.(LeaderRef)
	return ok && leader.Kind == "leader" && leader.Side == zone.Side && leader.ValueType == "leader"
}
