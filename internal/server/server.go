package server

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"

	"wbo/internal/ir"
	"wbo/internal/project"
	"wbo/internal/runner"
)

type Server struct {
	cards       *ir.CardPack
	tests       *ir.TestPack
	sessions    map[string]*runner.Session
	matches     map[string]*match
	mu          sync.Mutex
	sessionMu   sync.Mutex
	nextSession uint64
}

type match struct {
	session           *runner.Session
	ownDeck           []int
	players           map[string]string
	joinCode          string
	joined            bool
	started           bool
	mulligan          map[string]bool
	mulliganSelection map[string][]string
	mu                sync.Mutex
	replay            []replayFrame
}

type replayFrame struct {
	Revision   uint64
	EventCount int
	Own        runner.StateView
	Oppo       runner.StateView
}

type replayViewFrame struct {
	Revision   uint64           `json:"revision"`
	EventCount int              `json:"eventCount"`
	State      runner.StateView `json:"state"`
}

type command struct {
	Kind                string   `json:"kind"`
	Actor               string   `json:"actor,omitempty"`
	Source              string   `json:"source,omitempty"`
	Defender            string   `json:"defender,omitempty"`
	RequestID           string   `json:"requestId,omitempty"`
	ActionID            string   `json:"actionId,omitempty"`
	StateRevision       uint64   `json:"stateRevision,omitempty"`
	ExpectedRevision    *uint64  `json:"expectedRevision,omitempty"`
	SelectedInstanceIDs []string `json:"selectedInstanceIds,omitempty"`
	SelectedOptionID    int      `json:"selectedOptionId,omitempty"`
}

type createRequest struct {
	Scenario string `json:"scenario"`
	Deck     []int  `json:"deck,omitempty"`
}

type response struct {
	SessionID     string                       `json:"sessionId"`
	MatchID       string                       `json:"matchId,omitempty"`
	PlayerToken   string                       `json:"playerToken,omitempty"`
	JoinCode      string                       `json:"joinCode,omitempty"`
	Side          string                       `json:"side,omitempty"`
	Waiting       bool                         `json:"waiting,omitempty"`
	MatchPhase    string                       `json:"matchPhase,omitempty"`
	MulliganReady bool                         `json:"mulliganReady,omitempty"`
	OpponentReady bool                         `json:"opponentReady,omitempty"`
	Result        *runner.StepResult           `json:"result,omitempty"`
	State         runner.StateView             `json:"state"`
	LegalActions  []runner.LegalAction         `json:"legalActions"`
	Capabilities  runner.SimulatorCapabilities `json:"capabilities"`
	Events        []ir.RuntimeEvent            `json:"events"`
}

type replayResponse struct {
	MatchID string            `json:"matchId"`
	Events  []ir.RuntimeEvent `json:"events"`
	State   runner.StateView  `json:"state"`
	Winner  string            `json:"winner,omitempty"`
	Frames  []replayViewFrame `json:"frames"`
}

func New(root string, paths []string) (*Server, error) {
	loaded := project.LoadWithRoot(paths, true, root)
	if loaded.HasErrors() {
		return nil, fmt.Errorf("load simulator sources: %v", loaded.Diagnostics)
	}
	cards, tests, err := project.BuildRuntimePacks(loaded)
	if err != nil {
		return nil, err
	}
	return &Server{cards: cards, tests: tests, sessions: map[string]*runner.Session{}, matches: map[string]*match{}}, nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", s.health)
	mux.HandleFunc("/api/cards", s.cardCatalog)
	mux.HandleFunc("/api/scenarios", s.scenarios)
	mux.HandleFunc("/api/sessions", s.sessionsHandler)
	mux.HandleFunc("/api/sessions/", s.sessionHandler)
	mux.HandleFunc("/api/matches", s.matchesHandler)
	mux.HandleFunc("/api/matches/", s.matchHandler)
	return withCORS(mux)
}

func (s *Server) matchesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		w.Header().Set("Cache-Control", "no-store")
		type roomSummary struct {
			ID      string `json:"id"`
			Waiting bool   `json:"waiting"`
		}
		s.mu.Lock()
		items := make([]roomSummary, 0, len(s.matches))
		for id, room := range s.matches {
			room.mu.Lock()
			items = append(items, roomSummary{ID: id, Waiting: !room.joined})
			room.mu.Unlock()
		}
		s.mu.Unlock()
		sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
		writeJSON(w, items)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var input createRequest
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
	}
	deck := practiceDeck()
	if input.Deck != nil {
		if err := validateDeck(s.cards, input.Deck); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		deck = input.Deck
	}
	session, err := runner.NewMatchSession(s.cards, deck, practiceDeck(), 1)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	id, token, joinCode := randomID(6), randomID(24), randomID(8)
	room := &match{session: session, ownDeck: append([]int(nil), deck...), players: map[string]string{token: "own"}, joinCode: joinCode, mulligan: map[string]bool{}, mulliganSelection: map[string][]string{}}
	recordReplay(room)
	s.mu.Lock()
	for s.matches[id] != nil {
		id = randomID(6)
	}
	s.matches[id] = room
	s.mu.Unlock()
	writeMatch(w, id, token, joinCode, "own", room, nil)
}

func validateDeck(cards *ir.CardPack, deck []int) error {
	return runner.ValidateMatchDeck(cards, deck)
}

func practiceDeck() []int {
	ids := []int{10001110, 10001120, 10001130, 10001210, 10002110, 10002120, 10002210, 10011110, 10011120, 10011130, 10011210, 10012110, 10012120, 10012310}
	deck := make([]int, 0, 40)
	for len(deck) < 40 {
		for _, id := range ids {
			if len(deck) == 40 {
				break
			}
			deck = append(deck, id)
			if len(deck)%3 == 0 {
				continue
			}
		}
	}
	return deck
}

func simulationStateWithDeck(state ir.State, cards *ir.CardPack, deck []int) ir.State {
	player := state.Players["own"]
	for _, zone := range []string{"hand", "deck"} {
		player.Zones[zone] = nil
	}
	for n, id := range deck {
		typeName := "spell"
		if card := cardByID(cards, id); card != nil {
			typeName = card.CardType
		}
		instance := ir.TestInstance{InstanceID: fmt.Sprintf("%032x", 1000+n), Alias: fmt.Sprintf("own_deck_%d", n), CardID: id, DeclaredType: typeName}
		if n < 4 {
			player.Zones["hand"] = append(player.Zones["hand"], instance)
		} else {
			player.Zones["deck"] = append(player.Zones["deck"], instance)
		}
	}
	state.Players["own"] = player
	return state
}

func cardByID(cards *ir.CardPack, id int) *ir.Card {
	for _, card := range cards.Cards {
		if card.ID == id {
			return &card
		}
	}
	return nil
}

func (s *Server) matchHandler(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/matches/"), "/"), "/")
	if len(parts) < 1 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	s.mu.Lock()
	room := s.matches[parts[0]]
	s.mu.Unlock()
	if room == nil {
		http.NotFound(w, r)
		return
	}
	room.mu.Lock()
	defer room.mu.Unlock()
	if len(parts) == 2 && parts[1] == "replay" && r.Method == http.MethodGet {
		token := bearerToken(r)
		side, ok := room.players[token]
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		state, err := room.session.View(side)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		frames := make([]replayViewFrame, 0, len(room.replay))
		for _, frame := range room.replay {
			frameState := frame.Own
			if side == "oppo" {
				frameState = frame.Oppo
			}
			frames = append(frames, replayViewFrame{Revision: frame.Revision, EventCount: frame.EventCount, State: frameState})
		}
		writeJSON(w, replayResponse{MatchID: parts[0], Events: room.session.EventsFor(side), State: state, Winner: state.Winner, Frames: frames})
		return
	}
	if len(parts) == 2 && parts[1] == "join" && r.Method == http.MethodPost {
		if r.URL.Query().Get("code") != room.joinCode {
			http.Error(w, "invalid join code", http.StatusUnauthorized)
			return
		}
		if room.joined {
			http.Error(w, "match is full", http.StatusConflict)
			return
		}
		var input createRequest
		if r.Body != nil {
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil && err != io.EOF {
				http.Error(w, "invalid request", http.StatusBadRequest)
				return
			}
		}
		if input.Deck != nil {
			if err := validateDeck(s.cards, input.Deck); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			session, err := runner.NewMatchSession(s.cards, room.ownDeck, input.Deck, 1)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			room.session = session
			recordReplay(room)
		}
		token := randomID(24)
		room.players[token], room.joined = "oppo", true
		writeMatch(w, parts[0], token, "", "oppo", room, nil)
		return
	}
	if len(parts) == 2 && parts[1] == "mulligan" && r.Method == http.MethodPost {
		token := bearerToken(r)
		side, ok := room.players[token]
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if !room.joined || room.started || room.mulligan[side] {
			http.Error(w, "mulligan unavailable", http.StatusConflict)
			return
		}
		var input command
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		view, _ := room.session.View(side)
		if input.ExpectedRevision != nil && *input.ExpectedRevision != view.Revision {
			result := runner.StepResult{Status: runner.StatusRejected, ErrorCode: "stale_state"}
			writeMatch(w, parts[0], "", "", side, room, &result)
			return
		}
		if err := room.session.ValidateMulligan(side, input.SelectedInstanceIDs); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		room.mulliganSelection[side] = append([]string(nil), input.SelectedInstanceIDs...)
		room.mulligan[side] = true
		room.started = room.mulligan["own"] && room.mulligan["oppo"]
		if room.started {
			if err := room.session.Mulligan("own", room.mulliganSelection["own"]); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if err := room.session.Mulligan("oppo", room.mulliganSelection["oppo"]); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			room.session.StartMatch()
			recordReplay(room)
		}
		writeMatch(w, parts[0], "", "", side, room, nil)
		return
	}
	if len(parts) != 1 {
		http.NotFound(w, r)
		return
	}
	token := bearerToken(r)
	side, ok := room.players[token]
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if r.Method == http.MethodGet {
		writeMatch(w, parts[0], "", "", side, room, nil)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !room.joined {
		http.Error(w, "waiting for player", http.StatusConflict)
		return
	}
	if !room.started {
		http.Error(w, "mulligan in progress", http.StatusConflict)
		return
	}
	var input command
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	input.Actor = side
	view, _ := room.session.View(side)
	if input.ExpectedRevision != nil && *input.ExpectedRevision != view.Revision {
		result := runner.StepResult{Status: runner.StatusRejected, ErrorCode: "stale_state"}
		writeMatch(w, parts[0], "", "", side, room, &result)
		return
	}
	if room.session.PendingChoice() != nil && view.PendingChoice == nil {
		http.Error(w, "choice belongs to opponent", http.StatusConflict)
		return
	}
	result := submit(room.session, input)
	recordReplay(room)
	writeMatch(w, parts[0], "", "", side, room, &result)
}

func recordReplay(room *match) {
	if room == nil || room.session == nil {
		return
	}
	own, ownErr := room.session.View("own")
	oppo, oppoErr := room.session.View("oppo")
	if ownErr != nil || oppoErr != nil {
		return
	}
	frame := replayFrame{Revision: own.Revision, EventCount: len(room.session.Events()), Own: own, Oppo: oppo}
	if n := len(room.replay); n > 0 && room.replay[n-1].Revision == own.Revision {
		room.replay[n-1] = frame
		return
	}
	room.replay = append(room.replay, frame)
}

func writeMatch(w http.ResponseWriter, id, token, joinCode, side string, room *match, result *runner.StepResult) {
	w.Header().Set("Cache-Control", "no-store")
	state, err := room.session.View(side)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	phase := "mulligan"
	if room.started {
		phase = "main"
	}
	actions := []runner.LegalAction{}
	if room.started {
		actions = room.session.LegalActionsFor(side)
	}
	if result != nil && result.Choice != nil {
		visible := *result
		visible.Choice = state.PendingChoice
		result = &visible
	}
	writeJSON(w, response{MatchID: id, PlayerToken: token, JoinCode: joinCode, Side: side, Waiting: !room.joined, MatchPhase: phase, MulliganReady: room.mulligan[side], OpponentReady: room.mulligan[oppositeMatchSide(side)], Result: result, State: state, LegalActions: actions, Capabilities: runner.SupportedSimulatorCapabilities(), Events: room.session.EventsFor(side)})
}

func oppositeMatchSide(side string) string {
	if side == "own" {
		return "oppo"
	}
	return "own"
}

func bearerToken(r *http.Request) string {
	return strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
}

func randomID(bytes int) string {
	buffer := make([]byte, bytes)
	if _, err := rand.Read(buffer); err != nil {
		panic(err)
	}
	return fmt.Sprintf("%x", buffer)
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, map[string]any{"ok": true, "ruleset": "wbo-standard-0.3.0"})
}

func (s *Server) scenarios(w http.ResponseWriter, _ *http.Request) {
	items := make([]map[string]string, 0, len(s.tests.Scenarios))
	for _, item := range s.tests.Scenarios {
		items = append(items, map[string]string{"id": item.ID, "name": item.Name})
	}
	writeJSON(w, items)
}

func (s *Server) sessionsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var input createRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	var scenario *ir.Scenario
	for n := range s.tests.Scenarios {
		if s.tests.Scenarios[n].Name == input.Scenario || s.tests.Scenarios[n].ID == input.Scenario {
			scenario = &s.tests.Scenarios[n]
			break
		}
	}
	if scenario == nil {
		http.Error(w, "scenario not found", http.StatusNotFound)
		return
	}
	seed, err := strconv.ParseUint(strings.TrimPrefix(scenario.Seed, "0x"), 16, 64)
	if err != nil {
		http.Error(w, "invalid scenario seed", http.StatusInternalServerError)
		return
	}
	session, err := runner.NewSession(s.cards, scenario.InitialState, seed)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.writeNewSession(w, session)
}

func (s *Server) writeNewSession(w http.ResponseWriter, session *runner.Session) {
	s.mu.Lock()
	s.nextSession++
	id := fmt.Sprintf("session-%d", s.nextSession)
	s.sessions[id] = session
	s.mu.Unlock()
	writeSession(w, id, session, nil)
}

func simulationState() ir.State {
	instance := func(id int, alias string, cardID int) ir.TestInstance {
		return ir.TestInstance{InstanceID: fmt.Sprintf("%032x", id), Alias: alias, CardID: cardID, DeclaredType: "follower"}
	}
	zones := func(side string, handStart, deckStart int) map[string][]ir.TestInstance {
		cards := []int{10012110, 10001120, 10002110, 10011130}
		hand := make([]ir.TestInstance, 0, 4)
		for n := 0; n < 4; n++ {
			hand = append(hand, instance(handStart+n, fmt.Sprintf("%s_hand_%d", side, n), cards[n]))
		}
		deck := make([]ir.TestInstance, 0, 36)
		for n := 0; n < 36; n++ {
			deck = append(deck, instance(deckStart+n, fmt.Sprintf("%s_deck_%d", side, n), cards[n%len(cards)]))
		}
		return map[string][]ir.TestInstance{"hand": hand, "field": {}, "deck": deck}
	}
	return ir.State{Turn: ir.Turn{Active: "own", Number: 1}, Phase: "main", FirstPlayer: "own", Players: map[string]ir.PlayerState{
		"own":  {Leader: ir.Leader{Life: 20, MaxLife: 20}, PP: 1, MaxPP: 1, EP: 2, SEP: 2, Zones: zones("own", 1, 100)},
		"oppo": {Leader: ir.Leader{Life: 20, MaxLife: 20}, PP: 0, MaxPP: 0, EP: 2, SEP: 2, ExtraPPEarly: true, ExtraPPLate: true, Zones: zones("oppo", 50, 200)},
	}, Aliases: map[string]string{}}
}

func (s *Server) sessionHandler(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/sessions/"), "/"), "/")
	if len(parts) != 1 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	s.mu.Lock()
	session := s.sessions[parts[0]]
	s.mu.Unlock()
	if session == nil {
		http.NotFound(w, r)
		return
	}
	// A runner session is intentionally single-writer: commands and views must
	// observe one complete state transition even when HTTP requests overlap.
	s.sessionMu.Lock()
	defer s.sessionMu.Unlock()
	if r.Method == http.MethodGet {
		writeSession(w, parts[0], session, nil)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var input command
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	result := submit(session, input)
	writeSession(w, parts[0], session, &result)
}

func submit(session *runner.Session, input command) runner.StepResult {
	if pending := session.PendingChoice(); pending != nil {
		request := runner.ChoiceResponse{RequestID: input.RequestID, ActionID: input.ActionID, StateRevision: input.StateRevision, SelectedInstanceIDs: input.SelectedInstanceIDs, SelectedOptionID: input.SelectedOptionID}
		if request.RequestID == "" {
			request.RequestID = pending.RequestID
		}
		if request.ActionID == "" {
			request.ActionID = pending.ActionID
		}
		if request.StateRevision == 0 {
			request.StateRevision = pending.StateRevision
		}
		return session.Resume(request)
	}
	actor := input.Actor
	if actor == "" {
		actor = "own"
	}
	if input.ActionID == "" {
		input.ActionID = fmt.Sprintf("%032x", 1)
	}
	if input.Kind == "fusion" {
		return session.SubmitAs(input.ActionID, actor, runner.SimulatorCommand{Kind: "fusion", Source: input.Source})
	}
	return session.SubmitAs(input.ActionID, actor, runner.SimulatorCommand{Kind: input.Kind, Source: input.Source, Defender: input.Defender})
}

func writeSession(w http.ResponseWriter, id string, session *runner.Session, result *runner.StepResult) {
	w.Header().Set("Cache-Control", "no-store")
	state, err := session.View("own")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, response{SessionID: id, Result: result, State: state, LegalActions: session.LegalActions(), Capabilities: runner.SupportedSimulatorCapabilities(), Events: session.EventsFor("own")})
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(value)
}
func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
