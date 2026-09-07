package ir

func HasHistoryRef(ref Ref) bool {
	switch r := ref.(type) {
	case ZoneRef:
		return r.Zone == "destroyed"
	case FilterRef:
		return HasHistoryRef(r.Source)
	case ExcludeRef:
		return HasHistoryRef(r.Source) || HasHistoryRef(r.Value)
	case DestructionBatchRef:
		for _, target := range r.Targets {
			if HasHistoryRef(target) {
				return true
			}
		}
	}
	return false
}

func hasHistoryEffectTarget(effect Effect) bool {
	switch e := effect.(type) {
	case SelectionEffect:
		return HasHistoryRef(e.Source)
	case CardEffect:
		return HasHistoryRef(e.Target)
	case TargetEffect:
		return HasHistoryRef(e.Target)
	case AdjustEffect:
		return HasHistoryRef(e.Target)
	case GrantEffect:
		return HasHistoryRef(e.Target)
	}
	return false
}
