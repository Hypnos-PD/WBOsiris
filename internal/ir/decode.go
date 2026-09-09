package ir

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"unicode/utf8"
)

const MaxPackageSize = 16 << 20

var (
	nodeIDPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)
	hashPattern   = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	seedPattern   = regexp.MustCompile(`^0x[0-9a-f]{16}$`)
)

func strict(data []byte, dst any) error {
	if len(data) > MaxPackageSize {
		return fmt.Errorf("WBOS JSON exceeds %d bytes", MaxPackageSize)
	}
	if !utf8.Valid(data) {
		return fmt.Errorf("WBOS JSON is not valid UTF-8")
	}
	if err := validateJSONObjectKeys(data); err != nil {
		return err
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(dst); err != nil {
		return err
	}
	if err := d.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("trailing JSON")
	}
	return nil
}

func validateJSONObjectKeys(data []byte) error {
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	var value func() error
	value = func() error {
		token, err := d.Token()
		if err != nil {
			return err
		}
		delim, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		switch delim {
		case '{':
			seen := map[string]bool{}
			for d.More() {
				keyToken, err := d.Token()
				if err != nil {
					return err
				}
				key, ok := keyToken.(string)
				if !ok {
					return fmt.Errorf("object key is not a string")
				}
				if seen[key] {
					return fmt.Errorf("duplicate object key %q", key)
				}
				seen[key] = true
				if err := value(); err != nil {
					return err
				}
			}
			end, err := d.Token()
			if err != nil {
				return err
			}
			if end != json.Delim('}') {
				return fmt.Errorf("malformed object")
			}
		case '[':
			for d.More() {
				if err := value(); err != nil {
					return err
				}
			}
			end, err := d.Token()
			if err != nil {
				return err
			}
			if end != json.Delim(']') {
				return fmt.Errorf("malformed array")
			}
		default:
			return fmt.Errorf("unexpected JSON delimiter %q", delim)
		}
		return nil
	}
	if err := value(); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		if err == nil {
			return fmt.Errorf("trailing JSON")
		}
		return fmt.Errorf("trailing JSON: %w", err)
	}
	return nil
}

func DecodeCardPack(data []byte) (*CardPack, error) {
	type rawPack struct {
		Format                string            `json:"format"`
		ContainerVersion      string            `json:"containerVersion"`
		Encoding              string            `json:"encoding"`
		Kind                  string            `json:"kind"`
		IRVersion             string            `json:"irVersion"`
		SourceLanguageVersion string            `json:"sourceLanguageVersion"`
		RequiredFeatures      []string          `json:"requiredFeatures"`
		UnresolvedReferences  []string          `json:"unresolvedReferences,omitempty"`
		Sources               []json.RawMessage `json:"sources"`
		Cards                 []json.RawMessage `json:"cards"`
		ContentHash           string            `json:"contentHash"`
	}
	var raw rawPack
	if err := strict(data, &raw); err != nil {
		return nil, fmt.Errorf("decode card pack: %w", err)
	}
	if err := validateHeader(raw.Format, raw.ContainerVersion, raw.Encoding, raw.Kind, raw.IRVersion, "card-pack", raw.RequiredFeatures, raw.UnresolvedReferences); err != nil {
		return nil, err
	}
	if raw.SourceLanguageVersion != SourceLanguageVersion {
		return nil, fmt.Errorf("incompatible source language version %q", raw.SourceLanguageVersion)
	}
	if err := verifyHash(data, raw.ContentHash); err != nil {
		return nil, err
	}
	p := &CardPack{Format: raw.Format, ContainerVersion: raw.ContainerVersion, Encoding: raw.Encoding, Kind: raw.Kind, IRVersion: raw.IRVersion, SourceLanguageVersion: raw.SourceLanguageVersion, RequiredFeatures: raw.RequiredFeatures, UnresolvedReferences: raw.UnresolvedReferences, ContentHash: raw.ContentHash}
	var err error
	p.Sources, err = decodeSources(raw.Sources)
	if err != nil {
		return nil, err
	}
	seenCards, seenAbilities, seenNodes := map[int]bool{}, map[string]bool{}, map[string]bool{}
	for _, item := range raw.Cards {
		c, err := decodeCard(item, seenAbilities, seenNodes)
		if err != nil {
			return nil, err
		}
		if seenCards[c.ID] {
			return nil, fmt.Errorf("duplicate card ID %d", c.ID)
		}
		seenCards[c.ID] = true
		p.Cards = append(p.Cards, c)
	}
	crests := map[int]bool{}
	for _, c := range p.Cards {
		crests[c.ID] = c.Crest != nil
	}
	for _, c := range p.Cards {
		if err := validateCardRefs(c, seenCards, crests); err != nil {
			return nil, err
		}
	}
	return p, nil
}

func DecodeTestPack(data []byte) (*TestPack, error) {
	type rawPack struct {
		Format               string            `json:"format"`
		ContainerVersion     string            `json:"containerVersion"`
		Encoding             string            `json:"encoding"`
		Kind                 string            `json:"kind"`
		IRVersion            string            `json:"irVersion"`
		RequiredFeatures     []string          `json:"requiredFeatures"`
		UnresolvedReferences []string          `json:"unresolvedReferences,omitempty"`
		Ruleset              json.RawMessage   `json:"ruleset"`
		Sources              []json.RawMessage `json:"sources"`
		Scenarios            []json.RawMessage `json:"scenarios"`
		ContentHash          string            `json:"contentHash"`
	}
	var raw rawPack
	if err := strict(data, &raw); err != nil {
		return nil, fmt.Errorf("decode test pack: %w", err)
	}
	if err := validateHeader(raw.Format, raw.ContainerVersion, raw.Encoding, raw.Kind, raw.IRVersion, "test-pack", raw.RequiredFeatures, raw.UnresolvedReferences); err != nil {
		return nil, err
	}
	if err := verifyHash(data, raw.ContentHash); err != nil {
		return nil, err
	}
	type rng struct {
		Algorithm string `json:"algorithm"`
		Version   string `json:"version"`
	}
	type ordering struct {
		Collections          string `json:"collections"`
		ReplacementAbilities string `json:"replacementAbilities"`
		SimultaneousTriggers string `json:"simultaneousTriggers"`
		DeathBatchLastwords  string `json:"deathBatchLastwords"`
	}
	type budget struct {
		Instructions      uint64 `json:"instructions"`
		QueryVisits       uint64 `json:"queryVisits"`
		StackDepth        uint64 `json:"stackDepth"`
		Candidates        uint64 `json:"candidates"`
		Events            uint64 `json:"events"`
		Triggers          uint64 `json:"triggers"`
		CreatedInstances  uint64 `json:"createdInstances"`
		ContinuationBytes uint64 `json:"continuationBytes"`
	}
	type ruleset struct {
		ID              string   `json:"id"`
		RNG             rng      `json:"rng"`
		OrderingPolicy  ordering `json:"orderingPolicy"`
		ExecutionBudget budget   `json:"executionBudget"`
		ContentHash     string   `json:"contentHash"`
	}
	var r ruleset
	if err := strict(raw.Ruleset, &r); err != nil {
		return nil, fmt.Errorf("ruleset: %w", err)
	}
	if r.ID == "" || r.RNG.Algorithm == "" || r.RNG.Version == "" || !hashPattern.MatchString(r.ContentHash) ||
		r.ExecutionBudget.Instructions == 0 || r.ExecutionBudget.QueryVisits == 0 || r.ExecutionBudget.StackDepth == 0 ||
		r.ExecutionBudget.Candidates == 0 || r.ExecutionBudget.Events == 0 || r.ExecutionBudget.Triggers == 0 ||
		r.ExecutionBudget.CreatedInstances == 0 || r.ExecutionBudget.ContinuationBytes == 0 {
		return nil, fmt.Errorf("malformed ruleset dependency")
	}
	p := &TestPack{Format: raw.Format, ContainerVersion: raw.ContainerVersion, Encoding: raw.Encoding, Kind: raw.Kind, IRVersion: raw.IRVersion, RequiredFeatures: raw.RequiredFeatures, UnresolvedReferences: raw.UnresolvedReferences, Ruleset: RulesetDependency{
		ID: r.ID, ContentHash: r.ContentHash, RNG: RNGPolicy{r.RNG.Algorithm, r.RNG.Version},
		OrderingPolicy: OrderingPolicy{r.OrderingPolicy.Collections, r.OrderingPolicy.ReplacementAbilities, r.OrderingPolicy.SimultaneousTriggers, r.OrderingPolicy.DeathBatchLastwords},
		ExecutionBudget: ExecutionBudgetPolicy{
			Instructions: r.ExecutionBudget.Instructions, QueryVisits: r.ExecutionBudget.QueryVisits,
			StackDepth: r.ExecutionBudget.StackDepth, Candidates: r.ExecutionBudget.Candidates,
			Events: r.ExecutionBudget.Events, Triggers: r.ExecutionBudget.Triggers,
			CreatedInstances: r.ExecutionBudget.CreatedInstances, ContinuationBytes: r.ExecutionBudget.ContinuationBytes,
		},
	}, ContentHash: raw.ContentHash}
	var err error
	p.Sources, err = decodeSources(raw.Sources)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, item := range raw.Scenarios {
		s, err := decodeScenario(item)
		if err != nil {
			return nil, err
		}
		if seen[s.ID] {
			return nil, fmt.Errorf("duplicate scenario ID %s", s.ID)
		}
		seen[s.ID] = true
		p.Scenarios = append(p.Scenarios, s)
	}
	return p, nil
}

func validateHeader(format, container, encoding, kind, version, wantKind string, features, unresolved []string) error {
	if format != Format || container != ContainerVersion || encoding != Encoding || version != IRVersion || kind != wantKind {
		return fmt.Errorf("incompatible WBOS container or IR version")
	}
	if len(features) != 0 {
		return fmt.Errorf("unsupported requiredFeatures %v", features)
	}
	if len(unresolved) != 0 {
		return fmt.Errorf("unresolved executable references %v", unresolved)
	}
	return nil
}

func verifyHash(data []byte, got string) error {
	if !hashPattern.MatchString(got) {
		return fmt.Errorf("malformed content hash")
	}
	var object map[string]any
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	if err := d.Decode(&object); err != nil {
		return err
	}
	object["contentHash"] = ""
	canonical, err := json.Marshal(object)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(canonical)
	want := "sha256:" + hex.EncodeToString(sum[:])
	if got != want {
		return fmt.Errorf("content hash mismatch: got %s, want %s", got, want)
	}
	return nil
}

func decodeSources(items []json.RawMessage) ([]Source, error) {
	type rawSource struct {
		SourceID    string     `json:"sourceId"`
		Path        string     `json:"path"`
		ContentHash string     `json:"contentHash"`
		Span        SourceSpan `json:"span"`
	}
	out := make([]Source, 0, len(items))
	seen := map[string]bool{}
	for _, item := range items {
		var v rawSource
		if err := strict(item, &v); err != nil {
			return nil, fmt.Errorf("source: %w", err)
		}
		if !nodeIDPattern.MatchString(v.SourceID) || !hashPattern.MatchString(v.ContentHash) || v.Span.SourceID != v.SourceID {
			return nil, fmt.Errorf("malformed source")
		}
		if seen[v.SourceID] {
			return nil, fmt.Errorf("duplicate source ID")
		}
		seen[v.SourceID] = true
		out = append(out, Source{v.SourceID, v.Path, v.ContentHash, v.Span})
	}
	return out, nil
}

func decodeCard(data []byte, abilityIDs, nodeIDs map[string]bool) (Card, error) {
	type rawCard struct {
		Crest           json.RawMessage   `json:"crest"`
		Counters        map[string]int    `json:"counters,omitempty"`
		ID              int               `json:"id"`
		CardType        string            `json:"cardType"`
		Cost            int               `json:"cost"`
		Stats           *Stats            `json:"stats,omitempty"`
		Traits          []string          `json:"traits"`
		Intrinsic       []string          `json:"intrinsic"`
		IntrinsicState  []json.RawMessage `json:"intrinsicState"`
		Restrictions    []json.RawMessage `json:"restrictions"`
		Abilities       []json.RawMessage `json:"abilities"`
		FusionAbilities []json.RawMessage `json:"fusionAbilities"`
		PlayEffects     []json.RawMessage `json:"playEffects"`
		ActionPlans     []ActionPlanRaw   `json:"actionPlans"`
		Meta            Meta              `json:"meta"`
		Locales         map[string]Locale `json:"locales"`
		Origin          Origin            `json:"origin"`
	}
	var raw rawCard
	if err := strict(data, &raw); err != nil {
		return Card{}, fmt.Errorf("card: %w", err)
	}
	if !validCardID(raw.ID) || !oneOf(raw.CardType, "follower", "spell", "amulet") || raw.Cost < 0 || raw.Cost > 65535 {
		return Card{}, fmt.Errorf("malformed card ID/type/cost")
	}
	if raw.CardType == "follower" && raw.Stats == nil || raw.CardType != "follower" && raw.Stats != nil || raw.Meta.Pack <= 0 || !oneOf(raw.Meta.Class, "neutral", "forestcraft", "swordcraft", "runecraft", "dragoncraft", "abysscraft", "havencraft", "portalcraft") || !oneOf(raw.Meta.Rarity, "bronze", "silver", "gold", "legendary") {
		return Card{}, fmt.Errorf("malformed card shape")
	}
	for _, k := range raw.Intrinsic {
		if !validKeyword(k) {
			return Card{}, fmt.Errorf("invalid intrinsic keyword %q", k)
		}
	}
	for _, trait := range raw.Traits {
		if !ValidTrait(trait) {
			return Card{}, fmt.Errorf("invalid trait %q", trait)
		}
	}
	if !validOrigin(raw.Origin) {
		return Card{}, fmt.Errorf("malformed card origin")
	}
	c := Card{ID: raw.ID, CardType: raw.CardType, Cost: raw.Cost, Stats: raw.Stats, Traits: raw.Traits, Intrinsic: raw.Intrinsic, Meta: raw.Meta, Locales: raw.Locales, Origin: raw.Origin}
	c.Counters = raw.Counters
	for _, x := range raw.IntrinsicState {
		type v struct {
			Kind    string `json:"kind"`
			Initial int    `json:"initial"`
		}
		var s v
		if err := strict(x, &s); err != nil || !oneOf(s.Kind, "countdown", "earthsigil", "damage_reduction", "attack_limit") || s.Initial < 0 || s.Kind == "earthsigil" && s.Initial != 1 || s.Kind == "attack_limit" && s.Initial < 1 {
			return Card{}, fmt.Errorf("intrinsic state: %w", err)
		}
		c.IntrinsicState = append(c.IntrinsicState, IntrinsicState{s.Kind, s.Initial})
	}
	for _, x := range raw.Restrictions {
		var r Restriction
		if err := strict(x, &r); err != nil || r.Kind != "unplayable" {
			return Card{}, fmt.Errorf("restriction: %w", err)
		}
		c.Restrictions = append(c.Restrictions, r)
	}
	for _, x := range raw.Abilities {
		a, err := decodeAbility(x, nodeIDs)
		if err != nil {
			return Card{}, err
		}
		if abilityIDs[a.ID] {
			return Card{}, fmt.Errorf("duplicate ability ID %s", a.ID)
		}
		abilityIDs[a.ID] = true
		c.Abilities = append(c.Abilities, a)
	}
	for _, x := range raw.FusionAbilities {
		a, err := decodeFusion(x, nodeIDs)
		if err != nil {
			return Card{}, err
		}
		if abilityIDs[a.ID] {
			return Card{}, fmt.Errorf("duplicate ability ID %s", a.ID)
		}
		abilityIDs[a.ID] = true
		c.FusionAbilities = append(c.FusionAbilities, a)
	}
	var err error
	c.PlayEffects, err = decodeEffects(raw.PlayEffects, nodeIDs)
	if err != nil {
		return Card{}, err
	}
	localAbilities := map[string]bool{}
	for _, a := range c.Abilities {
		localAbilities[a.ID] = true
	}
	for _, p := range raw.ActionPlans {
		plan := ActionPlan{Action: p.Action}
		if !oneOf(p.Action, "evolve", "superevolve") {
			return Card{}, fmt.Errorf("bad action plan")
		}
		if len(p.Steps) == 0 {
			return Card{}, fmt.Errorf("empty action plan")
		}
		for _, s := range p.Steps {
			if !nodeIDPattern.MatchString(s.AbilityID) || !localAbilities[s.AbilityID] || !oneOf(s.Frame, "new", "continue") {
				return Card{}, fmt.Errorf("bad plan ability ref %q", s.AbilityID)
			}
			plan.Steps = append(plan.Steps, PlanStep{s.AbilityID, s.Frame})
		}
		if plan.Steps[0].Frame == "continue" {
			return Card{}, fmt.Errorf("action plan starts with continue frame")
		}
		c.ActionPlans = append(c.ActionPlans, plan)
	}
	if len(raw.Crest) > 0 {
		c.Crest, err = decodeCrest(raw.Crest, abilityIDs, nodeIDs)
		if err != nil {
			return Card{}, err
		}
		if err = validateCounterRefs(*c.CrestCard()); err != nil {
			return Card{}, err
		}
	}
	return c, validateCounterRefs(c)
}

type ActionPlanRaw struct {
	Action string `json:"action"`
	Steps  []struct {
		AbilityID string `json:"abilityId"`
		Frame     string `json:"frame"`
	} `json:"steps"`
}

func decodeAbility(data []byte, nodeIDs map[string]bool) (Ability, error) {
	type raw struct {
		ID       string            `json:"id"`
		Trigger  json.RawMessage   `json:"trigger"`
		Relation string            `json:"relation"`
		Body     []json.RawMessage `json:"body"`
		Origin   Origin            `json:"origin"`
	}
	var v raw
	if err := strict(data, &v); err != nil {
		return Ability{}, fmt.Errorf("ability: %w", err)
	}
	if err := newNode(v.ID, nodeIDs, v.Origin); err != nil {
		return Ability{}, err
	}
	if !oneOf(v.Relation, "independent", "replaces", "extends") {
		return Ability{}, fmt.Errorf("invalid ability relation %q", v.Relation)
	}
	t, err := decodeTrigger(v.Trigger)
	if err != nil {
		return Ability{}, err
	}
	body, err := decodeEffects(v.Body, nodeIDs)
	return Ability{v.ID, t, v.Relation, body, v.Origin}, err
}
func decodeFusion(data []byte, nodeIDs map[string]bool) (FusionAbility, error) {
	type raw struct {
		ID             string            `json:"id"`
		MaterialFilter json.RawMessage   `json:"materialFilter"`
		Body           []json.RawMessage `json:"body"`
		Origin         Origin            `json:"origin"`
	}
	var v raw
	if err := strict(data, &v); err != nil {
		return FusionAbility{}, err
	}
	if err := newNode(v.ID, nodeIDs, v.Origin); err != nil {
		return FusionAbility{}, err
	}
	m, err := decodeMaterial(v.MaterialFilter)
	if err != nil {
		return FusionAbility{}, err
	}
	body, err := decodeEffects(v.Body, nodeIDs)
	return FusionAbility{v.ID, m, body, v.Origin}, err
}
func decodeMaterial(data []byte) (MaterialFilter, error) {
	type raw struct {
		Kind          string          `json:"kind"`
		Source        json.RawMessage `json:"source"`
		Predicate     json.RawMessage `json:"predicate"`
		ExcludeSource bool            `json:"excludeSource"`
		Minimum       int             `json:"minimum"`
	}
	var v raw
	if err := strict(data, &v); err != nil {
		return MaterialFilter{}, err
	}
	if v.Kind != "material_filter" || !v.ExcludeSource || v.Minimum != 1 {
		return MaterialFilter{}, fmt.Errorf("bad material filter")
	}
	if len(v.Predicate) == 0 {
		return MaterialFilter{}, fmt.Errorf("material filter predicate must be explicit null or an object")
	}
	s, err := decodeRef(v.Source)
	if err != nil {
		return MaterialFilter{}, err
	}
	z, ok := s.(ZoneRef)
	if !ok || z.Side != "own" || z.Zone != "hand" || z.Member != "card" {
		return MaterialFilter{}, fmt.Errorf("invalid material source")
	}
	var p Predicate
	if len(v.Predicate) > 0 && !bytes.Equal(v.Predicate, []byte("null")) {
		p, err = decodePredicate(v.Predicate)
		if err != nil {
			return MaterialFilter{}, err
		}
	}
	return MaterialFilter{v.Kind, s, p, v.ExcludeSource, v.Minimum}, nil
}

func decodeTrigger(data []byte) (Trigger, error) {
	var k rawKind
	if err := json.Unmarshal(data, &k); err != nil {
		return nil, err
	}
	switch k.Kind {
	case "fanfare", "lastwords", "attack", "clash", "evolve", "superevolve", "spellboost":
		type raw struct {
			Kind string `json:"kind"`
		}
		var v raw
		if err := strict(data, &v); err != nil {
			return nil, err
		}
		return SimpleTrigger{v.Kind}, nil
	case "engage", "enhance":
		type raw struct {
			Kind string `json:"kind"`
			Cost int    `json:"cost"`
		}
		var v raw
		if err := strict(data, &v); err != nil {
			return nil, err
		}
		if v.Cost < 0 || v.Cost > 65535 {
			return nil, fmt.Errorf("invalid trigger cost")
		}
		return CostTrigger{v.Kind, v.Cost}, nil
	case "event":
		type raw struct {
			Kind        string          `json:"kind"`
			Event       string          `json:"event"`
			Side        string          `json:"side"`
			SubjectType string          `json:"subjectType,omitempty"`
			SourceZone  string          `json:"sourceZone,omitempty"`
			OncePerTurn string          `json:"oncePerTurn,omitempty"`
			SelfOnly    bool            `json:"selfOnly,omitempty"`
			Predicate   json.RawMessage `json:"predicate,omitempty"`
			Condition   json.RawMessage `json:"condition,omitempty"`
		}
		var v raw
		if err := strict(data, &v); err != nil {
			return nil, err
		}
		var p Predicate
		var err error
		if len(v.Predicate) > 0 {
			p, err = decodePredicate(v.Predicate)
		}
		if err != nil {
			return nil, err
		}
		var condition Condition
		if len(v.Condition) > 0 {
			condition, err = decodeCondition(v.Condition)
			if err != nil {
				return nil, err
			}
			if c, ok := condition.(CompareCondition); ok && c.Left.Kind == "fusion_material_scalar" {
				return nil, fmt.Errorf("event conditions cannot access fusion materials")
			}
		}
		if !validSide(v.Side) || !oneOf(v.Event, "follower_summoned", "follower_left", "destroyed", "healed", "card_fused", "amulet_engaged", "card_discarded", "turn_started", "turn_ended", "evolved", "super_evolved") || v.SubjectType != "" && !oneOf(v.SubjectType, "follower", "amulet", "leader") || v.SourceZone != "" && !oneOf(v.SourceZone, "hand", "field") || v.Event == "destroyed" && v.SubjectType == "" {
			return nil, fmt.Errorf("invalid event trigger")
		}
		if v.Event == "healed" && (v.SubjectType != "leader" || p != nil) || v.SubjectType == "leader" && v.Event != "healed" {
			return nil, fmt.Errorf("healed listeners require a leader without a card predicate")
		}
		if v.OncePerTurn != "" && !oneOf(v.OncePerTurn, "any", "own", "oppo") {
			return nil, fmt.Errorf("invalid trigger turn limit")
		}
		if v.SelfOnly && (condition != nil || v.OncePerTurn != "" || v.SourceZone != "" || v.Side != "own" || !(v.SubjectType == "follower" && oneOf(v.Event, "evolved", "super_evolved", "follower_summoned") || v.SubjectType == "" && v.Event == "card_discarded") || p != nil) {
			return nil, fmt.Errorf("invalid self event trigger")
		}
		return EventTrigger{Kind: v.Kind, Event: v.Event, Side: v.Side, SourceZone: v.SourceZone, SubjectType: v.SubjectType, SelfOnly: v.SelfOnly, Predicate: p, OncePerTurn: v.OncePerTurn, Condition: condition}, nil
	case "replacement":
		type raw struct {
			Kind    string          `json:"kind"`
			Subject json.RawMessage `json:"subject"`
			From    string          `json:"from"`
			Phase   string          `json:"phase"`
		}
		var v raw
		if err := strict(data, &v); err != nil {
			return nil, err
		}
		s, err := decodeRef(v.Subject)
		if v.From != "field" || v.Phase != "before" {
			return nil, fmt.Errorf("invalid replacement trigger")
		}
		return ReplacementTrigger{v.Kind, s, v.From, v.Phase}, err
	default:
		return nil, fmt.Errorf("unknown executable trigger kind %q", k.Kind)
	}
}

func decodeEffects(raw []json.RawMessage, nodeIDs map[string]bool) ([]Effect, error) {
	out := make([]Effect, 0, len(raw))
	for _, x := range raw {
		e, err := decodeEffect(x, nodeIDs)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, nil
}
func decodeEffect(data []byte, nodeIDs map[string]bool) (Effect, error) {
	effect, err := decodeEffectShape(data, nodeIDs)
	if err == nil && hasHistoryEffectTarget(effect) {
		return nil, fmt.Errorf("destroyed history is read-only")
	}
	return effect, err
}

func decodeEffectShape(data []byte, nodeIDs map[string]bool) (Effect, error) {
	var k rawKind
	if err := json.Unmarshal(data, &k); err != nil {
		return nil, err
	}
	switch k.Kind {
	case "choose", "require", "random_choose":
		type raw struct {
			ID       string             `json:"id"`
			Kind     string             `json:"kind"`
			Policy   string             `json:"policy"`
			Binding  string             `json:"binding"`
			Source   json.RawMessage    `json:"source"`
			Count    json.RawMessage    `json:"count"`
			Extremum *SelectionExtremum `json:"extremum,omitempty"`
			Origin   Origin             `json:"origin"`
		}
		var v raw
		if err := strict(data, &v); err != nil {
			return nil, err
		}
		if err := newNode(v.ID, nodeIDs, v.Origin); err != nil {
			return nil, err
		}
		s, err := decodeSelectionSource(v.Source)
		wantPolicy := map[string]string{"choose": "optional", "require": "required", "random_choose": "random"}[v.Kind]
		if v.Policy != wantPolicy || v.Binding == "" {
			return nil, fmt.Errorf("invalid selection effect")
		}
		if !ValidSelectionExtremum(v.Extremum) {
			return nil, fmt.Errorf("invalid selection extremum")
		}
		if _, mixed := s.(CharacterSetRef); mixed && v.Extremum != nil {
			return nil, fmt.Errorf("character sets do not have card extrema")
		}
		count := 0
		if len(v.Count) != 0 {
			if err := json.Unmarshal(v.Count, &count); err != nil || count < 1 || count > 65535 {
				return nil, fmt.Errorf("invalid selection count")
			}
		}
		return SelectionEffect{NodeBase: NodeBase{v.ID, v.Origin}, Kind: v.Kind, Policy: v.Policy, Binding: v.Binding, Source: s, Count: count, Extremum: v.Extremum}, err
	case "if":
		type raw struct {
			ID        string            `json:"id"`
			Kind      string            `json:"kind"`
			Condition json.RawMessage   `json:"condition"`
			Then      []json.RawMessage `json:"then"`
			Else      []json.RawMessage `json:"else"`
			Origin    Origin            `json:"origin"`
		}
		var v raw
		if err := strict(data, &v); err != nil {
			return nil, err
		}
		if err := newNode(v.ID, nodeIDs, v.Origin); err != nil {
			return nil, err
		}
		c, err := decodeCondition(v.Condition)
		if err != nil {
			return nil, err
		}
		a, err := decodeEffects(v.Then, nodeIDs)
		if err != nil {
			return nil, err
		}
		b, err := decodeEffects(v.Else, nodeIDs)
		return IfEffect{NodeBase{v.ID, v.Origin}, v.Kind, c, a, b}, err
	case "repeat":
		var v struct {
			ID     string            `json:"id"`
			Kind   string            `json:"kind"`
			Times  json.RawMessage   `json:"times"`
			Body   []json.RawMessage `json:"body"`
			Origin Origin            `json:"origin"`
		}
		if err := strict(data, &v); err != nil {
			return nil, err
		}
		if err := newNode(v.ID, nodeIDs, v.Origin); err != nil {
			return nil, err
		}
		times, expr, err := decodeEffectAmount(v.Times)
		if err != nil {
			return nil, err
		}
		body, err := decodeEffects(v.Body, nodeIDs)
		return RepeatEffect{NodeBase: NodeBase{v.ID, v.Origin}, Kind: v.Kind, Times: times, TimesExpr: expr, Body: body}, err
	case "mode":
		type raw struct {
			ID      string            `json:"id"`
			Kind    string            `json:"kind"`
			Options []json.RawMessage `json:"options"`
			Origin  Origin            `json:"origin"`
		}
		var v raw
		if err := strict(data, &v); err != nil {
			return nil, err
		}
		if err := newNode(v.ID, nodeIDs, v.Origin); err != nil {
			return nil, err
		}
		e := ModeEffect{NodeBase: NodeBase{v.ID, v.Origin}, Kind: v.Kind}
		seen := map[int]bool{}
		for _, x := range v.Options {
			type option struct {
				ID     int               `json:"id"`
				Body   []json.RawMessage `json:"body"`
				Origin Origin            `json:"origin"`
				Labels map[string]string `json:"labels,omitempty"`
			}
			var o option
			if err := strict(x, &o); err != nil {
				return nil, err
			}
			if seen[o.ID] || o.ID <= 0 || o.ID > 65535 || !validOrigin(o.Origin) || !ValidChoiceLabels(o.Labels) {
				return nil, fmt.Errorf("duplicate mode option")
			}
			seen[o.ID] = true
			b, err := decodeEffects(o.Body, nodeIDs)
			if err != nil {
				return nil, err
			}
			e.Options = append(e.Options, ModeOption{ID: o.ID, Body: b, Origin: o.Origin, Labels: o.Labels})
		}
		if len(e.Options) < 2 {
			return nil, fmt.Errorf("mode requires at least two options")
		}
		return e, nil
	case "pay_resource":
		type raw struct {
			ID       string            `json:"id"`
			Kind     string            `json:"kind"`
			Resource string            `json:"resource"`
			Amount   int               `json:"amount"`
			OnPaid   []json.RawMessage `json:"onPaid"`
			Origin   Origin            `json:"origin"`
		}
		var v raw
		if err := strict(data, &v); err != nil {
			return nil, err
		}
		if err := newNode(v.ID, nodeIDs, v.Origin); err != nil {
			return nil, err
		}
		if !oneOf(v.Resource, "earthsigil", "shadows") || v.Amount < 0 {
			return nil, fmt.Errorf("invalid resource payment")
		}
		b, err := decodeEffects(v.OnPaid, nodeIDs)
		return PayResourceEffect{NodeBase{v.ID, v.Origin}, v.Kind, v.Resource, v.Amount, b}, err
	case "draw":
		type raw struct {
			ID                              string `json:"id"`
			Kind, Owner, SourceZone, Output string
			Count                           *int            `json:"count,omitempty"`
			All                             bool            `json:"all"`
			Predicate                       json.RawMessage `json:"predicate,omitempty"`
			Origin                          Origin          `json:"origin"`
		}
		var v raw
		if err := strict(data, &v); err != nil {
			return nil, err
		}
		if err := newNode(v.ID, nodeIDs, v.Origin); err != nil {
			return nil, err
		}
		var p Predicate
		var err error
		if len(v.Predicate) > 0 {
			p, err = decodePredicate(v.Predicate)
		}
		if !validSide(v.Owner) || v.SourceZone != "deck" || v.Output != "drawn" || v.All && v.Count != nil || !v.All && (v.Count == nil || *v.Count < 0) {
			return nil, fmt.Errorf("invalid draw shape")
		}
		count := 0
		if v.Count != nil {
			count = *v.Count
		}
		return DrawEffect{NodeBase{v.ID, v.Origin}, v.Kind, v.Owner, v.SourceZone, v.Output, count, v.All, p}, err
	case "grant_ability":
		var v struct {
			NodeBase
			Labels          map[string]string
			Kind            string
			Target, Ability json.RawMessage
		}
		if err := strict(data, &v); err != nil {
			return nil, err
		}
		if err := newNode(v.ID, nodeIDs, v.Origin); err != nil {
			return nil, err
		}
		target, err := decodeRef(v.Target)
		if err != nil {
			return nil, err
		}
		ability, err := decodeAbility(v.Ability, nodeIDs)
		if err != nil {
			return nil, err
		}
		if !ValidGrantedTrigger(ability.Trigger) || ability.Relation != "independent" || !ValidChoiceLabels(v.Labels) || !validGrantedBody(ability.Body) {
			return nil, fmt.Errorf("invalid granted ability")
		}
		return GrantEffect{NodeBase: v.NodeBase, Kind: v.Kind, Target: target, Ability: ability, Labels: v.Labels}, nil
	case "summon_from_history":
		var v struct {
			NodeBase
			Kind     string             `json:"kind"`
			Owner    string             `json:"owner"`
			Source   json.RawMessage    `json:"source"`
			Count    int                `json:"count"`
			Extremum *SelectionExtremum `json:"extremum,omitempty"`
			Output   string             `json:"output"`
		}
		if err := strict(data, &v); err != nil {
			return nil, err
		}
		if err := newNode(v.ID, nodeIDs, v.Origin); err != nil {
			return nil, err
		}
		source, err := decodeRef(v.Source)
		if err != nil {
			return nil, err
		}
		if !validSide(v.Owner) || !ValidHistorySummonSource(source) || v.Count < 1 || v.Count > 65535 || !ValidSelectionExtremum(v.Extremum) || v.Output != "summoned" {
			return nil, fmt.Errorf("invalid summon_from_history shape")
		}
		return HistorySummonEffect{NodeBase: v.NodeBase, Kind: v.Kind, Owner: v.Owner, Source: source, Count: v.Count, Extremum: v.Extremum, Output: v.Output}, nil
	case "summon_copies":
		var v struct {
			NodeBase
			Kind, Owner, Output string
			Target              json.RawMessage
		}
		if err := strict(data, &v); err != nil {
			return nil, err
		}
		if err := newNode(v.ID, nodeIDs, v.Origin); err != nil {
			return nil, err
		}
		target, err := decodeRef(v.Target)
		if err != nil {
			return nil, err
		}
		if !validSide(v.Owner) || v.Output != "summoned" {
			return nil, fmt.Errorf("invalid summon_copies shape")
		}
		return CardEffect{NodeBase: v.NodeBase, Kind: v.Kind, Owner: v.Owner, Target: target, Output: v.Output}, nil
	case "add_card", "summon", "reanimate", "transform", "gain_crest":
		type raw struct {
			ID                                         string `json:"id"`
			Kind, Owner, Destination, Output, TieBreak string
			Count, CardID, MaxCost                     int
			Target                                     json.RawMessage `json:"target,omitempty"`
			PreserveInstanceID                         bool            `json:"preserveInstanceId,omitempty"`
			PreserveMaterials                          bool            `json:"preserveMaterials,omitempty"`
			Origin                                     Origin          `json:"origin"`
		}
		var v raw
		if err := strict(data, &v); err != nil {
			return nil, err
		}
		if err := newNode(v.ID, nodeIDs, v.Origin); err != nil {
			return nil, err
		}
		var r Ref
		var err error
		if len(v.Target) > 0 {
			r, err = decodeRef(v.Target)
		}
		switch v.Kind {
		case "gain_crest":
			if !validSide(v.Owner) || !validCardID(v.CardID) || v.Count != 0 || v.Output != "" || v.Destination != "" || v.TieBreak != "" || v.MaxCost != 0 || r != nil || v.PreserveInstanceID || v.PreserveMaterials {
				return nil, fmt.Errorf("invalid gain_crest shape")
			}
		case "add_card":
			if !validSide(v.Owner) || v.Destination != "hand" || v.Count < 0 || !validCardID(v.CardID) || v.Output != "" || v.TieBreak != "" || v.MaxCost != 0 || r != nil || v.PreserveInstanceID || v.PreserveMaterials {
				return nil, fmt.Errorf("invalid add_card shape")
			}
		case "summon":
			if !validSide(v.Owner) || v.Count < 0 || !validCardID(v.CardID) || v.Output != "summoned" || v.Destination != "" || v.TieBreak != "" || v.MaxCost != 0 || r != nil || v.PreserveInstanceID || v.PreserveMaterials {
				return nil, fmt.Errorf("invalid summon shape")
			}
		case "reanimate":
			if !validSide(v.Owner) || v.MaxCost < 0 || v.TieBreak != "random" || v.Output != "summoned" || v.CardID != 0 || v.Count != 0 || v.Destination != "" || r != nil || v.PreserveInstanceID || v.PreserveMaterials {
				return nil, fmt.Errorf("invalid reanimate shape")
			}
		case "transform":
			if !ValidTransformTarget(r) || !validCardID(v.CardID) || !v.PreserveInstanceID || !v.PreserveMaterials || v.Owner != "" || v.Destination != "" || v.Output != "" || v.TieBreak != "" || v.Count != 0 || v.MaxCost != 0 {
				return nil, fmt.Errorf("invalid transform shape")
			}
		}
		return CardEffect{NodeBase{v.ID, v.Origin}, v.Kind, v.Owner, v.Destination, v.Output, v.Count, v.CardID, v.MaxCost, v.TieBreak, v.PreserveInstanceID, v.PreserveMaterials, r}, err
	case "damage", "heal", "buff_stats", "destroy", "banish", "discard", "return", "add_keyword", "remove_keyword", "silent_evolve", "set_attack_limit", "set_life":
		type raw struct {
			Output                                                      string `json:"output,omitempty"`
			ID                                                          string `json:"id"`
			Kind, DamageType, Keyword, Form, Destination, DeckInsertion string
			Distribution                                                string          `json:"distribution,omitempty"`
			Until                                                       string          `json:"until,omitempty"`
			Overflow                                                    json.RawMessage `json:"overflow,omitempty"`
			Target                                                      json.RawMessage `json:"target"`
			Amount                                                      int             `json:"-"`
			AmountValue                                                 json.RawMessage `json:"amount"`
			AttackDelta, LifeDelta                                      int             `json:"-"`
			AttackValue                                                 json.RawMessage `json:"attackDelta"`
			LifeValue                                                   json.RawMessage `json:"lifeDelta"`
			Predicate                                                   json.RawMessage `json:"predicate,omitempty"`
			Origin                                                      Origin          `json:"origin"`
		}
		var v raw
		if err := strict(data, &v); err != nil {
			return nil, err
		}
		if v.Output != "" && (v.Kind != "destroy" || v.Output != "destroyed") {
			return nil, fmt.Errorf("invalid target effect output")
		}
		if v.Until != "" && (!oneOf(v.Kind, "add_keyword", "buff_stats") || !oneOf(v.Until, "turn_end", "own_turn_end", "oppo_turn_end")) {
			return nil, fmt.Errorf("invalid effect duration")
		}
		if err := newNode(v.ID, nodeIDs, v.Origin); err != nil {
			return nil, err
		}
		decodeTarget := decodeRef
		if v.Kind == "destroy" {
			decodeTarget = decodeDestructionTarget
		}
		r, err := decodeTarget(v.Target)
		if err != nil {
			return nil, err
		}
		if (v.Kind == "discard" || v.Kind == "set_life") && (r.refKind() == "leader" || r.refKind() == "leaders") {
			return nil, fmt.Errorf("%s requires card instances", v.Kind)
		}
		var p Predicate
		if len(v.Predicate) > 0 {
			p, err = decodePredicate(v.Predicate)
		}
		if _, batch := r.(DestructionBatchRef); batch && len(v.Predicate) != 0 {
			return nil, fmt.Errorf("destruction batch does not accept a predicate")
		}
		var amountExpr, attackExpr, lifeExpr NumericExpr
		if len(v.AmountValue) > 0 {
			var amountErr error
			v.Amount, amountExpr, amountErr = decodeEffectAmount(v.AmountValue)
			if amountErr != nil {
				return nil, amountErr
			}
		}
		if amountExpr != nil && v.Kind != "damage" && v.Kind != "heal" && v.Kind != "set_life" {
			return nil, fmt.Errorf("numeric amount is only supported for damage, heal and set_life")
		}
		if len(v.AttackValue) > 0 {
			var deltaErr error
			v.AttackDelta, attackExpr, deltaErr = decodeNumericValue(v.AttackValue, true)
			if deltaErr != nil {
				return nil, deltaErr
			}
		}
		if len(v.LifeValue) > 0 {
			var deltaErr error
			v.LifeDelta, lifeExpr, deltaErr = decodeNumericValue(v.LifeValue, true)
			if deltaErr != nil {
				return nil, deltaErr
			}
		}
		if (attackExpr != nil || lifeExpr != nil) && v.Kind != "buff_stats" {
			return nil, fmt.Errorf("numeric deltas are only supported for buffs")
		}
		var overflow Ref
		if len(v.Overflow) > 0 {
			var overflowErr error
			overflow, overflowErr = decodeRef(v.Overflow)
			if overflowErr != nil {
				return nil, overflowErr
			}
		}
		if !validDamageDistribution(v.Kind, v.Distribution, r, overflow) {
			return nil, fmt.Errorf("invalid damage distribution")
		}
		switch v.Kind {
		case "damage":
			if v.DamageType != "effect" || v.Amount < 0 || v.Keyword != "" || v.Form != "" || v.Destination != "" || v.DeckInsertion != "" || v.AttackDelta != 0 || v.LifeDelta != 0 {
				return nil, fmt.Errorf("invalid damage shape")
			}
		case "heal":
			if v.DamageType != "" || v.Amount < 0 || v.Keyword != "" || v.Form != "" || v.Destination != "" || v.DeckInsertion != "" || v.AttackDelta != 0 || v.LifeDelta != 0 {
				return nil, fmt.Errorf("invalid heal shape")
			}
		case "buff_stats":
			if v.DamageType != "" || v.Amount != 0 || v.Keyword != "" || v.Form != "" || v.Destination != "" || v.DeckInsertion != "" {
				return nil, fmt.Errorf("invalid buff shape")
			}
		case "destroy", "banish", "discard":
			if v.DamageType != "" || v.Amount != 0 || v.Keyword != "" || v.Form != "" || v.Destination != "" || v.DeckInsertion != "" || v.AttackDelta != 0 || v.LifeDelta != 0 {
				return nil, fmt.Errorf("invalid removal shape")
			}
		case "return":
			if !oneOf(v.Destination, "hand", "deck") || v.Destination == "deck" && v.DeckInsertion != "uniform_random_position" || v.Destination == "hand" && v.DeckInsertion != "" || v.DamageType != "" || v.Amount != 0 || v.Keyword != "" || v.Form != "" || v.AttackDelta != 0 || v.LifeDelta != 0 || p != nil {
				return nil, fmt.Errorf("invalid return shape")
			}
		case "add_keyword", "remove_keyword":
			if !validKeyword(v.Keyword) || v.DamageType != "" || v.Amount != 0 || v.Form != "" || v.Destination != "" || v.DeckInsertion != "" || v.AttackDelta != 0 || v.LifeDelta != 0 {
				return nil, fmt.Errorf("invalid keyword effect")
			}
		case "silent_evolve":
			if !oneOf(v.Form, "evolved", "super_evolved") || v.DamageType != "" || v.Amount != 0 || v.Keyword != "" || v.Destination != "" || v.DeckInsertion != "" || v.AttackDelta != 0 || v.LifeDelta != 0 || p != nil {
				return nil, fmt.Errorf("invalid silent evolve")
			}
		case "set_attack_limit":
			if v.Amount < 1 || v.DamageType != "" || v.Keyword != "" || v.Form != "" || v.Destination != "" || v.DeckInsertion != "" || v.AttackDelta != 0 || v.LifeDelta != 0 || p != nil {
				return nil, fmt.Errorf("invalid attack limit shape")
			}
		case "set_life":
			if len(v.AmountValue) == 0 || v.Amount < 0 || v.DamageType != "" || v.Keyword != "" || v.Form != "" || v.Destination != "" || v.DeckInsertion != "" || v.AttackDelta != 0 || v.LifeDelta != 0 || p != nil {
				return nil, fmt.Errorf("invalid set life shape")
			}
		}
		return TargetEffect{
			NodeBase: NodeBase{v.ID, v.Origin}, Kind: v.Kind, Output: v.Output, DamageType: v.DamageType,
			Distribution: v.Distribution, Overflow: overflow,
			Keyword: v.Keyword, Until: v.Until, Form: v.Form, Destination: v.Destination, DeckInsertion: v.DeckInsertion,
			Target: r, Amount: v.Amount, AmountExpr: amountExpr, AttackDelta: v.AttackDelta, LifeDelta: v.LifeDelta, AttackExpr: attackExpr, LifeExpr: lifeExpr, Predicate: p,
		}, err
	case "adjust_resource", "restore_resource", "adjust_earthsigil", "adjust_entity_field", "spellboost", "adjust_counter":
		type raw struct {
			ID                           string `json:"id"`
			Kind, Owner, Resource, Field string
			Target                       json.RawMessage `json:"target,omitempty"`
			Delta, Minimum, Times        int
			Origin                       Origin `json:"origin"`
		}
		var v raw
		if err := strict(data, &v); err != nil {
			return nil, err
		}
		if err := newNode(v.ID, nodeIDs, v.Origin); err != nil {
			return nil, err
		}
		var r Ref
		var err error
		if len(v.Target) > 0 {
			r, err = decodeRef(v.Target)
		}
		switch v.Kind {
		case "adjust_counter":
			if v.Owner != "" || v.Resource != "" || !ValidCounterName(v.Field) || r != nil || v.Minimum != 0 || v.Times != 0 || v.Delta < 0 || v.Delta > MaxCounterValue {
				return nil, fmt.Errorf("invalid counter adjustment")
			}
		case "restore_resource":
			if !validSide(v.Owner) || v.Resource != "pp" || v.Field != "" || r != nil || v.Delta != 0 || v.Minimum != 0 || v.Times != 0 {
				return nil, fmt.Errorf("invalid resource restoration")
			}
		case "adjust_resource":
			if !validSide(v.Owner) || !oneOf(v.Resource, "pp", "combo", "maxpp", "shadows") || v.Field != "" || r != nil || v.Minimum != 0 || v.Times != 0 {
				return nil, fmt.Errorf("invalid resource adjustment")
			}
		case "adjust_earthsigil":
			if !validSide(v.Owner) || v.Resource != "" || v.Field != "" || r != nil || v.Minimum != 0 || v.Times != 0 {
				return nil, fmt.Errorf("invalid earthsigil adjustment")
			}
		case "adjust_entity_field":
			if v.Owner != "" || v.Resource != "" || !oneOf(v.Field, "cost", "countdown") || r == nil || v.Times != 0 {
				return nil, fmt.Errorf("invalid entity adjustment")
			}
		case "spellboost":
			if v.Owner != "" || v.Resource != "" || v.Field != "" || r == nil || v.Delta != 0 || v.Minimum != 0 || v.Times < 0 {
				return nil, fmt.Errorf("invalid spellboost")
			}
		}
		return AdjustEffect{NodeBase{v.ID, v.Origin}, v.Kind, v.Owner, v.Resource, v.Field, r, v.Delta, v.Minimum, v.Times}, err
	default:
		return nil, fmt.Errorf("unknown executable effect kind %q", k.Kind)
	}
}

func decodeRef(data []byte) (Ref, error) {
	var k rawKind
	if err := json.Unmarshal(data, &k); err != nil {
		return nil, err
	}
	switch k.Kind {
	case "history":
		var ref HistoryRef
		if err := strict(data, &ref); err != nil {
			return nil, err
		}
		if !validHistoryRef(ref) {
			return nil, fmt.Errorf("invalid history reference")
		}
		return ref, nil
	case "self":
		var v struct {
			Kind      string `json:"kind"`
			ValueType string `json:"valueType"`
		}
		if err := strict(data, &v); err != nil {
			return nil, err
		}
		if v.ValueType != "" && v.ValueType != "entity" {
			return nil, fmt.Errorf("invalid self reference")
		}
		return SelfRef{v.Kind, v.ValueType}, nil
	case "binding":
		var v struct {
			Kind string `json:"kind"`
			Name string `json:"name"`
		}
		if err := strict(data, &v); err != nil {
			return nil, err
		}
		if v.Name == "" {
			return nil, fmt.Errorf("empty binding reference")
		}
		return BindingRef{v.Kind, v.Name}, nil
	case "leader":
		var v struct {
			Kind      string `json:"kind"`
			Side      string `json:"side"`
			ValueType string `json:"valueType,omitempty"`
		}
		if err := strict(data, &v); err != nil {
			return nil, err
		}
		if !validSide(v.Side) || v.ValueType != "" && v.ValueType != "leader" {
			return nil, fmt.Errorf("invalid leader reference")
		}
		return LeaderRef{v.Kind, v.Side, v.ValueType}, nil
	case "leaders":
		var v struct {
			Kind      string `json:"kind"`
			ValueType string `json:"valueType"`
		}
		if err := strict(data, &v); err != nil {
			return nil, err
		}
		if v.ValueType != "leaders" {
			return nil, fmt.Errorf("invalid leader set reference")
		}
		return LeaderSetRef{v.Kind, v.ValueType}, nil
	case "zone":
		var v struct {
			Kind   string `json:"kind"`
			Side   string `json:"side,omitempty"`
			Zone   string `json:"zone"`
			Member string `json:"member,omitempty"`
		}
		if err := strict(data, &v); err != nil {
			return nil, err
		}
		if v.Side != "" && !validSide(v.Side) || !validZone(v.Zone) || v.Zone == "crests" || v.Member != "" && !oneOf(v.Member, "card", "follower", "spell", "amulet") {
			return nil, fmt.Errorf("invalid zone reference")
		}
		return ZoneRef{v.Kind, v.Side, v.Zone, v.Member}, nil
	case "filter":
		var v struct {
			Kind      string          `json:"kind"`
			Source    json.RawMessage `json:"source"`
			Predicate json.RawMessage `json:"predicate"`
		}
		if err := strict(data, &v); err != nil {
			return nil, err
		}
		s, err := decodeRef(v.Source)
		if err != nil {
			return nil, err
		}
		p, err := decodePredicate(v.Predicate)
		return FilterRef{v.Kind, s, p}, err
	case "exclude":
		var v struct {
			Kind   string          `json:"kind"`
			Source json.RawMessage `json:"source"`
			Value  json.RawMessage `json:"value"`
		}
		if err := strict(data, &v); err != nil {
			return nil, err
		}
		s, err := decodeRef(v.Source)
		if err != nil {
			return nil, err
		}
		x, err := decodeRef(v.Value)
		return ExcludeRef{v.Kind, s, x}, err
	default:
		return nil, fmt.Errorf("unknown reference kind %q", k.Kind)
	}
}
func decodePredicate(data []byte) (Predicate, error) {
	var k rawKind
	if err := json.Unmarshal(data, &k); err != nil {
		return nil, err
	}
	if k.Kind == "and" || k.Kind == "or" {
		var v struct {
			Kind  string            `json:"kind"`
			Terms []json.RawMessage `json:"terms"`
		}
		if err := strict(data, &v); err != nil {
			return nil, err
		}
		if len(v.Terms) < 2 {
			return nil, fmt.Errorf("%s predicate requires at least two terms", k.Kind)
		}
		p := AndPredicate{Kind: v.Kind}
		for _, x := range v.Terms {
			t, err := decodePredicate(x)
			if err != nil {
				return nil, err
			}
			p.Terms = append(p.Terms, t)
		}
		if k.Kind == "or" {
			return OrPredicate{Kind: k.Kind, Terms: p.Terms}, nil
		}
		return p, nil
	}
	switch k.Kind {
	case "has_card":
		var v struct {
			Kind   string `json:"kind"`
			CardID int    `json:"cardId"`
		}
		if err := strict(data, &v); err != nil {
			return nil, err
		}
		if !validCardID(v.CardID) {
			return nil, fmt.Errorf("invalid predicate card ID")
		}
		return FieldPredicate{Kind: v.Kind, CardID: v.CardID}, nil
	case "has_type":
		var v struct {
			Kind     string `json:"kind"`
			CardType string `json:"cardType"`
		}
		if err := strict(data, &v); err != nil {
			return nil, err
		}
		if !oneOf(v.CardType, "follower", "spell", "amulet") {
			return nil, fmt.Errorf("invalid predicate card type")
		}
		return FieldPredicate{Kind: v.Kind, CardType: v.CardType}, nil
	case "has_class":
		var v struct {
			Kind  string `json:"kind"`
			Class string `json:"class"`
		}
		if err := strict(data, &v); err != nil {
			return nil, err
		}
		if !oneOf(v.Class, "neutral", "forestcraft", "swordcraft", "runecraft", "dragoncraft", "abysscraft", "havencraft", "portalcraft") {
			return nil, fmt.Errorf("invalid predicate class")
		}
		return FieldPredicate{Kind: v.Kind, Class: v.Class}, nil
	case "has_spellboost":
		var v struct {
			Kind string `json:"kind"`
		}
		if err := strict(data, &v); err != nil {
			return nil, err
		}
		return FieldPredicate{Kind: v.Kind}, nil
	case "has_trait":
		var v struct {
			Kind  string `json:"kind"`
			Trait string `json:"trait"`
		}
		if err := strict(data, &v); err != nil {
			return nil, err
		}
		if !ValidTrait(v.Trait) {
			return nil, fmt.Errorf("invalid predicate trait %q", v.Trait)
		}
		return FieldPredicate{Kind: v.Kind, Trait: v.Trait}, nil
	case "has_keyword":
		var v struct {
			Kind    string `json:"kind"`
			Keyword string `json:"keyword"`
		}
		if err := strict(data, &v); err != nil {
			return nil, err
		}
		if !ValidKeyword(v.Keyword) {
			return nil, fmt.Errorf("invalid predicate keyword %q", v.Keyword)
		}
		return FieldPredicate{Kind: v.Kind, Keyword: v.Keyword}, nil
	case "has_form":
		var v struct {
			Kind string `json:"kind"`
			Form string `json:"form"`
		}
		if err := strict(data, &v); err != nil {
			return nil, err
		}
		if !oneOf(v.Form, "unevolved", "evolved", "super_evolved") {
			return nil, fmt.Errorf("invalid predicate form")
		}
		return FieldPredicate{Kind: v.Kind, Form: v.Form}, nil
	case "compare":
		var v struct {
			Kind, Field, Op string
			Value           json.RawMessage
		}
		if err := strict(data, &v); err != nil {
			return nil, err
		}
		if !oneOf(v.Field, "life", "cost") || !validOp(v.Op) {
			return nil, fmt.Errorf("invalid comparison predicate")
		}
		value, expr, err := decodeNumericValue(v.Value, true)
		if err != nil {
			return nil, err
		}
		var scalar *Scalar
		if expr != nil {
			var ok bool
			scalar, ok = expr.(*Scalar)
			if !ok || scalar.Kind != "scalar" {
				return nil, fmt.Errorf("predicate value requires an integer or player scalar")
			}
		}
		return FieldPredicate{Kind: v.Kind, Field: v.Field, Op: v.Op, Value: value, ValueScalar: scalar}, nil
	default:
		return nil, fmt.Errorf("unknown predicate kind %q", k.Kind)
	}
}
func decodeCondition(data []byte) (Condition, error) {
	var k rawKind
	if err := json.Unmarshal(data, &k); err != nil {
		return nil, err
	}
	if k.Kind == "self_form" {
		var v SelfFormCondition
		if err := strict(data, &v); err != nil {
			return nil, err
		}
		if !oneOf(v.Form, "unevolved", "evolved", "super_evolved") {
			return nil, fmt.Errorf("invalid self form condition")
		}
		return v, nil
	}
	if k.Kind == "evolution_unlocked" {
		var v EvolutionUnlockedCondition
		if err := strict(data, &v); err != nil {
			return nil, err
		}
		if !validSide(v.Side) || !oneOf(v.Form, "evolved", "super_evolved") {
			return nil, fmt.Errorf("invalid evolution unlock condition")
		}
		return v, nil
	}
	if k.Kind == "overflow" {
		var v struct {
			Kind string `json:"kind"`
			Side string `json:"side"`
		}
		if err := strict(data, &v); err != nil {
			return nil, err
		}
		if !validSide(v.Side) {
			return nil, fmt.Errorf("invalid overflow side")
		}
		return OverflowCondition{v.Kind, v.Side}, nil
	}
	if k.Kind != "compare" {
		return nil, fmt.Errorf("unknown condition kind %q", k.Kind)
	}
	var v struct {
		Kind  string `json:"kind"`
		Left  Scalar `json:"left"`
		Op    string `json:"op"`
		Right int    `json:"right"`
	}
	if err := strict(data, &v); err != nil {
		return nil, err
	}
	if !oneOf(v.Left.Kind, "scalar", "fusion_material_scalar", "self_counter") || !validOp(v.Op) {
		return nil, fmt.Errorf("unknown scalar kind")
	}
	if v.Left.Kind == "scalar" && (!validSide(v.Left.Side) || !ValidPlayerScalar(v.Left.Field)) || v.Left.Kind == "fusion_material_scalar" && (v.Left.Side != "" || !oneOf(v.Left.Field, "cost", "distinct")) {
		return nil, fmt.Errorf("invalid condition scalar")
	}
	if v.Left.Kind == "self_counter" && (v.Left.Side != "" || !ValidCounterName(v.Left.Field)) {
		return nil, fmt.Errorf("invalid counter condition")
	}
	return CompareCondition{v.Kind, v.Op, v.Left, v.Right}, nil
}
func newNode(id string, seen map[string]bool, origins ...Origin) error {
	if !nodeIDPattern.MatchString(id) {
		return fmt.Errorf("malformed node ID %q", id)
	}
	if seen[id] {
		return fmt.Errorf("duplicate node ID %s", id)
	}
	if len(origins) != 0 && !validOrigin(origins[0]) {
		return fmt.Errorf("malformed node origin")
	}
	seen[id] = true
	return nil
}

func validOrigin(o Origin) bool {
	return nodeIDPattern.MatchString(o.Primary.SourceID) && o.Primary.StartLine != 0
}

func validateCardRefs(c Card, cards, crests map[int]bool) error {
	if c.Crest != nil {
		if err := validateCardRefs(*c.CrestCard(), cards, crests); err != nil {
			return err
		}
	}
	var walk func([]Effect) error
	walk = func(es []Effect) error {
		for _, e := range es {
			switch x := e.(type) {
			case GrantEffect:
				granted := Card{CardType: "follower", Abilities: []Ability{x.Ability}}
				if err := validateCardRefs(granted, cards, crests); err != nil {
					return err
				}
			case CardEffect:
				if x.Kind == "gain_crest" && !crests[x.CardID] {
					return fmt.Errorf("card %d has no crest definition", x.CardID)
				}
				if x.CardID != 0 && (!validCardID(x.CardID) || !cards[x.CardID]) {
					return fmt.Errorf("bad card ref %d", x.CardID)
				}
			case AdjustEffect:
				if x.Kind == "adjust_earthsigil" && x.Delta > 0 && !cards[MagicSedimentCardID] {
					return fmt.Errorf("earth sigil gain requires card %d", MagicSedimentCardID)
				}
			case TargetEffect:
				for _, expr := range []NumericExpr{x.AmountExpr, x.AttackExpr, x.LifeExpr} {
					if err := numericCardRefs(expr, cards, c.CardType); err != nil {
						return err
					}
				}
			case IfEffect:
				if err := walk(x.Then); err != nil {
					return err
				}
				if err := walk(x.Else); err != nil {
					return err
				}
			case RepeatEffect:
				if err := numericCardRefs(x.TimesExpr, cards, c.CardType); err != nil {
					return err
				}
				if err := walk(x.Body); err != nil {
					return err
				}
			case ModeEffect:
				for _, o := range x.Options {
					if err := walk(o.Body); err != nil {
						return err
					}
				}
			case PayResourceEffect:
				if err := walk(x.OnPaid); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := walk(c.PlayEffects); err != nil {
		return err
	}
	for _, a := range c.Abilities {
		if err := walk(a.Body); err != nil {
			return err
		}
	}
	for _, a := range c.FusionAbilities {
		if err := walk(a.Body); err != nil {
			return err
		}
	}
	return nil
}
func validCardID(id int) bool { return id >= 10000000 && id <= 99999999 }

func validatePredicateCardRefs(predicate Predicate, cards map[int]bool) error {
	switch p := predicate.(type) {
	case FieldPredicate:
		if p.Kind == "has_card" && !cards[p.CardID] {
			return fmt.Errorf("bad card ref %d", p.CardID)
		}
	case AndPredicate:
		for _, term := range p.Terms {
			if err := validatePredicateCardRefs(term, cards); err != nil {
				return err
			}
		}
	case OrPredicate:
		for _, term := range p.Terms {
			if err := validatePredicateCardRefs(term, cards); err != nil {
				return err
			}
		}
	}
	return nil
}

func validKeyword(v string) bool {
	return oneOf(v, "ward", "storm", "rush", "bane", "drain", "intimidate", "barrier", "stealth", "aura", "ability_target_guard", "cannot_attack", "cannot_attack_follower", "cannot_attack_leader")
}

func ValidKeyword(v string) bool { return validKeyword(v) }
func validOp(v string) bool      { return oneOf(v, "eq", "ne", "lt", "le", "gt", "ge") }
func oneOf(s string, v ...string) bool {
	for _, x := range v {
		if s == x {
			return true
		}
	}
	return false
}
func parseSeed(seed string) (uint64, error) {
	if !seedPattern.MatchString(seed) {
		return 0, fmt.Errorf("malformed seed %q", seed)
	}
	return strconv.ParseUint(seed[2:], 16, 64)
}
