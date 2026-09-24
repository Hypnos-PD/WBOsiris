package ir

import (
	"encoding/json"
	"testing"
)

func TestTraitPredicateVocabulary(t *testing.T) {
	for _, name := range []string{"officer", "luminous", "levin", "pixie", "departed", "earthsigil", "mysteria", "golem", "shikigami", "artifact", "puppetry", "marine", "loot", "encroacher", "anathema", "", "bat", "puppetryy"} {
		data, err := json.Marshal(FieldPredicate{Kind: "has_trait", Trait: name})
		if err != nil {
			t.Fatal(err)
		}
		_, err = decodePredicate(data)
		valid := name != "" && name != "bat" && name != "puppetryy"
		if (err == nil) != valid {
			t.Errorf("trait %q: expected valid=%v, got error=%v", name, valid, err)
		}
	}
}
