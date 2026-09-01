package ruleset

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

const DefaultID = "wbo-standard-0.2.0"

type MatchRuleset struct {
	ID              string          `json:"id"`
	RNG             RNGPolicy       `json:"rng"`
	OrderingPolicy  OrderingPolicy  `json:"orderingPolicy"`
	ExecutionBudget ExecutionBudget `json:"executionBudget"`
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

type ExecutionBudget struct {
	Instructions      uint64 `json:"instructions"`
	QueryVisits       uint64 `json:"queryVisits"`
	StackDepth        uint64 `json:"stackDepth"`
	Candidates        uint64 `json:"candidates"`
	Events            uint64 `json:"events"`
	Triggers          uint64 `json:"triggers"`
	CreatedInstances  uint64 `json:"createdInstances"`
	ContinuationBytes uint64 `json:"continuationBytes"`
}

type Dependency struct {
	MatchRuleset
	ContentHash string `json:"contentHash"`
}

func Default() MatchRuleset {
	return MatchRuleset{
		ID:  DefaultID,
		RNG: RNGPolicy{Algorithm: "splitmix64", Version: "1"},
		OrderingPolicy: OrderingPolicy{
			Collections:          "active_side_then_opposing_side;zone_order",
			ReplacementAbilities: "source_zone_order;declaration_order",
			SimultaneousTriggers: "active_side_then_opposing_side;source_zone_order;declaration_order",
			DeathBatchLastwords:  "active_side_then_opposing_side;source_zone_order;declaration_order",
		},
		ExecutionBudget: ExecutionBudget{
			Instructions: 10000, QueryVisits: 100000, StackDepth: 64, Candidates: 256,
			Events: 4096, Triggers: 512, CreatedInstances: 256, ContinuationBytes: 1 << 20,
		},
	}
}

func DefaultDependency() Dependency {
	r := Default()
	b, _ := json.Marshal(r)
	h := sha256.Sum256(b)
	return Dependency{MatchRuleset: r, ContentHash: "sha256:" + hex.EncodeToString(h[:])}
}

// RNG implements SplitMix64. Consumed counts rule decisions, not internal words.
type RNG struct {
	state    uint64
	consumed uint64
}

type RNGState struct {
	State    uint64
	Consumed uint64
}

func NewRNG(seed uint64) *RNG { return &RNG{state: seed} }

func (r *RNG) Consumed() uint64 { return r.consumed }
func (r *RNG) Snapshot() RNGState {
	return RNGState{State: r.state, Consumed: r.consumed}
}
func (r *RNG) Restore(s RNGState) { r.state, r.consumed = s.State, s.Consumed }
func (r *RNG) Clone() *RNG {
	clone := &RNG{}
	clone.Restore(r.Snapshot())
	return clone
}

func (r *RNG) Index(n int) int {
	if n <= 0 {
		panic("ruleset.RNG.Index called with an empty range")
	}
	r.consumed++
	r.state += 0x9e3779b97f4a7c15
	z := r.state
	z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
	z = (z ^ (z >> 27)) * 0x94d049bb133111eb
	z ^= z >> 31
	return int(z % uint64(n))
}
