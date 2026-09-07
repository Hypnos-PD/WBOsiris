package runner

import "wbo/internal/ir"

// Selecting changes bindings, but leaves these candidate sources unchanged.
func independentSelectionSource(ref ir.Ref) bool {
	switch r := ref.(type) {
	case ir.ZoneRef, ir.CharacterSetRef:
		return true
	case ir.FilterRef:
		return independentSelectionSource(r.Source)
	case ir.ExcludeRef:
		_, self := r.Value.(ir.SelfRef)
		return self && independentSelectionSource(r.Source)
	default:
		return false
	}
}
