package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"wbo/internal/ai"
	"wbo/internal/ir"
	"wbo/internal/project"
	"wbo/internal/runner"
)

// wbo env：给训练进程用的 JSON-lines 环境协议（stdin 收命令，stdout 发行事件）。
//
// 设计要点（详见 docs/tooling/env-protocol.md）：
//   - 一个进程一局（reset 开新局），双方卡组、种子、先手都由命令给出；
//   - 只有"学习者这一侧需要做决定"时才吐 state 事件，事件里带完整状态与合法动作列表，
//     动作以**下标**提交，避免训练侧解析动作语义；
//   - v2：学习者的选择（target/mode/fusion_material）也进入同一套动作列表——
//     选择被拆成"逐个选候选 + 需要时确认"的自回归子动作，环境侧累积到满足下限/上限再提交引擎；
//     同一组合只沿候选下标递增的顺序可达，避免同一组合出现多种排列；
//   - 结束一局吐 done 事件（终止奖励：赢 +1 / 输 -1）。
const (
	envProtocol = "wbo-env/2"
	envEncoding = "wbo-obs/1"
)

type envCommand struct {
	Cmd         string `json:"cmd"`
	Seed        uint64 `json:"seed,omitempty"`
	Deck        []int  `json:"deck,omitempty"`
	OppoDeck    []int  `json:"oppoDeck,omitempty"`
	Format      string `json:"format,omitempty"`
	FirstPlayer string `json:"firstPlayer,omitempty"`
	Opponent    string `json:"opponent,omitempty"`
	Action      int    `json:"action,omitempty"`
}

// envChoiceCandidate 是一个可选候选：Key 是稳定标识，训练侧只需回传下标，
// Key 只用于日志、调试与观测编码（例如拿到候选对应的卡牌 ID）。
type envChoiceCandidate struct {
	Key        string            `json:"key"`
	Kind       string            `json:"kind"`
	InstanceID string            `json:"instanceId,omitempty"`
	LeaderSide string            `json:"leaderSide,omitempty"`
	OptionID   int               `json:"optionId,omitempty"`
	CardID     int               `json:"cardId,omitempty"`
	Labels     map[string]string `json:"labels,omitempty"`
}

// envChoice 描述学习者当前待处理的选择请求与已累积的选择。
type envChoice struct {
	RequestID     string               `json:"requestId"`
	Kind          string               `json:"kind"`
	MinSelections int                  `json:"minSelections"`
	MaxSelections int                  `json:"maxSelections"`
	Selected      []string             `json:"selected,omitempty"`
	Candidates    []envChoiceCandidate `json:"candidates"`
}

type envEvent struct {
	Type     string               `json:"type"`
	Protocol string               `json:"protocol,omitempty"`
	Engine   string               `json:"engine,omitempty"`
	Ruleset  string               `json:"ruleset,omitempty"`
	PoolHash string               `json:"poolHash,omitempty"`
	Encoding string               `json:"encoding,omitempty"`
	Seed     uint64               `json:"seed,omitempty"`
	Side     string               `json:"side,omitempty"`
	Turn     int                  `json:"turn,omitempty"`
	Active   string               `json:"active,omitempty"`
	Phase    string               `json:"phase,omitempty"`
	View     *runner.StateView    `json:"view,omitempty"`
	Legal    []runner.LegalAction `json:"legal,omitempty"`
	Choice   *envChoice           `json:"choice,omitempty"`
	Done     bool                 `json:"done,omitempty"`
	Winner   string               `json:"winner,omitempty"`
	Reward   float64              `json:"reward,omitempty"`
	Fault    string               `json:"fault,omitempty"`
	Message  string               `json:"message,omitempty"`
}

// envSession 把一局封在环境里：学习者侧 + 对手侧（内置策略）。
type envSession struct {
	session  *runner.Session
	learner  string
	opponent *ai.Driver
	seed     uint64

	// v2 选择的累积状态：同一 RequestID 期间有效。
	choiceRequestID string
	chosenInstances []string
	chosenLeaders   []string
	chosenOptions   []int
}

func runEnv(args []string) int {
	args = interspersed(args, map[string]bool{"--source-root": true, "--engine": true, "--version": true})
	fs := flag.NewFlagSet("env", flag.ContinueOnError)
	root := fs.String("source-root", "", "稳定源路径根目录（默认当前工作目录）")
	engine := fs.String("engine", "", "回报给训练侧的引擎版本标识")
	fs.SetOutput(os.Stderr)
	if fs.Parse(args) != nil {
		return 2
	}
	paths := fs.Args()
	if len(paths) == 0 {
		paths = []string{"cards", "tests"}
	}
	loaded := load(paths, true, *root)
	printDiagnostics(loaded.Diagnostics)
	if loaded.HasErrors() {
		return 1
	}
	cards, _, err := project.BuildRuntimePacks(loaded)
	if err != nil {
		fmt.Fprintln(os.Stderr, "编译卡池失败:", err)
		return 1
	}

	encoder := json.NewEncoder(os.Stdout)
	decoder := json.NewDecoder(bufio.NewReaderSize(os.Stdin, 1<<20))
	poolHash := cardPoolHash(cards)

	var current *envSession
	send := func(event envEvent) {
		if err := encoder.Encode(event); err != nil {
			fmt.Fprintln(os.Stderr, "写出事件失败:", err)
			os.Exit(1)
		}
	}
	fail := func(err error) {
		send(envEvent{Type: "error", Fault: err.Error()})
	}

	for decoder.More() {
		var command envCommand
		if err := decoder.Decode(&command); err != nil {
			fail(fmt.Errorf("无法解析命令: %w", err))
			continue
		}
		switch command.Cmd {
		case "hello", "":
			send(envEvent{
				Type: "ready", Protocol: envProtocol, Engine: strings.TrimSpace(*engine),
				Ruleset: "wbo-standard-0.3.0", PoolHash: poolHash, Encoding: envEncoding,
				Message: "send {\"cmd\":\"reset\",…} to start an episode",
			})
		case "quit":
			return 0
		case "reset":
			session, err := newEnvSession(cards, command)
			if err != nil {
				fail(err)
				continue
			}
			current = session
			event, err := current.advance(cards)
			if err != nil {
				fail(err)
				continue
			}
			event.Seed = current.seed
			send(event)
		case "step":
			if current == nil {
				fail(fmt.Errorf("先发送 reset"))
				continue
			}
			if err := current.step(cards, command.Action); err != nil {
				fail(err)
				continue
			}
			event, err := current.advance(cards)
			if err != nil {
				fail(err)
				continue
			}
			event.Seed = current.seed
			send(event)
		default:
			fail(fmt.Errorf("未知命令 %q", command.Cmd))
		}
	}
	return 0
}

// newEnvSession 建一局：学习者固定坐 own（观察里按它转换），对手用内置策略。
func newEnvSession(cards *ir.CardPack, command envCommand) (*envSession, error) {
	format, err := runner.FormatByID(cards, command.Format)
	if err != nil {
		return nil, err
	}
	learnerDeck := command.Deck
	if len(learnerDeck) == 0 {
		learnerDeck = runner.PracticeDeck()
	}
	opponentDeck := command.OppoDeck
	if len(opponentDeck) == 0 {
		opponentDeck = runner.PracticeDeck()
	}
	if err := runner.ValidateDeckForFormat(cards, learnerDeck, format); err != nil {
		return nil, fmt.Errorf("learner deck: %w", err)
	}
	if err := runner.ValidateDeckForFormat(cards, opponentDeck, format); err != nil {
		return nil, fmt.Errorf("opponent deck: %w", err)
	}
	first := command.FirstPlayer
	if first != "own" && first != "oppo" {
		first = "own"
		if command.Seed%2 == 1 {
			first = "oppo"
		}
	}
	session, err := runner.NewMatchSessionWithFirstPlayer(cards, learnerDeck, opponentDeck, command.Seed, first)
	if err != nil {
		return nil, err
	}
	policy := ai.Policy(&ai.Greedy{})
	switch command.Opponent {
	case "random":
		policy = ai.NewRandom(command.Seed*2 + 1)
	case "greedy", "":
	default:
		return nil, fmt.Errorf("未知对手策略 %q（支持 random / greedy）", command.Opponent)
	}
	return &envSession{
		session:  session,
		learner:  "own",
		opponent: ai.NewDriver("oppo", policy, ai.Limits{}),
		seed:     command.Seed,
	}, nil
}

// advance 推进到"学习者要做决定"或终局：先让对手走完，再自动回答学习者的选择。
func (e *envSession) advance(cards *ir.CardPack) (envEvent, error) {
	for step := 0; step < 2000; step++ {
		view, err := e.session.View(e.learner)
		if err != nil {
			return envEvent{}, err
		}
		if view.GameOver {
			reward := 0.0
			if view.Winner == e.learner {
				reward = 1
			} else if view.Winner != "" {
				reward = -1
			}
			return envEvent{Type: "done", Done: true, Winner: view.Winner, Reward: reward, Turn: view.Turn.Number, View: &view}, nil
		}
		if pending := e.session.PendingChoice(); pending != nil {
			if pending.PublicTo == e.learner {
				// v2：学习者的选择进入同一套动作列表（select/confirm）。
				if e.choiceRequestID != pending.RequestID {
					e.resetChoice(pending.RequestID)
				}
				if len(e.choiceLegal(*pending)) == 0 {
					// 没有可选项且下限为 0：直接以空选择提交，避免环境卡死。
					if pending.MinSelections == 0 {
						if err := e.submitChoice(e.choiceResponse(*pending)); err != nil {
							return envEvent{}, err
						}
						continue
					}
					return envEvent{}, fmt.Errorf(
						"选择请求没有任何可选项（kind=%s min=%d max=%d candidates=%d selected=%d request=%s）",
						pending.Kind, pending.MinSelections, pending.MaxSelections, len(pending.Candidates),
						e.selectionCount(), pending.RequestID)
				}
				return e.choiceState(view, *pending), nil
			}
			// 对手的选择由对手策略回答。
			if _, err := e.opponent.Step(e.session); err != nil {
				return envEvent{}, err
			}
			continue
		}
		if view.Turn.Active == "own" {
			legal := e.session.LegalActionsFor(e.learner)
			if len(legal) == 0 {
				return envEvent{}, fmt.Errorf("学习者回合没有合法动作（引擎异常）")
			}
			return envEvent{
				Type: "state", Side: e.learner, Turn: view.Turn.Number, Phase: view.Phase,
				View: &view, Legal: legal,
			}, nil
		}
		// 轮到对手：让对手一路走到再次轮到学习者（或终局）。
		if _, err := e.opponent.RunUntilBlocked(e.session); err != nil {
			return envEvent{}, err
		}
		if e.session.PendingChoice() == nil {
			after, err := e.session.View(e.learner)
			if err == nil && after.Turn.Active != "own" && !after.GameOver {
				return envEvent{}, fmt.Errorf("对手停在不该停的位置（回合 %d）", after.Turn.Number)
			}
		}
	}
	return envEvent{}, fmt.Errorf("单次推进超过步数上限")
}

// step 按下标提交一个动作：待处理选择期间提交的是 select/confirm，否则是主动作。
func (e *envSession) step(cards *ir.CardPack, index int) error {
	view, err := e.session.View(e.learner)
	if err != nil {
		return err
	}
	if view.GameOver {
		return fmt.Errorf("对局已结束")
	}
	if pending := e.session.PendingChoice(); pending != nil && pending.PublicTo == e.learner {
		return e.stepChoice(*pending, index)
	}
	if view.Turn.Active != "own" {
		return fmt.Errorf("现在不是学习者的回合")
	}
	legal := e.session.LegalActionsFor(e.learner)
	if index < 0 || index >= len(legal) {
		return fmt.Errorf("动作下标 %d 越界（合法动作 %d 个）", index, len(legal))
	}
	action := legal[index]
	result := e.session.SubmitAs(envActionID(index), e.learner, ai.CommandFor(action))
	if result.Status != runner.StatusCompleted && result.Status != runner.StatusSuspended {
		return fmt.Errorf("动作 %s 被拒绝: %s", action.Kind, result.ErrorCode)
	}
	return nil
}

// choiceState 把待处理的选择渲染成一次 state 事件：合法动作是 select 与 confirm。
func (e *envSession) choiceState(view runner.StateView, pending runner.ChoiceRequest) envEvent {
	candidates := make([]envChoiceCandidate, 0, len(pending.Candidates))
	for _, candidate := range pending.Candidates {
		item := envChoiceCandidate{
			Key: choiceKey(candidate), Kind: candidate.Kind,
			InstanceID: candidate.InstanceID, LeaderSide: candidate.LeaderSide,
			OptionID: candidate.OptionID, Labels: candidate.Labels,
		}
		if candidate.Kind == "entity" {
			item.CardID = viewCardID(view, candidate.InstanceID)
		}
		candidates = append(candidates, item)
	}
	return envEvent{
		Type: "state", Side: e.learner, Turn: view.Turn.Number, Phase: view.Phase,
		View: &view, Legal: e.choiceLegal(pending),
		Choice: &envChoice{
			RequestID: pending.RequestID, Kind: pending.Kind,
			MinSelections: pending.MinSelections, MaxSelections: pending.MaxSelections,
			Selected: e.selectedKeys(), Candidates: candidates,
		},
	}
}

// choiceLegal 给出当前可用的选择动作：可选的候选、可撤销的已选项，
// 以及"达到下限但未满上限"时的 confirm。选满上限会自动提交，因此不会出现
// "想取消却已经提交"的情况；自由撤销保证任何合法组合都可达（递增顺序会走进死路）。
func (e *envSession) choiceLegal(pending runner.ChoiceRequest) []runner.LegalAction {
	selected := e.selectedKeys()
	legal := make([]runner.LegalAction, 0, len(pending.Candidates)+len(selected)+1)
	if e.selectionCount() < pending.MaxSelections {
		for _, candidate := range pending.Candidates {
			key := choiceKey(candidate)
			if e.isSelected(key) {
				continue
			}
			legal = append(legal, runner.LegalAction{Kind: "select", Actor: e.learner, Source: key})
		}
	}
	for _, key := range selected {
		legal = append(legal, runner.LegalAction{Kind: "deselect", Actor: e.learner, Source: key})
	}
	if e.selectionCount() >= pending.MinSelections && e.selectionCount() < pending.MaxSelections {
		legal = append(legal, runner.LegalAction{Kind: "confirm", Actor: e.learner})
	}
	return legal
}

// choiceStep 处理一个选择子动作，返回（可能已经完整的）响应与是否该提交给引擎。
// 这里不碰会话，便于单独验证"递增顺序 + 下限确认"这套累积规则。
func (e *envSession) choiceStep(pending runner.ChoiceRequest, index int) (runner.ChoiceResponse, bool, error) {
	if e.choiceRequestID != pending.RequestID {
		e.resetChoice(pending.RequestID)
	}
	legal := e.choiceLegal(pending)
	if index < 0 || index >= len(legal) {
		return runner.ChoiceResponse{}, false, fmt.Errorf("选择下标 %d 越界（合法选择 %d 个）", index, len(legal))
	}
	action := legal[index]
	if action.Kind == "confirm" {
		return e.choiceResponse(pending), true, nil
	}
	candidate, ok := findCandidate(pending, action.Source)
	if !ok {
		return runner.ChoiceResponse{}, false, fmt.Errorf("候选 %q 不存在", action.Source)
	}
	if action.Kind == "deselect" {
		e.dropSelection(action.Source)
		return runner.ChoiceResponse{}, false, nil
	}
	switch candidate.Kind {
	case "entity":
		e.chosenInstances = append(e.chosenInstances, candidate.InstanceID)
	case "leader":
		e.chosenLeaders = append(e.chosenLeaders, candidate.LeaderSide)
	case "option":
		e.chosenOptions = append(e.chosenOptions, candidate.OptionID)
	default:
		return runner.ChoiceResponse{}, false, fmt.Errorf("未知候选类型 %q", candidate.Kind)
	}
	if e.selectionCount() >= pending.MaxSelections {
		return e.choiceResponse(pending), true, nil
	}
	return runner.ChoiceResponse{}, false, nil
}

// stepChoice 处理一个选择子动作，必要时把完整响应交给引擎。
func (e *envSession) stepChoice(pending runner.ChoiceRequest, index int) error {
	response, complete, err := e.choiceStep(pending, index)
	if err != nil || !complete {
		return err
	}
	return e.submitChoice(response)
}

func (e *envSession) choiceResponse(pending runner.ChoiceRequest) runner.ChoiceResponse {
	return runner.ChoiceResponse{
		RequestID:           pending.RequestID,
		ActionID:            pending.ActionID,
		StateRevision:       pending.StateRevision,
		SelectedInstanceIDs: append([]string(nil), e.chosenInstances...),
		SelectedLeaderSides: append([]string(nil), e.chosenLeaders...),
		SelectedOptionIDs:   append([]int(nil), e.chosenOptions...),
	}
}

// submitChoice 把累积的选择一次性交给引擎。
func (e *envSession) submitChoice(response runner.ChoiceResponse) error {
	result := e.session.Resume(response)
	if result.Status != runner.StatusCompleted && result.Status != runner.StatusSuspended {
		return fmt.Errorf("选择被拒绝: %s", result.ErrorCode)
	}
	e.resetChoice("")
	return nil
}

func (e *envSession) selectionCount() int {
	return len(e.chosenInstances) + len(e.chosenLeaders) + len(e.chosenOptions)
}

func (e *envSession) resetChoice(requestID string) {
	e.choiceRequestID = requestID
	e.chosenInstances = nil
	e.chosenLeaders = nil
	e.chosenOptions = nil
}

// isSelected 按候选 key 判断是否已选。
func (e *envSession) isSelected(key string) bool {
	for _, item := range e.selectedKeys() {
		if item == key {
			return true
		}
	}
	return false
}

// dropSelection 撤销一个已选候选。
func (e *envSession) dropSelection(key string) {
	switch {
	case strings.HasPrefix(key, "e:"):
		e.chosenInstances = dropString(e.chosenInstances, strings.TrimPrefix(key, "e:"))
	case strings.HasPrefix(key, "l:"):
		e.chosenLeaders = dropString(e.chosenLeaders, strings.TrimPrefix(key, "l:"))
	case strings.HasPrefix(key, "o:"):
		option, err := strconv.Atoi(strings.TrimPrefix(key, "o:"))
		if err != nil {
			return
		}
		for index, item := range e.chosenOptions {
			if item == option {
				e.chosenOptions = append(e.chosenOptions[:index], e.chosenOptions[index+1:]...)
				break
			}
		}
	}
}

func dropString(items []string, want string) []string {
	out := items[:0]
	for _, item := range items {
		if item != want {
			out = append(out, item)
		}
	}
	return out
}

func (e *envSession) selectedKeys() []string {
	keys := make([]string, 0, e.selectionCount())
	for _, id := range e.chosenInstances {
		keys = append(keys, "e:"+id)
	}
	for _, side := range e.chosenLeaders {
		keys = append(keys, "l:"+side)
	}
	for _, option := range e.chosenOptions {
		keys = append(keys, "o:"+strconv.Itoa(option))
	}
	return keys
}

func choiceKey(candidate runner.ChoiceCandidate) string {
	switch candidate.Kind {
	case "entity":
		return "e:" + candidate.InstanceID
	case "leader":
		return "l:" + candidate.LeaderSide
	case "option":
		return "o:" + strconv.Itoa(candidate.OptionID)
	default:
		return candidate.Kind + ":"
	}
}

func findCandidate(pending runner.ChoiceRequest, key string) (runner.ChoiceCandidate, bool) {
	for _, candidate := range pending.Candidates {
		if choiceKey(candidate) == key {
			return candidate, true
		}
	}
	return runner.ChoiceCandidate{}, false
}

// viewCardID 在观察里查实例对应的卡牌 ID（找不到时返回 0，训练侧应容忍缺失）。
func viewCardID(view runner.StateView, instanceID string) int {
	for _, player := range []runner.PlayerView{view.Own, view.Oppo} {
		for _, zone := range [][]runner.EntityView{player.Hand, player.Field, player.Graveyard, player.Banished, player.Destroyed} {
			for _, entity := range zone {
				if entity.InstanceID == instanceID {
					return entity.CardID
				}
			}
		}
	}
	return 0
}

// envActionID 生成稳定的 32 位十六进制动作 ID（引擎要求 actionId 是这个形状）。
func envActionID(index int) string {
	return fmt.Sprintf("%032x", 0xe00000+index)
}

// cardPoolHash 是卡池指纹：训练侧把它写进数据集与模型，用来发现"引擎/卡池换了"。
func cardPoolHash(cards *ir.CardPack) string {
	ids := make([]int, 0, len(cards.Cards))
	for n := range cards.Cards {
		ids = append(ids, cards.Cards[n].ID)
	}
	sort.Ints(ids)
	hash := sha256.New()
	for _, id := range ids {
		fmt.Fprintf(hash, "%d\n", id)
	}
	return hex.EncodeToString(hash.Sum(nil))[:16]
}
