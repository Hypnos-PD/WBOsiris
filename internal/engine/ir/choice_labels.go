package ir

import "strings"

func ValidChoiceLabels(labels map[string]string) bool {
	for locale, text := range labels {
		if !oneOf(locale, "chs", "eng", "jpn", "kor", "cht") || strings.TrimSpace(text) == "" {
			return false
		}
	}
	return true
}
