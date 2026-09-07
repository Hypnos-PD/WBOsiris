package project

import "wbo/internal/syntax"

func parseSelectionResponse(t []syntax.Token) (entities, leaders []syntax.Token, valid bool) {
	if len(t) < 2 || t[0].Value != "select" {
		return nil, nil, false
	}
	seen := map[string]bool{}
	for n := 1; n < len(t); {
		value := t[n]
		key := "entity:" + value.Value
		if n+2 < len(t) && set("own", "oppo")[value.Value] && t[n+1].Value == "." && t[n+2].Value == "leader" {
			key = "leader:" + value.Value
			leaders = append(leaders, value)
			n += 3
		} else {
			if value.Kind != syntax.Identifier {
				return nil, nil, false
			}
			entities = append(entities, value)
			n++
		}
		if seen[key] {
			return nil, nil, false
		}
		seen[key] = true
		if n < len(t) {
			if t[n].Value != "," || n+1 == len(t) {
				return nil, nil, false
			}
			n++
		}
	}
	return entities, leaders, true
}
