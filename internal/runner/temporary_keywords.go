package runner

// KeywordExpiry keeps independent turn deadlines and any permanent grant.
type KeywordExpiry struct {
	OwnTurnEnd  bool `json:"ownTurnEnd,omitempty"`
	OppoTurnEnd bool `json:"oppoTurnEnd,omitempty"`
	Permanent   bool `json:"permanent,omitempty"`
}

func (i *instance) addKeyword(keyword, endingSide string) {
	expiry, temporary := i.temporaryKeywords[keyword]
	if endingSide == "" {
		if temporary {
			expiry.Permanent = true
			i.temporaryKeywords[keyword] = expiry
		}
	} else {
		if !temporary {
			expiry.Permanent = i.abilities[keyword]
		}
		if endingSide == "own" {
			expiry.OwnTurnEnd = true
		} else {
			expiry.OppoTurnEnd = true
		}
		if i.temporaryKeywords == nil {
			i.temporaryKeywords = map[string]KeywordExpiry{}
		}
		i.temporaryKeywords[keyword] = expiry
	}
	i.abilities[keyword] = true
}

func (i *instance) removeKeyword(keyword string) {
	delete(i.abilities, keyword)
	delete(i.temporaryKeywords, keyword)
	if len(i.temporaryKeywords) == 0 {
		i.temporaryKeywords = nil
	}
}

func (g *game) expireKeywords(side string) bool {
	visits := len(g.instances)
	for _, i := range g.instances {
		visits += len(i.temporaryKeywords)
	}
	if !g.chargeQueryVisits(visits) {
		return false
	}
	for _, i := range g.instances {
		for keyword, expiry := range i.temporaryKeywords {
			if side == "own" {
				expiry.OwnTurnEnd = false
			} else {
				expiry.OppoTurnEnd = false
			}
			if expiry.OwnTurnEnd || expiry.OppoTurnEnd {
				i.temporaryKeywords[keyword] = expiry
				continue
			}
			delete(i.temporaryKeywords, keyword)
			if !expiry.Permanent {
				delete(i.abilities, keyword)
			}
		}
		if len(i.temporaryKeywords) == 0 {
			i.temporaryKeywords = nil
		}
	}
	return true
}
