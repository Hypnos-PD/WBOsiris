package ir

func ValidTransformTarget(ref Ref) bool {
	switch r := ref.(type) {
	case SelfRef, BindingRef:
		return true
	case ZoneRef:
		return r.Zone == "hand" || r.Zone == "deck" || r.Zone == "field"
	case FilterRef:
		return ValidTransformTarget(r.Source)
	case ExcludeRef:
		return ValidTransformTarget(r.Source) && ValidTransformTarget(r.Value)
	default:
		return false
	}
}
