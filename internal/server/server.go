package server

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"wbo/internal/ai"
	"wbo/internal/ir"
	"wbo/internal/project"
	"wbo/internal/runner"
)

type Server struct {
	matchSetup func() (uint64, string, error)
	cards      *ir.CardPack
	tests      *ir.TestPack
	// illustrationRoot 指向 WBArts 的 data 目录；为空时只用内置主界面插图。
	illustrationRoot string
	illustrations    illustrationCache
	// lobbyAuth 是"进入大厅需要登录"的校验（复用 WBArts 的账号）。
	lobbyAuth lobbyAuth
	// version 是部署时写入的版本标识（通常是提交号），用于核对线上跑的是哪一版。
	version     string
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
	// format 是本局的构筑赛制；客方（含 AI）的牌组要按同一赛制校验。
	format runner.Format
	// botPolicy 非空表示客方席位由 AI 驱动（练习模式），此时不需要第二名玩家。
	botPolicy ai.Policy
	botDriver *ai.Driver
	botError  string
	// spectators 是只读观众凭据：只能读状态、事件与推送流。
	spectators map[string]bool
	mu         sync.Mutex
	replay     []replayFrame
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
	SelectedLeaderSides []string `json:"selectedLeaderSides,omitempty"`
	SelectedOptionID    int      `json:"selectedOptionId,omitempty"`
}

type createRequest struct {
	Scenario string `json:"scenario"`
	Deck     []int  `json:"deck,omitempty"`
	// Format 是构筑赛制：rotation（指定模式，默认）或 unlimited（无限制模式）。
	Format string `json:"format,omitempty"`
	// Mode 为 "bot" 时创建练习模式：客方席位由 BotPolicy 指定的策略接管。
	Mode      string `json:"mode,omitempty"`
	BotDeck   []int  `json:"botDeck,omitempty"`
	BotPolicy string `json:"botPolicy,omitempty"`
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
	// Bot 表示这一局是练习模式（对手是 AI）；BotError 非空表示 AI 驱动失败。
	Bot      bool   `json:"bot,omitempty"`
	BotError string `json:"botError,omitempty"`
}

type replayResponse struct {
	MatchID string            `json:"matchId"`
	Events  []ir.RuntimeEvent `json:"events"`
	State   runner.StateView  `json:"state"`
	Winner  string            `json:"winner,omitempty"`
	Frames  []replayViewFrame `json:"frames"`
}

func New(root string, paths []string) (*Server, error) {
	return NewWithIllustrations(root, paths, DefaultIllustrationRoot(root))
}

// DefaultIllustrationRoot 找同级的 WBArts 数据目录（有 home_illust_index.json 才算）。
func DefaultIllustrationRoot(root string) string {
	candidates := []string{
		filepath.Join(root, "..", "WBArts", "data"),
		filepath.Join(root, "..", "..", "WBArts", "data"),
	}
	for _, candidate := range candidates {
		absolute, err := filepath.Abs(candidate)
		if err != nil {
			continue
		}
		if _, err := os.Stat(filepath.Join(absolute, "home_illust_index.json")); err == nil {
			return absolute
		}
	}
	return ""
}

// NewWithIllustrations 允许显式指定主界面插图的数据目录（传空字符串表示只用内置素材）。
func NewWithIllustrations(root string, paths []string, illustrationRoot string) (*Server, error) {
	return NewWithOptions(root, paths, illustrationRoot, "")
}

// NewWithOptions 额外指定大厅登录校验地址（WBArts 的 /api/auth/me；空字符串=本地模式不校验）。
func NewWithOptions(root string, paths []string, illustrationRoot, authVerifyURL string) (*Server, error) {
	loaded := project.LoadWithRoot(paths, true, root)
	if loaded.HasErrors() {
		return nil, fmt.Errorf("load simulator sources: %v", loaded.Diagnostics)
	}
	cards, tests, err := project.BuildRuntimePacks(loaded)
	if err != nil {
		return nil, err
	}
	return NewWithPacks(cards, tests, illustrationRoot, authVerifyURL)
}

// NewWithPacks 用已经编译好的卡池构造服务：桌面客户端把卡池嵌在二进制里，
// 解包后编译一次，之后不再读磁盘上的 .wbo。
func NewWithPacks(cards *ir.CardPack, tests *ir.TestPack, illustrationRoot, authVerifyURL string) (*Server, error) {
	if cards == nil {
		return nil, fmt.Errorf("card pack is required")
	}
	if tests == nil {
		tests = &ir.TestPack{}
	}
	return &Server{
		cards: cards, tests: tests, illustrationRoot: illustrationRoot,
		lobbyAuth: newLobbyAuth(authVerifyURL),
		sessions:  map[string]*runner.Session{}, matches: map[string]*match{},
	}, nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", s.health)
	mux.HandleFunc("/api/cards", s.cardCatalog)
	mux.HandleFunc("/api/deckcode", s.deckCodeHandler)
	mux.HandleFunc("/api/illustrations", s.illustrationsHandler)
	mux.HandleFunc("/illustration-assets/", s.illustrationAssetHandler)
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
			ID          string `json:"id"`
			Waiting     bool   `json:"waiting"`
			Started     bool   `json:"started"`
			Spectatable bool   `json:"spectatable"`
			Bot         bool   `json:"bot"`
			Turn        int    `json:"turn,omitempty"`
		}
		s.mu.Lock()
		items := make([]roomSummary, 0, len(s.matches))
		for id, room := range s.matches {
			room.mu.Lock()
			summary := roomSummary{ID: id, Waiting: !room.joined, Started: room.started, Spectatable: room.session != nil, Bot: room.botDriver != nil}
			if room.session != nil {
				if view, err := room.session.SpectatorView(); err == nil {
					summary.Turn = view.Turn.Number
				}
			}
			items = append(items, summary)
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
	// 建房属于大厅动作：托管在线上的实例要求先登录（本地模式不需要）。
	if !s.requireLobbyLogin(w, r) {
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
	format, err := runner.FormatByID(s.cards, input.Format)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if input.Deck != nil {
		if err := runner.ValidateDeckForFormat(s.cards, input.Deck, format); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		deck = input.Deck
	}
	id, token, joinCode := randomID(6), randomID(24), randomID(8)
	room := &match{ownDeck: append([]int(nil), deck...), players: map[string]string{token: "own"}, joinCode: joinCode, format: format, mulligan: map[string]bool{}, mulliganSelection: map[string][]string{}}
	if input.Mode == "bot" {
		botDeck := input.BotDeck
		if botDeck == nil {
			botDeck = practiceDeck()
		}
		if err := runner.ValidateDeckForFormat(s.cards, botDeck, format); err != nil {
			http.Error(w, "bot deck: "+err.Error(), http.StatusBadRequest)
			return
		}
		room.botPolicy = botPolicyNamed(input.BotPolicy)
		room.botDriver = ai.NewDriver("oppo", room.botPolicy, ai.Limits{})
		// 练习模式不需要等第二名玩家：客方席位就绪，AI 的换牌直接保留起手。
		room.players[botToken] = "oppo"
		room.joined = true
		room.mulligan["oppo"] = true
		if err := s.startMatch(room, botDeck); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	s.mu.Lock()
	for s.matches[id] != nil {
		id = randomID(6)
	}
	s.matches[id] = room
	s.mu.Unlock()
	if room.botDriver != nil {
		runBot(room)
	}
	writeMatch(w, id, token, "own", room, nil)
}

// botToken 是练习模式里客方席位凭据；它从不返回给客户端。
const botToken = "bot-seat"

// spectatorSide 是只读观众在接口层使用的"席位"。
const spectatorSide = "spectator"

// viewerFor 解析凭据：玩家席位或只读观众。
func viewerFor(room *match, token string) (string, bool) {
	if token == "" {
		return "", false
	}
	if side, ok := room.players[token]; ok {
		return side, true
	}
	if room.spectators[token] {
		return spectatorSide, true
	}
	return "", false
}

// botPolicyNamed 把请求里的策略名映射成策略实例；未知名称退回 greedy。
func botPolicyNamed(name string) ai.Policy {
	if name == "random" {
		return ai.NewRandom(1)
	}
	return &ai.Greedy{}
}

// startMatch 在客方牌组就绪后建局（人类加入与练习模式共用）。
func (s *Server) startMatch(room *match, oppoDeck []int) error {
	setup := s.matchSetup
	if setup == nil {
		setup = randomMatchSetup
	}
	seed, first, err := setup()
	if err != nil {
		return fmt.Errorf("could not initialize match randomness")
	}
	session, err := runner.NewMatchSessionWithFirstPlayer(s.cards, room.ownDeck, oppoDeck, seed, first)
	if err != nil {
		return err
	}
	room.session = session
	recordReplay(room)
	return nil
}

// runBot 反复推进 AI 席位，直到轮不到它为止（轮到人类、人类需要做选择，或对局结束）。
func runBot(room *match) {
	if room == nil || room.botDriver == nil || room.session == nil || room.botError != "" {
		return
	}
	if _, err := room.botDriver.RunUntilBlocked(room.session); err != nil {
		room.botError = err.Error()
	}
	recordReplay(room)
}

func practiceDeck() []int {
	return runner.PracticeDeck()
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
	// 推送流是长连接，不能握着房间锁；它自己在每次取样时短暂加锁。
	if len(parts) == 2 && parts[1] == "stream" && r.Method == http.MethodGet {
		s.streamMatch(w, r, parts[0], room)
		return
	}
	room.mu.Lock()
	defer room.mu.Unlock()
	if len(parts) == 2 && parts[1] == "spectate" && r.Method == http.MethodPost {
		// 观战凭据：只读。任何人都可以申请，但拿不到玩家令牌与邀请码。
		if room.session == nil {
			http.Error(w, "match has not started", http.StatusConflict)
			return
		}
		token := randomID(24)
		if room.spectators == nil {
			room.spectators = map[string]bool{}
		}
		room.spectators[token] = true
		state, err := room.session.SpectatorView()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, response{MatchID: parts[0], PlayerToken: token, Side: "spectator", State: state, Capabilities: runner.SupportedSimulatorCapabilities(), Events: room.session.EventsFor("spectator"), Bot: room.botDriver != nil})
		return
	}
	if len(parts) == 2 && parts[1] == "replay" && r.Method == http.MethodGet {
		token := bearerToken(r)
		side, ok := room.players[token]
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		state, err := matchState(room, side)
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
		if !s.requireLobbyLogin(w, r) {
			return
		}
		var input createRequest
		if r.Body != nil {
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil && err != io.EOF {
				http.Error(w, "invalid request", http.StatusBadRequest)
				return
			}
		}
		deck := input.Deck
		if deck == nil {
			deck = practiceDeck()
		}
		if err := runner.ValidateDeckForFormat(s.cards, deck, room.format); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := s.startMatch(room, deck); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		token := randomID(24)
		room.players[token], room.joined = "oppo", true
		writeMatch(w, parts[0], token, "oppo", room, nil)
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
			writeMatch(w, parts[0], "", side, room, &result)
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
			// 练习模式里 AI 可能先手，开局就要让它动起来。
			runBot(room)
		}
		writeMatch(w, parts[0], "", side, room, nil)
		return
	}
	if len(parts) != 1 {
		http.NotFound(w, r)
		return
	}
	token := bearerToken(r)
	side, ok := viewerFor(room, token)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if r.Method == http.MethodGet {
		writeMatch(w, parts[0], "", side, room, nil)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if side == spectatorSide {
		http.Error(w, "spectator credentials are read-only", http.StatusForbidden)
		return
	}
	if !room.joined {
		http.Error(w, "waiting for player", http.StatusConflict)
		return
	}
	var input command
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	input.Actor = side
	if input.Kind == "concede" {
		result := room.session.SubmitAs(input.ActionID, side, runner.SimulatorCommand{Kind: "concede"})
		recordReplay(room)
		writeMatch(w, parts[0], "", side, room, &result)
		return
	}
	if !room.started {
		http.Error(w, "mulligan in progress", http.StatusConflict)
		return
	}
	view, _ := room.session.View(side)
	if input.ExpectedRevision != nil && *input.ExpectedRevision != view.Revision {
		result := runner.StepResult{Status: runner.StatusRejected, ErrorCode: "stale_state"}
		writeMatch(w, parts[0], "", side, room, &result)
		return
	}
	if room.session.PendingChoice() != nil && view.PendingChoice == nil {
		http.Error(w, "choice belongs to opponent", http.StatusConflict)
		return
	}
	result := submit(room.session, input)
	recordReplay(room)
	// 练习模式：人类每提交一次（含回答选择）就让 AI 推进到它再次等待为止。
	runBot(room)
	writeMatch(w, parts[0], "", side, room, &result)
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
	if !room.started {
		own.Phase, oppo.Phase = "mulligan", "mulligan"
	}
	frame := replayFrame{Revision: own.Revision, EventCount: len(room.session.Events()), Own: own, Oppo: oppo}
	if n := len(room.replay); n > 0 && room.replay[n-1].Revision == own.Revision {
		room.replay[n-1] = frame
		return
	}
	room.replay = append(room.replay, frame)
}

func writeMatch(w http.ResponseWriter, id, token, side string, room *match, result *runner.StepResult) {
	w.Header().Set("Cache-Control", "no-store")
	state, err := matchState(room, side)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	phase := "mulligan"
	if !room.joined {
		phase = "waiting"
	}
	if room.started {
		phase = "main"
	}
	state.Phase = phase
	actions := []runner.LegalAction{}
	if room.started {
		actions = room.session.LegalActionsFor(side)
	}
	if result != nil && result.Choice != nil {
		visible := *result
		visible.Choice = state.PendingChoice
		result = &visible
	}
	joinCode := ""
	if side == "own" && !room.joined {
		joinCode = room.joinCode
	}
	writeJSON(w, response{MatchID: id, PlayerToken: token, JoinCode: joinCode, Side: side, Waiting: !room.joined, MatchPhase: phase, MulliganReady: room.mulligan[side], OpponentReady: room.mulligan[oppositeMatchSide(side)], Result: result, State: state, LegalActions: actions, Capabilities: runner.SupportedSimulatorCapabilities(), Events: room.session.EventsFor(side), Bot: room.botDriver != nil, BotError: room.botError})
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
	writeJSON(w, map[string]any{
		"ok": true, "ruleset": "wbo-standard-0.3.0", "version": s.version,
		// 客户端据此决定进大厅是否需要登录（本地开发/离线时是 false）。
		"lobbyRequiresLogin": s.lobbyAuth.required,
	})
}

// SetVersion 记录构建/部署版本，供 /api/health 汇报。
func (s *Server) SetVersion(version string) {
	s.version = strings.TrimSpace(version)
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
		request := runner.ChoiceResponse{RequestID: input.RequestID, ActionID: input.ActionID, StateRevision: input.StateRevision, SelectedInstanceIDs: input.SelectedInstanceIDs, SelectedLeaderSides: input.SelectedLeaderSides, SelectedOptionID: input.SelectedOptionID}
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
