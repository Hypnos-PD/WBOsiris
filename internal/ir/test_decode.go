package ir

import (
	"encoding/json"
	"fmt"
)

func decodeScenario(data []byte) (Scenario, error) {
	type raw struct {
		ID           string            `json:"id"`
		Name         string            `json:"name"`
		Seed         string            `json:"seed"`
		InitialState json.RawMessage   `json:"initialState"`
		Actions      []json.RawMessage `json:"actions"`
		Assertions   []json.RawMessage `json:"assertions"`
		Origin       Origin            `json:"origin"`
	}
	var v raw
	if err := strict(data, &v); err != nil {
		return Scenario{}, fmt.Errorf("scenario: %w", err)
	}
	if !nodeIDPattern.MatchString(v.ID) {
		return Scenario{}, fmt.Errorf("malformed scenario ID %q", v.ID)
	}
	if !validOrigin(v.Origin) {
		return Scenario{}, fmt.Errorf("malformed scenario origin")
	}
	if _, err := parseSeed(v.Seed); err != nil {
		return Scenario{}, err
	}
	s := Scenario{ID: v.ID, Name: v.Name, Seed: v.Seed, Origin: v.Origin}
	var err error
	s.InitialState, err = decodeState(v.InitialState)
	if err != nil {
		return Scenario{}, err
	}
	for _, x := range v.Actions {
		a, err := decodeAction(x)
		if err != nil {
			return Scenario{}, err
		}
		s.Actions = append(s.Actions, a)
	}
	for _, x := range v.Assertions {
		a, err := decodeAssertion(x)
		if err != nil {
			return Scenario{}, err
		}
		s.Assertions = append(s.Assertions, a)
	}
	if err := validateScenarioReferences(s); err != nil {
		return Scenario{}, err
	}
	return s, nil
}

func decodeState(data []byte) (State, error) {
	type raw struct {
		Turn    Turn                       `json:"turn"`
		Phase   string                     `json:"phase"`
		Players map[string]json.RawMessage `json:"players"`
		Aliases map[string]string          `json:"aliases"`
	}
	var v raw
	if err := strict(data, &v); err != nil {
		return State{}, fmt.Errorf("state: %w", err)
	}
	if !oneOf(v.Turn.Active, "own", "oppo") || v.Turn.Number < 0 || v.Phase != "main" || len(v.Players) != 2 {
		return State{}, fmt.Errorf("malformed test state")
	}
	s := State{Turn: v.Turn, Phase: v.Phase, Players: map[string]PlayerState{}, Aliases: v.Aliases}
	instances := map[string]bool{}
	for _, side := range []string{"own", "oppo"} {
		rawPlayer, ok := v.Players[side]
		if !ok {
			return State{}, fmt.Errorf("missing player %s", side)
		}
		p, err := decodePlayer(rawPlayer, instances)
		if err != nil {
			return State{}, err
		}
		s.Players[side] = p
	}
	for alias, id := range v.Aliases {
		if alias == "" || !nodeIDPattern.MatchString(id) || !instances[id] {
			return State{}, fmt.Errorf("bad alias %q", alias)
		}
	}
	return s, nil
}
func decodePlayer(data []byte, instances map[string]bool) (PlayerState, error) {
	type raw struct {
		Leader  Leader                       `json:"leader"`
		PP      int                          `json:"pp"`
		MaxPP   int                          `json:"maxpp"`
		EP      int                          `json:"ep"`
		SEP     int                          `json:"sep"`
		Combo   int                          `json:"combo"`
		Shadows int                          `json:"shadows"`
		Zones   map[string][]json.RawMessage `json:"zones"`
	}
	var v raw
	if err := strict(data, &v); err != nil {
		return PlayerState{}, fmt.Errorf("player: %w", err)
	}
	if len(v.Zones) != 6 {
		return PlayerState{}, fmt.Errorf("malformed zones")
	}
	p := PlayerState{Leader: v.Leader, PP: v.PP, MaxPP: v.MaxPP, EP: v.EP, SEP: v.SEP, Combo: v.Combo, Shadows: v.Shadows, Zones: map[string][]TestInstance{}}
	for _, zone := range []string{"deck", "hand", "field", "graveyard", "banished", "destroyed"} {
		items, ok := v.Zones[zone]
		if !ok {
			return PlayerState{}, fmt.Errorf("missing zone %s", zone)
		}
		for _, x := range items {
			i, err := decodeInstance(x)
			if err != nil {
				return PlayerState{}, err
			}
			if instances[i.InstanceID] {
				return PlayerState{}, fmt.Errorf("duplicate instance ID %s", i.InstanceID)
			}
			instances[i.InstanceID] = true
			p.Zones[zone] = append(p.Zones[zone], i)
		}
	}
	return p, nil
}
func decodeInstance(data []byte) (TestInstance, error) {
	type overrides struct {
		Stats        *Stats   `json:"stats,omitempty"`
		Evolved      *bool    `json:"evolved,omitempty"`
		SuperEvolved *bool    `json:"super_evolved,omitempty"`
		Engaged      *bool    `json:"engaged,omitempty"`
		Keywords     []string `json:"keywords,omitempty"`
		Countdown    *int     `json:"countdown,omitempty"`
		Earthsigil   *int     `json:"earthsigil,omitempty"`
	}
	type raw struct {
		InstanceID   string    `json:"instanceId"`
		Alias        string    `json:"alias"`
		CardID       int       `json:"cardId"`
		DeclaredType string    `json:"declaredType"`
		Overrides    overrides `json:"overrides"`
	}
	var v raw
	if err := strict(data, &v); err != nil {
		return TestInstance{}, fmt.Errorf("instance: %w", err)
	}
	if !nodeIDPattern.MatchString(v.InstanceID) || v.Alias == "" || !validCardID(v.CardID) || !oneOf(v.DeclaredType, "follower", "spell", "amulet") {
		return TestInstance{}, fmt.Errorf("malformed test instance")
	}
	o := InstanceOverrides{v.Overrides.Stats, v.Overrides.Evolved, v.Overrides.SuperEvolved, v.Overrides.Engaged, v.Overrides.Keywords, v.Overrides.Countdown, v.Overrides.Earthsigil}
	return TestInstance{v.InstanceID, v.Alias, v.CardID, v.DeclaredType, o}, nil
}

func decodeAction(data []byte) (Action, error) {
	var k rawKind
	if err := json.Unmarshal(data, &k); err != nil {
		return nil, err
	}
	switch k.Kind {
	case "play", "engage", "evolve", "superevolve":
		var v struct {
			Kind   string `json:"kind"`
			Actor  string `json:"actor"`
			Source string `json:"source"`
		}
		if err := strict(data, &v); err != nil {
			return nil, err
		}
		if !validSide(v.Actor) || !nodeIDPattern.MatchString(v.Source) {
			return nil, fmt.Errorf("malformed action source")
		}
		return SourceAction{v.Kind, v.Actor, v.Source}, nil
	case "select":
		var v struct {
			Kind   string `json:"kind"`
			Target string `json:"target"`
		}
		if err := strict(data, &v); err != nil {
			return nil, err
		}
		if !nodeIDPattern.MatchString(v.Target) {
			return nil, fmt.Errorf("malformed selection")
		}
		return SelectAction{v.Kind, v.Target}, nil
	case "select_mode":
		var v struct {
			Kind     string `json:"kind"`
			OptionID int    `json:"optionId"`
		}
		if err := strict(data, &v); err != nil {
			return nil, err
		}
		if v.OptionID <= 0 || v.OptionID > 65535 {
			return nil, fmt.Errorf("malformed mode selection")
		}
		return ModeAction{v.Kind, v.OptionID}, nil
	case "attack_entity", "attack_leader":
		var v struct {
			Kind     string `json:"kind"`
			Actor    string `json:"actor"`
			Attacker string `json:"attacker"`
			Defender string `json:"defender"`
		}
		if err := strict(data, &v); err != nil {
			return nil, err
		}
		if !validSide(v.Actor) || !nodeIDPattern.MatchString(v.Attacker) || v.Kind == "attack_entity" && !nodeIDPattern.MatchString(v.Defender) || v.Kind == "attack_leader" && !validSide(v.Defender) {
			return nil, fmt.Errorf("malformed attack action")
		}
		return AttackAction{v.Kind, v.Actor, v.Attacker, v.Defender}, nil
	case "end_turn":
		var v struct {
			Kind  string `json:"kind"`
			Actor string `json:"actor"`
		}
		if err := strict(data, &v); err != nil {
			return nil, err
		}
		if !validSide(v.Actor) {
			return nil, fmt.Errorf("malformed end-turn actor")
		}
		return SourceAction{Kind: v.Kind, Actor: v.Actor}, nil
	case "advance":
		var v struct {
			Kind   string `json:"kind"`
			Timing string `json:"timing"`
			Side   string `json:"side"`
		}
		if err := strict(data, &v); err != nil {
			return nil, err
		}
		if !oneOf(v.Timing, "turn_start", "turn_end") || !validSide(v.Side) {
			return nil, fmt.Errorf("malformed advance action")
		}
		return AdvanceAction{v.Kind, v.Timing, v.Side}, nil
	default:
		return nil, fmt.Errorf("unknown executable action kind %q", k.Kind)
	}
}
func decodeAssertion(data []byte) (Assertion, error) {
	var k rawKind
	if err := json.Unmarshal(data, &k); err != nil {
		return nil, err
	}
	var base struct {
		Origin Origin `json:"origin"`
	}
	if err := json.Unmarshal(data, &base); err != nil || !validOrigin(base.Origin) {
		return nil, fmt.Errorf("malformed assertion origin")
	}
	switch k.Kind {
	case "legal", "unchanged":
		var v struct {
			Kind   string `json:"kind"`
			Origin Origin `json:"origin"`
		}
		if err := strict(data, &v); err != nil {
			return nil, err
		}
		return BasicAssertion{Kind: v.Kind, Origin: v.Origin}, nil
	case "illegal":
		var v struct {
			Kind   string `json:"kind"`
			Code   string `json:"code"`
			Origin Origin `json:"origin"`
		}
		if err := strict(data, &v); err != nil {
			return nil, err
		}
		return BasicAssertion{v.Kind, v.Code, v.Origin}, nil
	case "compare_value":
		var v struct {
			Kind   string          `json:"kind"`
			Left   json.RawMessage `json:"left"`
			Op     string          `json:"op"`
			Right  json.RawMessage `json:"right"`
			Origin Origin          `json:"origin"`
		}
		if err := strict(data, &v); err != nil {
			return nil, err
		}
		l, err := decodeTestRef(v.Left)
		if err != nil {
			return nil, err
		}
		if !oneOf(v.Op, "eq", "ne", "lt", "le", "gt", "ge") {
			return nil, fmt.Errorf("bad assertion operator")
		}
		r, err := decodeLiteral(v.Right)
		return CompareAssertion{v.Kind, v.Op, l, r, v.Origin}, err
	case "has_keyword", "all_have_keyword":
		var v struct {
			Kind     string          `json:"kind"`
			Target   string          `json:"target,omitempty"`
			Keyword  string          `json:"keyword"`
			Expected bool            `json:"expected,omitempty"`
			Source   json.RawMessage `json:"source,omitempty"`
			Origin   Origin          `json:"origin"`
		}
		if err := strict(data, &v); err != nil {
			return nil, err
		}
		var src Ref
		var err error
		if len(v.Source) > 0 {
			src, err = decodeRef(v.Source)
		}
		if v.Keyword == "" || v.Kind == "has_keyword" && !nodeIDPattern.MatchString(v.Target) || v.Kind == "all_have_keyword" && src == nil {
			return nil, fmt.Errorf("malformed keyword assertion")
		}
		return KeywordAssertion{v.Kind, v.Target, v.Keyword, v.Expected, src, v.Origin}, err
	case "zone_count", "ordered_instances":
		var v struct {
			Kind, Side, Zone, Op, Containment string
			CardID, Count                     int
			Source                            json.RawMessage `json:"source,omitempty"`
			Expected                          []string        `json:"expected,omitempty"`
			Origin                            Origin          `json:"origin"`
		}
		if err := strict(data, &v); err != nil {
			return nil, err
		}
		var src Ref
		var err error
		if len(v.Source) > 0 {
			src, err = decodeRef(v.Source)
		}
		if v.Kind == "zone_count" && (!validSide(v.Side) || !validZone(v.Zone) || v.Op != "eq" || !validCardID(v.CardID) || v.Count < 0 || src != nil || v.Containment != "" || len(v.Expected) != 0) {
			return nil, fmt.Errorf("malformed zone count assertion")
		}
		if v.Kind == "ordered_instances" && (src == nil || !oneOf(v.Containment, "exact", "subsequence") || v.Side != "" || v.Zone != "" || v.Op != "" || v.CardID != 0 || v.Count != 0) {
			return nil, fmt.Errorf("malformed ordered assertion")
		}
		for _, id := range v.Expected {
			if !nodeIDPattern.MatchString(id) {
				return nil, fmt.Errorf("bad expected instance ID")
			}
		}
		return ZoneAssertion{v.Kind, v.Side, v.Zone, v.Op, v.Containment, v.CardID, v.Count, src, v.Expected, v.Origin}, err
	case "events":
		var v struct {
			Kind     string            `json:"kind"`
			Mode     string            `json:"mode"`
			Expected []json.RawMessage `json:"expected"`
			Origin   Origin            `json:"origin"`
		}
		if err := strict(data, &v); err != nil {
			return nil, err
		}
		if !oneOf(v.Mode, "contains_ordered", "excludes", "exact") {
			return nil, fmt.Errorf("malformed event assertion mode")
		}
		a := EventsAssertion{Kind: v.Kind, Mode: v.Mode, Origin: v.Origin}
		for _, x := range v.Expected {
			m, err := decodeMatcher(x)
			if err != nil {
				return nil, err
			}
			a.Expected = append(a.Expected, m)
		}
		return a, nil
	case "rng_consumed":
		var v struct {
			Kind   string `json:"kind"`
			Op     string `json:"op"`
			Count  int    `json:"count"`
			Origin Origin `json:"origin"`
		}
		if err := strict(data, &v); err != nil {
			return nil, err
		}
		if v.Op != "eq" || v.Count < 0 {
			return nil, fmt.Errorf("malformed RNG assertion")
		}
		return ZoneAssertion{Kind: v.Kind, Op: v.Op, Count: v.Count, Origin: v.Origin}, nil
	default:
		return nil, fmt.Errorf("unknown assertion kind %q", k.Kind)
	}
}
func decodeTestRef(data []byte) (TestRef, error) {
	var k rawKind
	if err := json.Unmarshal(data, &k); err != nil {
		return TestRef{}, err
	}
	switch k.Kind {
	case "player_field", "player_pp_pair":
		var v struct {
			Kind  string `json:"kind"`
			Side  string `json:"side"`
			Field string `json:"field,omitempty"`
		}
		if err := strict(data, &v); err != nil {
			return TestRef{}, err
		}
		if !validSide(v.Side) || v.Kind == "player_pp_pair" && v.Field != "" || v.Kind == "player_field" && !oneOf(v.Field, "leader.life", "leader.maxlife", "pp", "maxpp", "ep", "sep", "combo", "shadows") {
			return TestRef{}, fmt.Errorf("malformed player reference")
		}
		return TestRef{Kind: v.Kind, Side: v.Side, Field: v.Field}, nil
	case "instance_field":
		var v struct {
			Kind       string `json:"kind"`
			InstanceID string `json:"instanceId"`
			Field      string `json:"field"`
		}
		if err := strict(data, &v); err != nil {
			return TestRef{}, err
		}
		if !nodeIDPattern.MatchString(v.InstanceID) || !oneOf(v.Field, "zone", "stats", "attack", "life", "evolved", "super_evolved", "engaged", "countdown", "earthsigil") {
			return TestRef{}, fmt.Errorf("malformed instance reference")
		}
		return TestRef{Kind: v.Kind, InstanceID: v.InstanceID, Field: v.Field}, nil
	case "rng_consumed":
		var v struct {
			Kind string `json:"kind"`
		}
		if err := strict(data, &v); err != nil {
			return TestRef{}, err
		}
		return TestRef{Kind: v.Kind}, nil
	default:
		return TestRef{}, fmt.Errorf("unknown test ref kind %q", k.Kind)
	}
}
func decodeLiteral(data []byte) (Literal, error) {
	var k rawKind
	if err := json.Unmarshal(data, &k); err != nil {
		return Literal{}, err
	}
	switch k.Kind {
	case "integer":
		var v struct {
			Kind  string `json:"kind"`
			Value int    `json:"value"`
		}
		if err := strict(data, &v); err != nil {
			return Literal{}, err
		}
		return Literal{Kind: v.Kind, Integer: v.Value}, nil
	case "boolean":
		var v struct {
			Kind  string `json:"kind"`
			Value bool   `json:"value"`
		}
		if err := strict(data, &v); err != nil {
			return Literal{}, err
		}
		return Literal{Kind: v.Kind, Boolean: v.Value}, nil
	case "zone":
		var v struct {
			Kind  string `json:"kind"`
			Value string `json:"value"`
		}
		if err := strict(data, &v); err != nil {
			return Literal{}, err
		}
		if !validZone(v.Value) {
			return Literal{}, fmt.Errorf("malformed zone literal")
		}
		return Literal{Kind: v.Kind, Value: v.Value}, nil
	case "stats":
		var v struct {
			Kind   string `json:"kind"`
			Attack int    `json:"attack"`
			Life   int    `json:"life"`
		}
		if err := strict(data, &v); err != nil {
			return Literal{}, err
		}
		return Literal{Kind: v.Kind, Attack: v.Attack, Life: v.Life}, nil
	case "pp":
		var v struct {
			Kind    string `json:"kind"`
			Current int    `json:"current"`
			Maximum int    `json:"maximum"`
		}
		if err := strict(data, &v); err != nil {
			return Literal{}, err
		}
		return Literal{Kind: v.Kind, Current: v.Current, Maximum: v.Maximum}, nil
	default:
		return Literal{}, fmt.Errorf("unknown literal kind %q", k.Kind)
	}
}
func decodeMatcher(data []byte) (EventMatcher, error) {
	var k rawKind
	if err := json.Unmarshal(data, &k); err != nil {
		return EventMatcher{}, err
	}
	switch k.Kind {
	case "healed", "damaged":
		var v struct {
			Kind   string          `json:"kind"`
			Target json.RawMessage `json:"target"`
			Actual *int            `json:"actual"`
		}
		if err := strict(data, &v); err != nil {
			return EventMatcher{}, err
		}
		if v.Actual == nil {
			return EventMatcher{}, fmt.Errorf("missing event amount")
		}
		t, err := decodeEventTarget(v.Target)
		return EventMatcher{Kind: v.Kind, Target: &t, Actual: *v.Actual}, err
	case "destroyed", "banished":
		var v struct {
			Kind    string          `json:"kind"`
			Subject json.RawMessage `json:"subject"`
		}
		if err := strict(data, &v); err != nil {
			return EventMatcher{}, err
		}
		t, err := decodeEventTarget(v.Subject)
		return EventMatcher{Kind: v.Kind, Subject: &t}, err
	case "card_drawn":
		var v struct {
			Kind  string `json:"kind"`
			Side  string `json:"side"`
			Count *int   `json:"count"`
		}
		if err := strict(data, &v); err != nil {
			return EventMatcher{}, err
		}
		if !validSide(v.Side) || v.Count == nil || *v.Count < 0 {
			return EventMatcher{}, fmt.Errorf("malformed draw matcher")
		}
		return EventMatcher{Kind: v.Kind, Side: v.Side, Count: *v.Count}, nil
	case "follower_summoned":
		var v struct {
			Kind       string `json:"kind"`
			Side       string `json:"side,omitempty"`
			InstanceID string `json:"instanceId,omitempty"`
			CardID     int    `json:"cardId,omitempty"`
			Count      *int   `json:"count,omitempty"`
		}
		if err := strict(data, &v); err != nil {
			return EventMatcher{}, err
		}
		byInstance := v.InstanceID != "" && nodeIDPattern.MatchString(v.InstanceID) && v.CardID == 0 && v.Count == nil
		byCard := v.InstanceID == "" && validCardID(v.CardID) && v.Count != nil && *v.Count >= 0
		if v.Side != "" && !validSide(v.Side) || !byInstance && !byCard {
			return EventMatcher{}, fmt.Errorf("malformed summon matcher")
		}
		count := 0
		if v.Count != nil {
			count = *v.Count
		}
		return EventMatcher{Kind: v.Kind, Side: v.Side, InstanceID: v.InstanceID, CardID: v.CardID, Count: count}, nil
	case "zone_moved":
		var v struct {
			Kind        string          `json:"kind"`
			InstanceID  string          `json:"instanceId,omitempty"`
			From        string          `json:"from,omitempty"`
			To          string          `json:"to,omitempty"`
			Reason      string          `json:"reason,omitempty"`
			Subject     json.RawMessage `json:"subject,omitempty"`
			Destination string          `json:"destination,omitempty"`
		}
		if err := strict(data, &v); err != nil {
			return EventMatcher{}, err
		}
		m := EventMatcher{Kind: v.Kind, InstanceID: v.InstanceID, From: v.From, To: v.To, Reason: v.Reason, Destination: v.Destination}
		if v.Reason == "return" {
			if len(v.Subject) == 0 || !oneOf(v.Destination, "hand", "deck") || v.InstanceID != "" || v.From != "" || v.To != "" {
				return EventMatcher{}, fmt.Errorf("malformed return matcher")
			}
			s, err := decodeEventTarget(v.Subject)
			m.Subject = &s
			return m, err
		}
		if !nodeIDPattern.MatchString(v.InstanceID) || !validZone(v.To) || v.From != "" && !validZone(v.From) || len(v.Subject) != 0 || v.Destination != "" {
			return EventMatcher{}, fmt.Errorf("malformed move matcher")
		}
		return m, nil
	case "evolved", "super_evolved", "amulet_engaged":
		var v struct {
			Kind       string `json:"kind"`
			InstanceID string `json:"instanceId"`
		}
		if err := strict(data, &v); err != nil {
			return EventMatcher{}, err
		}
		if !nodeIDPattern.MatchString(v.InstanceID) {
			return EventMatcher{}, fmt.Errorf("malformed event instance")
		}
		return EventMatcher{Kind: v.Kind, InstanceID: v.InstanceID}, nil
	case "attacked":
		var v struct {
			Kind     string          `json:"kind"`
			Attacker json.RawMessage `json:"attacker"`
			Defender json.RawMessage `json:"defender"`
		}
		if err := strict(data, &v); err != nil {
			return EventMatcher{}, err
		}
		a, err := decodeEventTarget(v.Attacker)
		if err != nil {
			return EventMatcher{}, err
		}
		d, err := decodeEventTarget(v.Defender)
		return EventMatcher{Kind: v.Kind, Attacker: &a, Defender: &d}, err
	case "turn_started", "turn_ended":
		var v struct {
			Kind string `json:"kind"`
			Side string `json:"side"`
		}
		if err := strict(data, &v); err != nil {
			return EventMatcher{}, err
		}
		if !validSide(v.Side) {
			return EventMatcher{}, fmt.Errorf("malformed turn matcher")
		}
		return EventMatcher{Kind: v.Kind, Side: v.Side}, nil
	case "resource_changed":
		var v struct {
			Kind      string `json:"kind"`
			Side      string `json:"side"`
			Resource  string `json:"resource"`
			Direction string `json:"direction"`
			Amount    *int   `json:"amount"`
		}
		if err := strict(data, &v); err != nil {
			return EventMatcher{}, err
		}
		if !validSide(v.Side) || !oneOf(v.Resource, "life", "pp", "maxpp", "ep", "sep", "combo", "shadows") || !oneOf(v.Direction, "gain", "spend") || v.Amount == nil || *v.Amount < 0 {
			return EventMatcher{}, fmt.Errorf("malformed resource matcher")
		}
		return EventMatcher{Kind: v.Kind, Side: v.Side, Resource: v.Resource, Direction: v.Direction, Amount: *v.Amount}, nil
	default:
		return EventMatcher{}, fmt.Errorf("unknown event matcher kind %q", k.Kind)
	}
}
func decodeEventTarget(data []byte) (EventTarget, error) {
	var k rawKind
	if err := json.Unmarshal(data, &k); err != nil {
		return EventTarget{}, err
	}
	switch k.Kind {
	case "leader":
		var v struct {
			Kind string `json:"kind"`
			Side string `json:"side"`
		}
		if err := strict(data, &v); err != nil {
			return EventTarget{}, err
		}
		if !validSide(v.Side) {
			return EventTarget{}, fmt.Errorf("malformed leader target")
		}
		return EventTarget{Kind: v.Kind, Side: v.Side}, nil
	case "instance":
		var v struct {
			Kind       string `json:"kind"`
			InstanceID string `json:"instanceId"`
		}
		if err := strict(data, &v); err != nil {
			return EventTarget{}, err
		}
		if !nodeIDPattern.MatchString(v.InstanceID) {
			return EventTarget{}, fmt.Errorf("malformed instance target")
		}
		return EventTarget{Kind: v.Kind, InstanceID: v.InstanceID}, nil
	case "card":
		var v struct {
			Kind   string `json:"kind"`
			CardID int    `json:"cardId"`
		}
		if err := strict(data, &v); err != nil {
			return EventTarget{}, err
		}
		if !validCardID(v.CardID) {
			return EventTarget{}, fmt.Errorf("malformed card target")
		}
		return EventTarget{Kind: v.Kind, CardID: v.CardID}, nil
	default:
		return EventTarget{}, fmt.Errorf("unknown event target kind %q", k.Kind)
	}
}

func validateScenarioReferences(s Scenario) error {
	instances := map[string]bool{}
	for _, p := range s.InitialState.Players {
		for _, zone := range p.Zones {
			for _, i := range zone {
				instances[i.InstanceID] = true
			}
		}
	}
	if len(s.Actions) == 0 || oneOf(ActionKind(s.Actions[0]), "select", "select_mode") {
		return fmt.Errorf("scenario must start with one primary action")
	}
	for n, a := range s.Actions {
		if n > 0 && !oneOf(ActionKind(a), "select", "select_mode") {
			return fmt.Errorf("primary action may not follow a response")
		}
		switch x := a.(type) {
		case SourceAction:
			if x.Source != "" && !instances[x.Source] {
				return fmt.Errorf("action references unknown instance %s", x.Source)
			}
		case SelectAction:
			if !instances[x.Target] {
				return fmt.Errorf("selection references unknown instance %s", x.Target)
			}
		case AttackAction:
			if !instances[x.Attacker] || x.Kind == "attack_entity" && !instances[x.Defender] {
				return fmt.Errorf("attack references unknown instance")
			}
		}
	}
	checkTarget := func(t *EventTarget) bool { return t == nil || t.Kind != "instance" || instances[t.InstanceID] }
	for _, a := range s.Assertions {
		switch x := a.(type) {
		case CompareAssertion:
			if x.Left.Kind == "instance_field" && !instances[x.Left.InstanceID] {
				return fmt.Errorf("assertion references unknown instance %s", x.Left.InstanceID)
			}
		case KeywordAssertion:
			if x.Target != "" && !instances[x.Target] {
				return fmt.Errorf("assertion references unknown instance %s", x.Target)
			}
		case ZoneAssertion:
			for _, id := range x.Expected {
				if !instances[id] {
					return fmt.Errorf("assertion references unknown instance %s", id)
				}
			}
		case EventsAssertion:
			for _, m := range x.Expected {
				if m.InstanceID != "" && !instances[m.InstanceID] || !checkTarget(m.Target) || !checkTarget(m.Subject) || !checkTarget(m.Attacker) || !checkTarget(m.Defender) {
					return fmt.Errorf("event matcher references unknown instance")
				}
			}
		}
	}
	return nil
}

func ValidateRuntimePacks(cards *CardPack, tests *TestPack) error {
	known := map[int]Card{}
	for _, c := range cards.Cards {
		known[c.ID] = c
	}
	for _, s := range tests.Scenarios {
		for _, p := range s.InitialState.Players {
			for _, zone := range p.Zones {
				for _, i := range zone {
					c, ok := known[i.CardID]
					if !ok {
						return fmt.Errorf("scenario %q references unknown card %d", s.Name, i.CardID)
					}
					if c.CardType != i.DeclaredType {
						return fmt.Errorf("scenario %q card %d type mismatch", s.Name, i.CardID)
					}
				}
			}
		}
		for _, a := range s.Assertions {
			if z, ok := a.(ZoneAssertion); ok && z.Kind == "zone_count" {
				if _, exists := known[z.CardID]; !exists {
					return fmt.Errorf("scenario %q zone_count references unknown card %d", s.Name, z.CardID)
				}
			}
		}
	}
	return nil
}

func validSide(v string) bool { return oneOf(v, "own", "oppo") }
func validZone(v string) bool {
	return oneOf(v, "deck", "hand", "field", "graveyard", "banished", "destroyed")
}
