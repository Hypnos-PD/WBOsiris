package ir

// HistorySummonEffect creates printed cards from destruction records without moving them.
type HistorySummonEffect struct {
	NodeBase
	Kind     string             `json:"kind"`
	Owner    string             `json:"owner"`
	Source   Ref                `json:"source"`
	Count    int                `json:"count"`
	Extremum *SelectionExtremum `json:"extremum,omitempty"`
	Output   string             `json:"output"`
}

func (e HistorySummonEffect) effectKind() string   { return e.Kind }
func (e HistorySummonEffect) effectBase() NodeBase { return e.NodeBase }

func ValidHistorySummonSource(ref Ref) bool {
	switch r := ref.(type) {
	case ZoneRef:
		return r.Kind == "zone" && validSide(r.Side) && r.Zone == "destroyed" && oneOf(r.Member, "", "card", "follower", "amulet")
	case HistoryRef:
		return validHistoryRef(r)
	case FilterRef:
		_, nested := r.Source.(FilterRef)
		return r.Kind == "filter" && !nested && r.Predicate != nil && ValidHistorySummonSource(r.Source)
	}
	return false
}

func ValidSelectionExtremum(e *SelectionExtremum) bool {
	return e == nil || oneOf(e.Direction, "highest", "lowest") &&
		oneOf(e.Field, "attack", "life", "cost", "base_attack", "base_life", "base_cost")
}
