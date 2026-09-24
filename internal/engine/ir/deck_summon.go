package ir

// DeckSummonEffect moves sampled physical cards from the controller's deck to its field.
type DeckSummonEffect struct {
	NodeBase
	Kind          string `json:"kind"`
	Source        Ref    `json:"source"`
	Count         int    `json:"count"`
	DistinctNames bool   `json:"distinctNames,omitempty"`
	Output        string `json:"output"`
}

func (e DeckSummonEffect) effectKind() string   { return e.Kind }
func (e DeckSummonEffect) effectBase() NodeBase { return e.NodeBase }

func ValidDeckSummonSource(ref Ref) bool {
	switch r := ref.(type) {
	case ZoneRef:
		return r.Kind == "zone" && r.Side == "own" && r.Zone == "deck" && oneOf(r.Member, "follower", "amulet")
	case FilterRef:
		_, nested := r.Source.(FilterRef)
		return r.Kind == "filter" && !nested && r.Predicate != nil && ValidDeckSummonSource(r.Source)
	}
	return false
}
