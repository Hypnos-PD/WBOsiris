package ir

import (
	"encoding/json"
	"fmt"
)

const (
	Format                = "wbos"
	ContainerVersion      = "0.1.0"
	IRVersion             = "0.2.0"
	SourceLanguageVersion = "0.1.0"
	Encoding              = "canonical-json"
)

type SourceSpan struct {
	SourceID    string `json:"sourceId"`
	StartByte   uint32 `json:"startByte"`
	EndByte     uint32 `json:"endByte"`
	StartLine   uint32 `json:"startLine"`
	StartColumn uint32 `json:"startColumn"`
	EndLine     uint32 `json:"endLine"`
	EndColumn   uint32 `json:"endColumn"`
}

// UnmarshalJSON is implemented explicitly because the serialized span uses
// distinct field names while the public representation stays compact.
func (s *SourceSpan) UnmarshalJSON(data []byte) error {
	type span struct {
		SourceID    string `json:"sourceId"`
		StartByte   uint32 `json:"startByte"`
		EndByte     uint32 `json:"endByte"`
		StartLine   uint32 `json:"startLine"`
		StartColumn uint32 `json:"startColumn"`
		EndLine     uint32 `json:"endLine"`
		EndColumn   uint32 `json:"endColumn"`
	}
	var v span
	if err := strict(data, &v); err != nil {
		return err
	}
	*s = SourceSpan{v.SourceID, v.StartByte, v.EndByte, v.StartLine, v.StartColumn, v.EndLine, v.EndColumn}
	if !nodeIDPattern.MatchString(s.SourceID) || s.StartLine == 0 || s.StartColumn == 0 || s.EndLine == 0 || s.EndColumn == 0 || s.EndByte < s.StartByte {
		return fmt.Errorf("malformed source span")
	}
	return nil
}

type Origin struct {
	Primary SourceSpan `json:"primary"`
}
type Source struct {
	SourceID    string     `json:"sourceId"`
	Path        string     `json:"path"`
	ContentHash string     `json:"contentHash"`
	Span        SourceSpan `json:"span"`
}

type CardPack struct {
	Format                string   `json:"format"`
	ContainerVersion      string   `json:"containerVersion"`
	Encoding              string   `json:"encoding"`
	Kind                  string   `json:"kind"`
	IRVersion             string   `json:"irVersion"`
	SourceLanguageVersion string   `json:"sourceLanguageVersion"`
	RequiredFeatures      []string `json:"requiredFeatures"`
	UnresolvedReferences  []string `json:"unresolvedReferences"`
	Sources               []Source `json:"sources"`
	Cards                 []Card   `json:"cards"`
	ContentHash           string   `json:"contentHash"`
}

type TestPack struct {
	Format               string            `json:"format"`
	ContainerVersion     string            `json:"containerVersion"`
	Encoding             string            `json:"encoding"`
	Kind                 string            `json:"kind"`
	IRVersion            string            `json:"irVersion"`
	RequiredFeatures     []string          `json:"requiredFeatures"`
	UnresolvedReferences []string          `json:"unresolvedReferences"`
	Ruleset              RulesetDependency `json:"ruleset"`
	Sources              []Source          `json:"sources"`
	Scenarios            []Scenario        `json:"scenarios"`
	ContentHash          string            `json:"contentHash"`
}

type RulesetDependency struct {
	ID              string                `json:"id"`
	RNG             RNGPolicy             `json:"rng"`
	OrderingPolicy  OrderingPolicy        `json:"orderingPolicy"`
	ExecutionBudget ExecutionBudgetPolicy `json:"executionBudget"`
	ContentHash     string                `json:"contentHash"`
}
type RNGPolicy struct {
	Algorithm string `json:"algorithm"`
	Version   string `json:"version"`
}
type OrderingPolicy struct {
	Collections          string `json:"collections"`
	ReplacementAbilities string `json:"replacementAbilities"`
	SimultaneousTriggers string `json:"simultaneousTriggers"`
	DeathBatchLastwords  string `json:"deathBatchLastwords"`
}
type ExecutionBudgetPolicy struct {
	Instructions      uint64 `json:"instructions"`
	QueryVisits       uint64 `json:"queryVisits"`
	StackDepth        uint64 `json:"stackDepth"`
	Candidates        uint64 `json:"candidates"`
	Events            uint64 `json:"events"`
	Triggers          uint64 `json:"triggers"`
	CreatedInstances  uint64 `json:"createdInstances"`
	ContinuationBytes uint64 `json:"continuationBytes"`
}
type Stats struct {
	Attack int `json:"attack"`
	Life   int `json:"life"`
}
type Meta struct {
	Pack   int    `json:"pack"`
	Class  string `json:"class"`
	Rarity string `json:"rarity"`
}
type Locale struct {
	Name string `json:"name"`
	Text string `json:"text"`
}

type Card struct {
	ID              int               `json:"id"`
	CardType        string            `json:"cardType"`
	Cost            int               `json:"cost"`
	Stats           *Stats            `json:"stats,omitempty"`
	Traits          []string          `json:"traits"`
	Intrinsic       []string          `json:"intrinsic"`
	IntrinsicState  []IntrinsicState  `json:"intrinsicState"`
	Restrictions    []Restriction     `json:"restrictions"`
	Abilities       []Ability         `json:"abilities"`
	FusionAbilities []FusionAbility   `json:"fusionAbilities"`
	PlayEffects     []Effect          `json:"playEffects"`
	ActionPlans     []ActionPlan      `json:"actionPlans"`
	Meta            Meta              `json:"meta"`
	Locales         map[string]Locale `json:"locales"`
	Origin          Origin            `json:"origin"`
}

type IntrinsicState struct {
	Kind    string `json:"kind"`
	Initial int    `json:"initial"`
}
type Restriction struct {
	Kind string `json:"kind"`
}
type Ability struct {
	ID       string   `json:"id"`
	Trigger  Trigger  `json:"trigger"`
	Relation string   `json:"relation"`
	Body     []Effect `json:"body"`
	Origin   Origin   `json:"origin"`
}
type FusionAbility struct {
	ID             string         `json:"id"`
	MaterialFilter MaterialFilter `json:"materialFilter"`
	Body           []Effect       `json:"body"`
	Origin         Origin         `json:"origin"`
}
type MaterialFilter struct {
	Kind          string    `json:"kind"`
	Source        Ref       `json:"source"`
	Predicate     Predicate `json:"predicate"`
	ExcludeSource bool      `json:"excludeSource"`
	Minimum       int       `json:"minimum"`
}
type ActionPlan struct {
	Action string     `json:"action"`
	Steps  []PlanStep `json:"steps"`
}
type PlanStep struct {
	AbilityID string `json:"abilityId"`
	Frame     string `json:"frame"`
}

type Trigger interface{ triggerKind() string }
type SimpleTrigger struct {
	Kind string `json:"kind"`
}

func (t SimpleTrigger) triggerKind() string { return t.Kind }

type CostTrigger struct {
	Kind string `json:"kind"`
	Cost int    `json:"cost"`
}

func (t CostTrigger) triggerKind() string { return t.Kind }

type EventTrigger struct {
	Kind        string    `json:"kind"`
	Event       string    `json:"event"`
	Side        string    `json:"side"`
	SubjectType string    `json:"subjectType,omitempty"`
	Predicate   Predicate `json:"predicate,omitempty"`
}

func (t EventTrigger) triggerKind() string { return t.Kind }

type ReplacementTrigger struct {
	Kind    string `json:"kind"`
	Subject Ref    `json:"subject"`
	From    string `json:"from"`
	Phase   string `json:"phase"`
}

func (t ReplacementTrigger) triggerKind() string { return t.Kind }
func TriggerKind(t Trigger) string {
	if t == nil {
		return ""
	}
	return t.triggerKind()
}

type Ref interface{ refKind() string }
type SelfRef struct {
	Kind      string `json:"kind"`
	ValueType string `json:"valueType,omitempty"`
}

func (r SelfRef) refKind() string { return r.Kind }

type BindingRef struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
}

func (r BindingRef) refKind() string { return r.Kind }

type LeaderRef struct {
	Kind      string `json:"kind"`
	Side      string `json:"side"`
	ValueType string `json:"valueType"`
}

func (r LeaderRef) refKind() string { return r.Kind }

type ZoneRef struct {
	Kind   string `json:"kind"`
	Side   string `json:"side,omitempty"`
	Zone   string `json:"zone"`
	Member string `json:"member,omitempty"`
}

func (r ZoneRef) refKind() string { return r.Kind }

type FilterRef struct {
	Kind      string    `json:"kind"`
	Source    Ref       `json:"source"`
	Predicate Predicate `json:"predicate"`
}

func (r FilterRef) refKind() string { return r.Kind }

type ExcludeRef struct {
	Kind   string `json:"kind"`
	Source Ref    `json:"source"`
	Value  Ref    `json:"value"`
}

func (r ExcludeRef) refKind() string { return r.Kind }

type Predicate interface{ predicateKind() string }
type FieldPredicate struct {
	Kind     string `json:"kind"`
	CardType string `json:"cardType,omitempty"`
	Class    string `json:"class,omitempty"`
	Trait    string `json:"trait,omitempty"`
	Field    string `json:"field,omitempty"`
	Op       string `json:"op,omitempty"`
	CardID   int    `json:"cardId,omitempty"`
	Value    int    `json:"value,omitempty"`
}

func (p FieldPredicate) predicateKind() string { return p.Kind }

type AndPredicate struct {
	Kind  string      `json:"kind"`
	Terms []Predicate `json:"terms"`
}

func (p AndPredicate) predicateKind() string { return p.Kind }

type Condition interface{ conditionKind() string }
type OverflowCondition struct {
	Kind string `json:"kind"`
	Side string `json:"side"`
}

func (c OverflowCondition) conditionKind() string { return c.Kind }

type CompareCondition struct {
	Kind  string `json:"kind"`
	Op    string `json:"op"`
	Left  Scalar `json:"left"`
	Right int    `json:"right"`
}

func (c CompareCondition) conditionKind() string { return c.Kind }

type Scalar struct {
	Kind  string `json:"kind"`
	Side  string `json:"side,omitempty"`
	Field string `json:"field"`
}

type Effect interface {
	effectKind() string
	effectBase() NodeBase
}
type NodeBase struct {
	ID     string `json:"id"`
	Origin Origin `json:"origin"`
}
type SelectionEffect struct {
	NodeBase
	Kind    string `json:"kind"`
	Policy  string `json:"policy"`
	Binding string `json:"binding"`
	Source  Ref    `json:"source"`
}

func (e SelectionEffect) effectKind() string   { return e.Kind }
func (e SelectionEffect) effectBase() NodeBase { return e.NodeBase }

type IfEffect struct {
	NodeBase
	Kind      string    `json:"kind"`
	Condition Condition `json:"condition"`
	Then      []Effect  `json:"then"`
	Else      []Effect  `json:"else"`
}

func (e IfEffect) effectKind() string   { return e.Kind }
func (e IfEffect) effectBase() NodeBase { return e.NodeBase }

type ModeEffect struct {
	NodeBase
	Kind    string       `json:"kind"`
	Options []ModeOption `json:"options"`
}

func (e ModeEffect) effectKind() string   { return e.Kind }
func (e ModeEffect) effectBase() NodeBase { return e.NodeBase }

type ModeOption struct {
	ID     int      `json:"id"`
	Body   []Effect `json:"body"`
	Origin Origin   `json:"origin"`
}
type PayResourceEffect struct {
	NodeBase
	Kind     string   `json:"kind"`
	Resource string   `json:"resource"`
	Amount   int      `json:"amount"`
	OnPaid   []Effect `json:"onPaid"`
}

func (e PayResourceEffect) effectKind() string   { return e.Kind }
func (e PayResourceEffect) effectBase() NodeBase { return e.NodeBase }

type DrawEffect struct {
	NodeBase
	Kind       string    `json:"kind"`
	Owner      string    `json:"owner"`
	SourceZone string    `json:"sourceZone"`
	Output     string    `json:"output"`
	Count      int       `json:"count,omitempty"`
	All        bool      `json:"all"`
	Predicate  Predicate `json:"predicate,omitempty"`
}

func (e DrawEffect) effectKind() string   { return e.Kind }
func (e DrawEffect) effectBase() NodeBase { return e.NodeBase }

type CardEffect struct {
	NodeBase
	Kind               string `json:"kind"`
	Owner              string `json:"owner,omitempty"`
	Destination        string `json:"destination,omitempty"`
	Output             string `json:"output,omitempty"`
	Count              int    `json:"count,omitempty"`
	CardID             int    `json:"cardId,omitempty"`
	MaxCost            int    `json:"maxCost,omitempty"`
	TieBreak           string `json:"tieBreak,omitempty"`
	PreserveInstanceID bool   `json:"preserveInstanceId,omitempty"`
	PreserveMaterials  bool   `json:"preserveMaterials,omitempty"`
	Target             Ref    `json:"target,omitempty"`
}

func (e CardEffect) effectKind() string   { return e.Kind }
func (e CardEffect) effectBase() NodeBase { return e.NodeBase }

type TargetEffect struct {
	NodeBase
	Kind          string    `json:"kind"`
	DamageType    string    `json:"damageType,omitempty"`
	Keyword       string    `json:"keyword,omitempty"`
	Form          string    `json:"form,omitempty"`
	Destination   string    `json:"destination,omitempty"`
	DeckInsertion string    `json:"deckInsertion,omitempty"`
	Target        Ref       `json:"target"`
	Amount        int       `json:"amount,omitempty"`
	AttackDelta   int       `json:"attackDelta,omitempty"`
	LifeDelta     int       `json:"lifeDelta,omitempty"`
	Predicate     Predicate `json:"predicate,omitempty"`
}

func (e TargetEffect) effectKind() string   { return e.Kind }
func (e TargetEffect) effectBase() NodeBase { return e.NodeBase }

type AdjustEffect struct {
	NodeBase
	Kind     string `json:"kind"`
	Owner    string `json:"owner,omitempty"`
	Resource string `json:"resource,omitempty"`
	Field    string `json:"field,omitempty"`
	Target   Ref    `json:"target,omitempty"`
	Delta    int    `json:"delta,omitempty"`
	Minimum  int    `json:"minimum,omitempty"`
	Times    int    `json:"times,omitempty"`
}

func (e AdjustEffect) effectKind() string   { return e.Kind }
func (e AdjustEffect) effectBase() NodeBase { return e.NodeBase }
func EffectKind(e Effect) string {
	if e == nil {
		return ""
	}
	return e.effectKind()
}
func EffectBase(e Effect) NodeBase {
	if e == nil {
		return NodeBase{}
	}
	return e.effectBase()
}

type Scenario struct {
	ID           string      `json:"id"`
	Name         string      `json:"name"`
	Seed         string      `json:"seed"`
	InitialState State       `json:"initialState"`
	Actions      []Action    `json:"actions"`
	Assertions   []Assertion `json:"assertions"`
	Origin       Origin      `json:"origin"`
}
type State struct {
	Turn    Turn                   `json:"turn"`
	Phase   string                 `json:"phase"`
	Players map[string]PlayerState `json:"players"`
	Aliases map[string]string      `json:"aliases"`
}
type Turn struct {
	Active string `json:"active"`
	Number int    `json:"number"`
}
type Leader struct {
	Life    int `json:"life"`
	MaxLife int `json:"maxLife"`
}
type PlayerState struct {
	Leader  Leader                    `json:"leader"`
	PP      int                       `json:"pp"`
	MaxPP   int                       `json:"maxpp"`
	EP      int                       `json:"ep"`
	SEP     int                       `json:"sep"`
	Combo   int                       `json:"combo"`
	Shadows int                       `json:"shadows"`
	Zones   map[string][]TestInstance `json:"zones"`
}
type TestInstance struct {
	InstanceID   string            `json:"instanceId"`
	Alias        string            `json:"alias"`
	CardID       int               `json:"cardId"`
	DeclaredType string            `json:"declaredType"`
	Overrides    InstanceOverrides `json:"overrides"`
}
type InstanceOverrides struct {
	Stats        *Stats   `json:"stats,omitempty"`
	Evolved      *bool    `json:"evolved,omitempty"`
	SuperEvolved *bool    `json:"super_evolved,omitempty"`
	Engaged      *bool    `json:"engaged,omitempty"`
	Keywords     []string `json:"keywords,omitempty"`
	Countdown    *int     `json:"countdown,omitempty"`
	Earthsigil   *int     `json:"earthsigil,omitempty"`
}

type Action interface{ actionKind() string }
type SourceAction struct {
	Kind   string `json:"kind"`
	Actor  string `json:"actor"`
	Source string `json:"source"`
}

func (a SourceAction) actionKind() string { return a.Kind }

type SelectAction struct {
	Kind   string `json:"kind"`
	Target string `json:"target"`
}

func (a SelectAction) actionKind() string { return a.Kind }

type ModeAction struct {
	Kind     string `json:"kind"`
	OptionID int    `json:"optionId"`
}

func (a ModeAction) actionKind() string { return a.Kind }

type AttackAction struct {
	Kind     string `json:"kind"`
	Actor    string `json:"actor"`
	Attacker string `json:"attacker"`
	Defender string `json:"defender"`
}

func (a AttackAction) actionKind() string { return a.Kind }

type AdvanceAction struct {
	Kind   string `json:"kind"`
	Timing string `json:"timing"`
	Side   string `json:"side"`
}

func (a AdvanceAction) actionKind() string { return a.Kind }
func ActionKind(a Action) string {
	if a == nil {
		return ""
	}
	return a.actionKind()
}

type Assertion interface {
	assertionKind() string
	assertionOrigin() Origin
}
type BasicAssertion struct {
	Kind   string `json:"kind"`
	Code   string `json:"code,omitempty"`
	Origin Origin `json:"origin"`
}

func (a BasicAssertion) assertionKind() string   { return a.Kind }
func (a BasicAssertion) assertionOrigin() Origin { return a.Origin }

type CompareAssertion struct {
	Kind   string  `json:"kind"`
	Op     string  `json:"op"`
	Left   TestRef `json:"left"`
	Right  Literal `json:"right"`
	Origin Origin  `json:"origin"`
}

func (a CompareAssertion) assertionKind() string   { return a.Kind }
func (a CompareAssertion) assertionOrigin() Origin { return a.Origin }

type KeywordAssertion struct {
	Kind     string `json:"kind"`
	Target   string `json:"target,omitempty"`
	Keyword  string `json:"keyword"`
	Expected bool   `json:"expected,omitempty"`
	Source   Ref    `json:"source,omitempty"`
	Origin   Origin `json:"origin"`
}

func (a KeywordAssertion) assertionKind() string   { return a.Kind }
func (a KeywordAssertion) assertionOrigin() Origin { return a.Origin }

type ZoneAssertion struct {
	Kind        string   `json:"kind"`
	Side        string   `json:"side,omitempty"`
	Zone        string   `json:"zone,omitempty"`
	Op          string   `json:"op,omitempty"`
	Containment string   `json:"containment,omitempty"`
	CardID      int      `json:"cardId,omitempty"`
	Count       int      `json:"count,omitempty"`
	Source      Ref      `json:"source,omitempty"`
	Expected    []string `json:"expected,omitempty"`
	Origin      Origin   `json:"origin"`
}

func (a ZoneAssertion) assertionKind() string   { return a.Kind }
func (a ZoneAssertion) assertionOrigin() Origin { return a.Origin }

type EventsAssertion struct {
	Kind     string         `json:"kind"`
	Mode     string         `json:"mode"`
	Expected []EventMatcher `json:"expected"`
	Origin   Origin         `json:"origin"`
}

func (a EventsAssertion) assertionKind() string   { return a.Kind }
func (a EventsAssertion) assertionOrigin() Origin { return a.Origin }
func AssertionKind(a Assertion) string {
	if a == nil {
		return ""
	}
	return a.assertionKind()
}
func AssertionOrigin(a Assertion) Origin { return a.assertionOrigin() }

type TestRef struct {
	Kind       string `json:"kind"`
	Side       string `json:"side,omitempty"`
	Field      string `json:"field,omitempty"`
	InstanceID string `json:"instanceId,omitempty"`
}
type Literal struct {
	Kind    string
	Value   string
	Integer int
	Boolean bool
	Attack  int
	Life    int
	Current int
	Maximum int
}
type EventMatcher struct {
	Kind        string       `json:"kind"`
	Side        string       `json:"side,omitempty"`
	InstanceID  string       `json:"instanceId,omitempty"`
	From        string       `json:"from,omitempty"`
	To          string       `json:"to,omitempty"`
	Reason      string       `json:"reason,omitempty"`
	Destination string       `json:"destination,omitempty"`
	Attacker    *EventTarget `json:"attacker,omitempty"`
	Defender    *EventTarget `json:"defender,omitempty"`
	Target      *EventTarget `json:"target,omitempty"`
	Subject     *EventTarget `json:"subject,omitempty"`
	Actual      int          `json:"actual,omitempty"`
	Count       int          `json:"count,omitempty"`
	CardID      int          `json:"cardId,omitempty"`
	Amount      int          `json:"amount,omitempty"`
	Resource    string       `json:"resource,omitempty"`
	Direction   string       `json:"direction,omitempty"`
}
type EventTarget struct {
	Kind       string `json:"kind"`
	Side       string `json:"side,omitempty"`
	InstanceID string `json:"instanceId,omitempty"`
	CardID     int    `json:"cardId,omitempty"`
}

type RuntimeEvent struct {
	Kind, Side, InstanceID string
	CardID, Count, Actual  int
	Target, Subject        *EventTarget
	Sequence, BatchID      uint64
}

// rawKind is shared by all context-specific tagged-union decoders.
type rawKind struct {
	Kind string `json:"kind"`
}
type rawObject map[string]json.RawMessage
