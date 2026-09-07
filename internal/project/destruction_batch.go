package project

import "wbo/internal/syntax"

func destructionBatchTargets(t []syntax.Token) ([]syntax.Token, bool) {
	if len(t) < 4 || len(t)%2 != 0 || t[0].Value != "destroy" {
		return nil, false
	}
	var targets []syntax.Token
	seen := map[string]bool{}
	for n := 1; n < len(t); n += 2 {
		if t[n].Kind != syntax.Identifier || seen[t[n].Value] || n > 1 && t[n-1].Value != "," {
			return nil, false
		}
		seen[t[n].Value] = true
		targets = append(targets, t[n])
	}
	return targets, true
}
