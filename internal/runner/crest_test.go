package runner

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"wbo/internal/ir"
)

func crestState(owner string) ir.State {
	state := testState()
	state.Turn.Active, state.Turn.Number = owner, 7
	for side, p := range state.Players {
		p.PP, p.MaxPP, p.EP, p.SEP = 10, 10, 1, 1
		for n := 0; n < 12; n++ {
			p = withInstance(p, "deck", ir.TestInstance{InstanceID: fmt.Sprintf("%032x", 100+n+len(side)*100), CardID: 10001110, DeclaredType: "follower"})
		}
		state.Players[side] = p
	}
	return state
}

func crestEndTurn(t *testing.T, s *Session) StepResult {
	t.Helper()
	r := s.SubmitAs(fmt.Sprintf("%032x", s.g.revision+1), s.g.turn.Active, SimulatorCommand{Kind: "end_turn"})
	if r.Status != StatusCompleted && r.Status != StatusSuspended {
		t.Fatal(r)
	}
	return r
}

func TestAriaCrestPersistsAndGrantsStormOnlyToAlliedPixies(t *testing.T) {
	pack := repeatCardPack(t)
	for _, owner := range []string{"own", "oppo"} {
		t.Run(owner, func(t *testing.T) {
			state := crestState(owner)
			id := strings.Repeat("a", 32)
			state.Players[owner] = withInstance(state.Players[owner], "hand", ir.TestInstance{InstanceID: id, CardID: 10114110, DeclaredType: "follower"})
			s, err := NewSession(pack, state, 1)
			if err != nil {
				t.Fatal(err)
			}
			if r := s.SubmitAs(strings.Repeat("b", 32), owner, SimulatorCommand{Kind: "play", Source: id}); r.Status != StatusCompleted {
				t.Fatal(r)
			}
			p := s.g.player(owner)
			if len(p.crests) != 1 || p.crests[0].id == id {
				t.Fatal("crest identity missing")
			}
			if r := s.SubmitAs(strings.Repeat("c", 32), owner, SimulatorCommand{Kind: "evolve", Source: id}); r.Status != StatusCompleted {
				t.Fatal(r)
			}
			for _, fairy := range p.field[1:] {
				if fairy.card.ID != 90011110 || !fairy.abilities["storm"] {
					t.Fatal("Aria evolution did not grant Storm")
				}
			}
			s.g.resolveDeathBatch([]*instance{s.g.instances[id]})
			if r := s.run(); r.Status != StatusCompleted {
				t.Fatal(r)
			}
			own := s.g.summonFor(nil, owner, 1, 90011110, false)
			enemy := s.g.summonFor(nil, oppositeSide(owner), 1, 90011110, false)
			other := s.g.summonFor(nil, oppositeSide(owner), 1, 10001110, false)
			if r := s.run(); r.Status != StatusCompleted {
				t.Fatal(r)
			}
			if !own[0].abilities["storm"] || enemy[0].abilities["storm"] || other[0].abilities["storm"] || len(p.crests) != 1 {
				t.Fatal("crest ownership or lifetime")
			}
			for _, viewer := range []string{"own", "oppo"} {
				v, err := s.View(viewer)
				if err != nil {
					t.Fatal(err)
				}
				visible := v.Own.Crests
				if viewer != owner {
					visible = v.Oppo.Crests
				}
				if len(visible) != 1 || visible[0].CardType != "crest" || len(visible[0].CrestLocales) != 5 {
					t.Fatal("public crest metadata missing")
				}
			}
		})
	}
}

func TestBaltoCrestCountdownDuplicateAndLeaderDamage(t *testing.T) {
	pack := repeatCardPack(t)
	for _, owner := range []string{"own", "oppo"} {
		s, err := NewSession(pack, crestState(owner), 1)
		if err != nil {
			t.Fatal(err)
		}
		s.g.gainCrest(nil, owner, 10153140)
		p := s.g.player(owner)
		crest := p.crests[0]
		for remaining := 4; remaining > 0; remaining-- {
			if crest.countdown != remaining {
				t.Fatal("countdown drift")
			}
			s.g.gainCrest(nil, owner, 10153140)
			if len(p.crests) != 1 || crest.countdown != remaining {
				t.Fatal("duplicate refreshed crest")
			}
			crestEndTurn(t, s)
			if crest.countdown != remaining || s.g.player("own").leaderLife != 16+remaining-1 || s.g.player("oppo").leaderLife != 16+remaining-1 {
				t.Fatal("wrong end turn damage or opponent tick")
			}
			crestEndTurn(t, s)
		}
		if len(p.crests) != 0 || len(p.retiredCrests) != 1 || p.shadows != 0 || len(p.destroyed) != 0 || len(p.graveyard) != 0 {
			t.Fatal("crest contaminated card zones")
		}
		s.g.gainCrest(nil, owner, 10153140)
		if len(p.crests) != 1 || p.crests[0].id == crest.id || p.crests[0].countdown != 4 {
			t.Fatal("expired crest cannot be regained")
		}
	}
}

func TestCrestLastWordsRecreatesFollowerAndRetainsOwner(t *testing.T) {
	pack := repeatCardPack(t)
	for _, owner := range []string{"own", "oppo"} {
		for _, card := range []int{10124130, 10163130} {
			t.Run(fmt.Sprintf("%s/%d", owner, card), func(t *testing.T) {
				state := crestState(owner)
				id := strings.Repeat("a", 32)
				state.Players[owner] = withInstance(state.Players[owner], "field", ir.TestInstance{InstanceID: id, CardID: card, DeclaredType: "follower"})
				s, err := NewSession(pack, state, 1)
				if err != nil {
					t.Fatal(err)
				}
				s.g.resolveDeathBatch([]*instance{s.g.instances[id]})
				if r := s.run(); r.Status != StatusCompleted {
					t.Fatal(r)
				}
				p := s.g.player(owner)
				if len(p.crests) != 1 || p.crests[0].countdown != 2 {
					t.Fatal("missing Last Words crest")
				}
				for n := 0; n < 4; n++ {
					crestEndTurn(t, s)
				}
				if len(p.crests) != 0 || len(p.field) != 1 || p.field[0].card.ID != card || p.field[0].abilities["storm"] != (card == 10163130) || p.shadows != 1 || len(p.destroyed) != 1 {
					t.Fatal("wrong crest resurrection")
				}
				if p.field[0].id == id || p.field[0].evolved || p.field[0].departed {
					t.Fatal("resurrection reused old state")
				}
			})
		}
	}
}

func TestCrestLimitIsSeparateFromFieldAndPerPlayer(t *testing.T) {
	pack := repeatCardPack(t)
	for n := 0; n < 6; n++ {
		pack.Cards = append(pack.Cards, ir.Card{ID: 77778000 + n, CardType: "follower", Crest: &ir.CrestDefinition{}})
	}
	s, err := NewSession(pack, crestState("own"), 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, owner := range []string{"own", "oppo"} {
		s.g.summonFor(nil, owner, 5, 10001110, false)
		for n := 0; n < 6; n++ {
			s.g.gainCrest(nil, owner, 77778000+n)
		}
		p := s.g.player(owner)
		if len(p.crests) != 5 || len(p.field) != 5 {
			t.Fatal("crest cap shared with field or opponent")
		}
	}
	before := s.g.snapshot()
	s.g.gainCrest(nil, "own", 77778000)
	if !reflect.DeepEqual(before, s.g.snapshot()) {
		t.Fatal("duplicate at cap mutated state")
	}
}

func TestCrestContinuationDuringLastWordsAndTurnStart(t *testing.T) {
	for _, lastwords := range []bool{false, true} {
		pack := repeatCardPack(t)
		for n := range pack.Cards {
			if pack.Cards[n].ID != 10163130 {
				continue
			}
			crest := pack.Cards[n].Crest
			if !lastwords {
				crest.Countdown = 0
				crest.Abilities[0].Trigger = ir.EventTrigger{Kind: "event", Event: "turn_started", Side: "own"}
			}
			pause := ir.ModeEffect{NodeBase: ir.NodeBase{ID: strings.Repeat("f", 32)}, Kind: "mode", Options: []ir.ModeOption{{ID: 1, Body: []ir.Effect{}}}}
			crest.Abilities[0].Body = append([]ir.Effect{pause}, crest.Abilities[0].Body...)
		}
		for _, owner := range []string{"own", "oppo"} {
			state := crestState(oppositeSide(owner))
			decl := ir.TestInstance{InstanceID: strings.Repeat("a", 32), CardID: 10163130, DeclaredType: "crest"}
			if lastwords {
				one := 1
				decl.Overrides.Countdown = &one
			}
			state.Players[owner] = withInstance(state.Players[owner], "crests", decl)
			s, err := NewSession(pack, state, 1)
			if err != nil {
				t.Fatal(err)
			}
			step := crestEndTurn(t, s)
			if step.Status != StatusSuspended || s.g.turnTransition != "starting_crests" || len(s.g.player(owner).hand) != 0 {
				t.Fatal("crest must precede draw", step)
			}
			data, err := s.EncodeContinuation()
			if err != nil {
				t.Fatal(err)
			}
			checkpoint, err := DecodeContinuation(data)
			if err != nil {
				t.Fatal(err)
			}
			restored, err := RestoreSession(pack, checkpoint)
			if err != nil {
				t.Fatal(err)
			}
			response := ChoiceResponse{RequestID: step.Choice.RequestID, ActionID: step.Choice.ActionID, StateRevision: step.Choice.StateRevision, SelectedOptionID: 1}
			for _, current := range []*Session{s, restored} {
				if r := current.Resume(response); r.Status != StatusCompleted {
					t.Fatal(r)
				}
				p := current.g.player(owner)
				if len(p.hand) != 1 || len(p.field) != 1 || !p.field[0].abilities["storm"] || p.shadows != 0 {
					t.Fatal("restored crest resolved incorrectly")
				}
			}
			if !reflect.DeepEqual(s.g.snapshot(), restored.g.snapshot()) {
				t.Fatal("crest continuation diverged")
			}
			for _, mutate := range []func(*ContinuationEntity){
				func(e *ContinuationEntity) { e.Crest = false }, func(e *ContinuationEntity) { e.Countdown = 3 },
				func(e *ContinuationEntity) { e.Cost = 1 }, func(e *ContinuationEntity) { e.AttackLimit = 2 },
			} {
				bad, err := DecodeContinuation(data)
				if err != nil {
					t.Fatal(err)
				}
				for n := range bad.Game.Instances {
					if bad.Game.Instances[n].Crest {
						mutate(&bad.Game.Instances[n])
						break
					}
				}
				if _, err := RestoreSession(pack, bad); err == nil {
					t.Fatal("restored malformed crest")
				}
			}
		}
	}
}

func TestRejectInvalidInitialCrests(t *testing.T) {
	pack := repeatCardPack(t)
	zero, one, three := 0, 1, 3
	for _, o := range []ir.InstanceOverrides{{Cost: &zero}, {Stats: &ir.Stats{}}, {Keywords: []string{"storm"}}, {Countdown: &zero}, {Countdown: &three}, {Earthsigil: &one}} {
		state := crestState("own")
		state.Players["own"] = withInstance(state.Players["own"], "crests", ir.TestInstance{InstanceID: strings.Repeat("a", 32), CardID: 10163130, DeclaredType: "crest", Overrides: o})
		if _, err := NewSession(pack, state, 1); err == nil {
			t.Fatal("accepted invalid crest override", o)
		}
	}
}

func TestCrestTurnAbilitiesFollowAcquisitionOrderBeforeFieldAndDraw(t *testing.T) {
	for _, event := range []string{"turn_started", "turn_ended"} {
		pack := repeatCardPack(t)
		for n := range pack.Cards {
			c := &pack.Cards[n]
			amount := 0
			if c.ID == 10114110 {
				amount = 1
			}
			if c.ID == 10153140 {
				amount = 2
			}
			if c.ID == 10001110 {
				amount = 3
			}
			if amount == 0 {
				continue
			}
			ability := ir.Ability{ID: fmt.Sprintf("%032x", 900+amount), Trigger: ir.EventTrigger{Kind: "event", Event: event, Side: "own"}, Body: []ir.Effect{
				ir.TargetEffect{NodeBase: ir.NodeBase{ID: fmt.Sprintf("%032x", 910+amount)}, Kind: "damage", Target: ir.LeaderRef{Kind: "leader", Side: "oppo"}, Amount: amount},
			}}
			if c.Crest != nil {
				c.Crest.Countdown = 0
				c.Crest.Abilities = []ir.Ability{ability}
			} else {
				c.Abilities = []ir.Ability{ability}
			}
		}
		for _, reverse := range []bool{false, true} {
			state := crestState("own")
			owner := "own"
			if event == "turn_started" {
				owner = "oppo"
			}
			state.Players[owner] = withInstance(state.Players[owner], "field", ir.TestInstance{InstanceID: strings.Repeat("a", 32), CardID: 10001110, DeclaredType: "follower"})
			s, err := NewSession(pack, state, 1)
			if err != nil {
				t.Fatal(err)
			}
			cards, want := []int{10114110, 10153140}, []int{1, 2, 3}
			if reverse {
				cards, want = []int{10153140, 10114110}, []int{2, 1, 3}
			}
			for _, id := range cards {
				s.g.gainCrest(nil, owner, id)
			}
			crestEndTurn(t, s)
			var got []int
			for _, e := range s.g.events {
				if e.Kind == "damaged" {
					got = append(got, e.Actual)
				}
				if e.Kind == "card_drawn" && len(got) < 2 {
					t.Fatal("rule draw preceded crests")
				}
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatal(event, got, want)
			}
		}
	}
}

func TestCrestCreationHonorsExecutionBudget(t *testing.T) {
	pack := repeatCardPack(t)
	s, err := NewSession(pack, crestState("own"), 1)
	if err != nil {
		t.Fatal(err)
	}
	s.ensureBudget()
	s.budget.policy.CreatedInstances = 0
	s.g.gainCrest(nil, "own", 10114110)
	if !s.budget.exceeded || len(s.g.own.crests) != 0 || len(s.g.events) != 0 {
		t.Fatal("crest creation bypassed budget")
	}
}

func TestCrestSourceCannotMoveIntoCardZonesOrTransform(t *testing.T) {
	pack := repeatCardPack(t)
	s, err := NewSession(pack, crestState("own"), 1)
	if err != nil {
		t.Fatal(err)
	}
	s.g.gainCrest(nil, "oppo", 10153140)
	crest := s.g.oppo.crests[0]
	before := s.g.snapshot()
	self := ir.SelfRef{Kind: "self", ValueType: "entity"}
	s.g.execTargetEffect(ir.TargetEffect{Kind: "return", Target: self, Destination: "hand"}, crest, frame{})
	s.g.execTargetEffect(ir.TargetEffect{Kind: "banish", Target: self}, crest, frame{})
	s.g.execCardEffect(ir.CardEffect{Kind: "transform", Target: self, CardID: 10001110, PreserveInstanceID: true, PreserveMaterials: true}, crest, frame{})
	if !reflect.DeepEqual(before, s.g.snapshot()) {
		t.Fatal("crest was treated as an ordinary card")
	}
}
