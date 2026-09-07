package ir

import (
	"encoding/json"
	"fmt"
)

func decodeSelectionSource(data []byte) (Ref, error) {
	var kind rawKind
	if err := json.Unmarshal(data, &kind); err != nil {
		return nil, err
	}
	if kind.Kind != "characters" {
		return decodeRef(data)
	}
	var ref CharacterSetRef
	if err := strict(data, &ref); err != nil {
		return nil, err
	}
	if !validSide(ref.Side) {
		return nil, fmt.Errorf("invalid character set side")
	}
	return ref, nil
}
