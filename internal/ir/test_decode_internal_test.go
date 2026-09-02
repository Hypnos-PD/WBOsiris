package ir

import "testing"

func TestDecodeEveryEventMatcherKind(t *testing.T) {
	id := "0123456789abcdef0123456789abcdef"
	cases := []string{
		`{"kind":"damaged","target":{"kind":"leader","side":"own"},"actual":1}`,
		`{"kind":"healed","target":{"kind":"instance","instanceId":"` + id + `"},"actual":0}`,
		`{"kind":"card_drawn","side":"own","count":1}`,
		`{"kind":"destroyed","subject":{"kind":"instance","instanceId":"` + id + `"}}`,
		`{"kind":"banished","subject":{"kind":"card","cardId":10001110}}`,
		`{"kind":"follower_summoned","cardId":10001110,"count":1}`,
		`{"kind":"zone_moved","instanceId":"` + id + `","to":"hand"}`,
		`{"kind":"zone_moved","reason":"return","subject":{"kind":"instance","instanceId":"` + id + `"},"destination":"deck"}`,
		`{"kind":"evolved","instanceId":"` + id + `"}`,
		`{"kind":"super_evolved","instanceId":"` + id + `"}`,
		`{"kind":"amulet_engaged","instanceId":"` + id + `"}`,
		`{"kind":"attacked","attacker":{"kind":"instance","instanceId":"` + id + `"},"defender":{"kind":"leader","side":"oppo"}}`,
		`{"kind":"turn_started","side":"own"}`,
		`{"kind":"turn_ended","side":"oppo"}`,
		`{"kind":"game_ended","side":"own"}`,
		`{"kind":"resource_changed","side":"own","resource":"pp","direction":"spend","amount":1}`,
	}
	for _, src := range cases {
		if _, err := decodeMatcher([]byte(src)); err != nil {
			t.Errorf("%s: %v", src, err)
		}
	}
}
