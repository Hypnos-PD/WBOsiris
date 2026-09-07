package ir

import (
	"fmt"
	"math"
	"regexp"
)

const MaxCounterValue = math.MaxInt32

var counterNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)

func ValidCounterName(name string) bool { return counterNamePattern.MatchString(name) }

func ValidCounters(counters map[string]int) bool {
	for name, value := range counters {
		if !ValidCounterName(name) || value < 0 || value > MaxCounterValue {
			return false
		}
	}
	return true
}

func validateCounterRefs(card Card) error {
	if !ValidCounters(card.Counters) {
		return fmt.Errorf("invalid card counters")
	}
	check := func(name string) error {
		if _, ok := card.Counters[name]; !ok {
			return fmt.Errorf("undeclared counter %q", name)
		}
		return nil
	}
	var numeric func(NumericExpr) error
	numeric = func(expr NumericExpr) error {
		switch e := expr.(type) {
		case *Scalar:
			if e.Kind == "self_counter" {
				return check(e.Field)
			}
		case *NegateExpr:
			return numeric(e.Value)
		}
		return nil
	}
	var walk func([]Effect) error
	walk = func(effects []Effect) error {
		for _, effect := range effects {
			switch e := effect.(type) {
			case AdjustEffect:
				if e.Kind == "adjust_counter" {
					if err := check(e.Field); err != nil {
						return err
					}
				}
			case TargetEffect:
				for _, expr := range []NumericExpr{e.AmountExpr, e.AttackExpr, e.LifeExpr} {
					if err := numeric(expr); err != nil {
						return err
					}
				}
			case RepeatEffect:
				if err := numeric(e.TimesExpr); err != nil {
					return err
				}
				if err := walk(e.Body); err != nil {
					return err
				}
			case IfEffect:
				if condition, ok := e.Condition.(CompareCondition); ok && condition.Left.Kind == "self_counter" {
					if err := check(condition.Left.Field); err != nil {
						return err
					}
				}
				if err := walk(e.Then); err != nil {
					return err
				}
				if err := walk(e.Else); err != nil {
					return err
				}
			case ModeEffect:
				for _, option := range e.Options {
					if err := walk(option.Body); err != nil {
						return err
					}
				}
			case PayResourceEffect:
				if err := walk(e.OnPaid); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := walk(card.PlayEffects); err != nil {
		return err
	}
	for _, ability := range card.Abilities {
		if err := walk(ability.Body); err != nil {
			return err
		}
	}
	for _, ability := range card.FusionAbilities {
		if err := walk(ability.Body); err != nil {
			return err
		}
	}
	return nil
}
