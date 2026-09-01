package runner

import (
	"math"

	"wbo/internal/ruleset"
)

const executionBudgetExceeded = "execution_budget_exceeded"

type ExecutionBudgetState struct {
	Instructions     uint64 `json:"instructions"`
	QueryVisits      uint64 `json:"queryVisits"`
	MaxStackDepth    uint64 `json:"maxStackDepth"`
	Candidates       uint64 `json:"candidates"`
	Events           uint64 `json:"events"`
	Triggers         uint64 `json:"triggers"`
	CreatedInstances uint64 `json:"createdInstances"`
}

type budgetTracker struct {
	policy   ruleset.ExecutionBudget
	state    ExecutionBudgetState
	exceeded bool
}

func (b *budgetTracker) reset(policy ruleset.ExecutionBudget) {
	b.policy = policy
	b.state = ExecutionBudgetState{}
	b.exceeded = false
}

func (b *budgetTracker) chargeInstructions(amount uint64) bool {
	return b.charge(&b.state.Instructions, amount, b.policy.Instructions)
}

func (b *budgetTracker) chargeQueryVisits(amount uint64) bool {
	return b.charge(&b.state.QueryVisits, amount, b.policy.QueryVisits)
}

func (b *budgetTracker) chargeCandidates(amount uint64) bool {
	return b.charge(&b.state.Candidates, amount, b.policy.Candidates)
}

func (b *budgetTracker) chargeEvents(amount uint64) bool {
	return b.charge(&b.state.Events, amount, b.policy.Events)
}

func (b *budgetTracker) chargeTriggers(amount uint64) bool {
	return b.charge(&b.state.Triggers, amount, b.policy.Triggers)
}

func (b *budgetTracker) chargeCreatedInstances(amount uint64) bool {
	return b.charge(&b.state.CreatedInstances, amount, b.policy.CreatedInstances)
}

func (b *budgetTracker) observeStack(depth int) bool {
	if depth < 0 {
		b.exceeded = true
		return false
	}
	value := uint64(depth)
	if value > b.policy.StackDepth {
		b.exceeded = true
		return false
	}
	if value > b.state.MaxStackDepth {
		b.state.MaxStackDepth = value
	}
	return true
}

func (b *budgetTracker) charge(value *uint64, amount, limit uint64) bool {
	if *value > limit || amount > math.MaxUint64-*value || amount > limit-*value {
		b.exceeded = true
		return false
	}
	*value += amount
	return true
}

func (b *budgetTracker) validState(state ExecutionBudgetState) bool {
	return state.Instructions <= b.policy.Instructions &&
		state.QueryVisits <= b.policy.QueryVisits &&
		state.MaxStackDepth <= b.policy.StackDepth &&
		state.Candidates <= b.policy.Candidates &&
		state.Events <= b.policy.Events &&
		state.Triggers <= b.policy.Triggers &&
		state.CreatedInstances <= b.policy.CreatedInstances
}
