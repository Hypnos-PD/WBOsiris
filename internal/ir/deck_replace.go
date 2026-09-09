package ir

type DeckEntry struct {
	CardID int `json:"cardId"`
	Count  int `json:"count"`
}

// DeckReplaceEffect creates the listed cards at printed state and shuffles them.
type DeckReplaceEffect struct {
	NodeBase
	Kind  string      `json:"kind"`
	Owner string      `json:"owner"`
	Cards []DeckEntry `json:"cards"`
}

func (e DeckReplaceEffect) effectKind() string   { return e.Kind }
func (e DeckReplaceEffect) effectBase() NodeBase { return e.NodeBase }

func ValidDeckRecipe(cards []DeckEntry) bool {
	total := 0
	if len(cards) == 0 || len(cards) > 65535 {
		return false
	}
	for _, card := range cards {
		if !validCardID(card.CardID) || card.Count < 1 || card.Count > 65535-total {
			return false
		}
		total += card.Count
	}
	return true
}

type LeaderMaxLifeEffect struct {
	NodeBase
	Kind   string `json:"kind"`
	Side   string `json:"side"`
	Amount int    `json:"amount"`
}

func (e LeaderMaxLifeEffect) effectKind() string   { return e.Kind }
func (e LeaderMaxLifeEffect) effectBase() NodeBase { return e.NodeBase }
