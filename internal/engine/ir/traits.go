package ir

// ValidTrait is shared by card declarations and predicate validation.
func ValidTrait(trait string) bool {
	switch trait {
	case "officer", "luminous", "levin", "pixie", "departed", "earthsigil",
		"mysteria", "golem", "shikigami", "artifact", "puppetry", "marine",
		"loot", "encroacher", "anathema":
		return true
	default:
		return false
	}
}
