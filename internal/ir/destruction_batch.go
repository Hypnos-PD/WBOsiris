package ir

import (
	"encoding/json"
	"fmt"
)

func decodeDestructionTarget(data []byte) (Ref, error) {
	var kind rawKind
	if err := json.Unmarshal(data, &kind); err != nil {
		return nil, err
	}
	if kind.Kind != "destruction_batch" {
		return decodeRef(data)
	}
	var raw struct {
		Kind    string            `json:"kind"`
		Targets []json.RawMessage `json:"targets"`
	}
	if err := strict(data, &raw); err != nil {
		return nil, err
	}
	if len(raw.Targets) < 2 || len(raw.Targets) > 65535 {
		return nil, fmt.Errorf("invalid destruction batch size")
	}
	ref := DestructionBatchRef{Kind: raw.Kind}
	seen := map[string]bool{}
	for _, target := range raw.Targets {
		value, err := decodeRef(target)
		if err != nil {
			return nil, err
		}
		key := "self"
		switch v := value.(type) {
		case SelfRef:
		case BindingRef:
			key = "binding:" + v.Name
		default:
			return nil, fmt.Errorf("destruction batch requires bindings or self")
		}
		if seen[key] {
			return nil, fmt.Errorf("duplicate destruction batch reference")
		}
		seen[key] = true
		ref.Targets = append(ref.Targets, value)
	}
	return ref, nil
}
