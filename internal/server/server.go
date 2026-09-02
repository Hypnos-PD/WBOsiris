package server

import (
	"encoding/json"
	"fmt"
	"net/http"
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
	mu          sync.Mutex
	nextSession uint64
}

type command struct {
	Kind                string   `json:"kind"`
	Actor               string   `json:"actor,omitempty"`
	Source              string   `json:"source,omitempty"`
	Defender            string   `json:"defender,omitempty"`
	RequestID           string   `json:"requestId,omitempty"`
	ActionID            string   `json:"actionId,omitempty"`
	StateRevision       uint64   `json:"stateRevision,omitempty"`
	SelectedInstanceIDs []string `json:"selectedInstanceIds,omitempty"`
	SelectedOptionID    int      `json:"selectedOptionId,omitempty"`
}

type createRequest struct {
	Scenario string `json:"scenario"`
}

type response struct {
	SessionID    string                       `json:"sessionId"`
	Result       *runner.StepResult           `json:"result,omitempty"`
	State        runner.StateView             `json:"state"`
	LegalActions []runner.LegalAction         `json:"legalActions"`
	Capabilities runner.SimulatorCapabilities `json:"capabilities"`
	Events       []ir.RuntimeEvent            `json:"events"`
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
	return &Server{cards: cards, tests: tests, sessions: map[string]*runner.Session{}}, nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", s.health)
	mux.HandleFunc("/api/scenarios", s.scenarios)
	mux.HandleFunc("/api/sessions", s.sessionsHandler)
	mux.HandleFunc("/api/sessions/", s.sessionHandler)
	return withCORS(mux)
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
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
	s.mu.Lock()
	s.nextSession++
	id := fmt.Sprintf("session-%d", s.nextSession)
	s.sessions[id] = session
	s.mu.Unlock()
	writeSession(w, id, session, nil)
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
	state, err := session.View("own")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, response{SessionID: id, Result: result, State: state, LegalActions: session.LegalActions(), Capabilities: runner.SupportedSimulatorCapabilities(), Events: session.Events()})
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(value)
}
func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
