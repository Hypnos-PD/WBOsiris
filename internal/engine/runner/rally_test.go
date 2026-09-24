package runner

import (
	"strings"
	"testing"

	"wbo/internal/engine/ir"
)

// 协作（Rally）统计进入过自己战场的随从：打出的随从在本次结算结束后才计入，
// 能力召唤的随从立即计入，法术不计入。
func TestRallyCountsFollowerEntries(t *testing.T) {
	const follower, spell = 70000001, 70000002
	pack := &ir.CardPack{Cards: []ir.Card{
		{ID: follower, CardType: "follower", Cost: 1, Stats: &ir.Stats{Attack: 1, Life: 1}},
		{ID: spell, CardType: "spell", Cost: 1},
	}}
	followerID, spellID := strings.Repeat("1", 32), strings.Repeat("2", 32)
	state := testState()
	own := state.Players["own"]
	own.PP, own.MaxPP = 10, 10
	own = withInstance(own, "hand", ir.TestInstance{InstanceID: followerID, CardID: follower, DeclaredType: "follower"})
	own = withInstance(own, "hand", ir.TestInstance{InstanceID: spellID, CardID: spell, DeclaredType: "spell"})
	state.Players["own"] = own
	session, err := NewSession(pack, state, 1)
	if err != nil {
		t.Fatal(err)
	}
	if result := session.Begin(strings.Repeat("a", 32), ir.SourceAction{Kind: "play", Actor: "own", Source: spellID}); result.Status != StatusCompleted {
		t.Fatalf("spell play failed: %#v", result)
	}
	if session.g.own.rally != 0 {
		t.Fatalf("spell counted toward rally: %d", session.g.own.rally)
	}
	if result := session.Begin(strings.Repeat("b", 32), ir.SourceAction{Kind: "play", Actor: "own", Source: followerID}); result.Status != StatusCompleted {
		t.Fatalf("follower play failed: %#v", result)
	}
	if session.g.own.rally != 1 {
		t.Fatalf("played follower was not credited: %d", session.g.own.rally)
	}
	before := session.g.own.rally
	session.g.summonFor(session.g.instances[followerID], "own", 1, follower, false)
	if session.g.own.rally != before+1 {
		t.Fatalf("summoned follower was not credited immediately: %d", session.g.own.rally)
	}
}
