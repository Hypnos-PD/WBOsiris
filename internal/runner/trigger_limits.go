package runner

import "wbo/internal/ir"

func triggerTurnMatches(scope, sourceSide, activeSide string) bool {
	return scope == "any" || scope == "own" && sourceSide == activeSide || scope == "oppo" && sourceSide != activeSide
}

type TriggerLimitView struct {
	AbilityID string `json:"abilityId"`
	TurnScope string `json:"turnScope"`
	Used      bool   `json:"used"`
}

func triggerLimitViews(i *instance) []TriggerLimitView {
	var views []TriggerLimitView
	for _, ability := range i.card.Abilities {
		trigger, ok := ability.Trigger.(ir.EventTrigger)
		if !ok || trigger.OncePerTurn == "" {
			continue
		}
		views = append(views, TriggerLimitView{AbilityID: ability.ID, TurnScope: trigger.OncePerTurn, Used: i.usedTriggers[ability.ID]})
	}
	return views
}
