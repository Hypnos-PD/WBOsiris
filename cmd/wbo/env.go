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
	"wbo/internal/ruleset"
	"wbo/internal/runner"
	"wbo/internal/search"
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
	envProtocol = "wbo-env/3"
	envEncoding = "wbo-obs/1"
	// maxChoiceSteps 是同一个选择请求允许的子动作上限（正常最多几十次）。
	maxChoiceSteps = 64
)

type envCommand struct {
	Cmd         string `json:"cmd"`
	Seed        uint64 `json:"seed,omitempty"`
	Deck        []int  `json:"deck,omitempty"`
	OppoDeck    []int  `json:"oppoDeck,omitempty"`
	Format      string `json:"format,omitempty"`
	FirstPlayer string `json:"firstPlayer,omitempty"`
	Opponent    string `json:"opponent,omitempty"`
	// Oracle=true 时，每次 state 事件附带特权信息（训练专用；客户端不要传）。
	Oracle bool `json:"oracle,omitempty"`
	// OracleDeckTop 限制特权信息里牌库给多少张（默认 8）。
	OracleDeckTop int `json:"oracleDeckTop,omitempty"`
	Action        int `json:"action,omitempty"`
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

// envCardInfo 是卡池清单里的一张卡：训练侧用它检查外来数据的卡牌覆盖率与卡包窗口。
type envCardInfo struct {
	ID   int `json:"id"`
	Pack int `json:"pack"`
	// Keywords 是卡牌的**固有**关键词（含由触发推导的 lastwords/engage/triggered）。
	// 训练侧用它给手牌行补关键词：回放的场面行带打包状态位，而手牌行没有，
	// 不补就会造成"训练时手牌没有关键词、推理时引擎却在手牌上给了关键词"的分布偏移。
	Keywords []string `json:"keywords,omitempty"`
	Class    string   `json:"class,omitempty"`
	Type     string   `json:"type,omitempty"`
	Name     string   `json:"name,omitempty"`
	// 结构化卡面数据（给训练侧做**具体**的结构特征，而不只是费用/身材）：
	// 稀有度、费用、基础身材、效果语义标签（damage/destroy/summon/add_keyword/spellboost…）。
	Rarity string   `json:"rarity,omitempty"`
	Cost   int      `json:"cost,omitempty"`
	Attack int      `json:"attack,omitempty"`
	Life   int      `json:"life,omitempty"`
	Traits []string `json:"traits,omitempty"`
	Tags   []string `json:"tags,omitempty"`
	// Effects 是效果程序的 token 序列（最多 24 条）：训练侧用它编码"这张牌具体怎么做事"，
	// 连"mode 二选一"这类结构也保留（见 envEffectToken.option）。
	Effects []envEffectToken `json:"effects,omitempty"`
}

// envFormatInfo 是一个赛制：ID/名称 + 允许的卡包窗口。
type envFormatInfo struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Packs []int  `json:"packs"`
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
	// Oracle 是训练专用特权信息（仅在 reset 时传 oracle=true 才出现）。
	Oracle *runner.OracleView `json:"oracle,omitempty"`
	// History 是最近若干条**该视角可见**的对局事件（尾部窗口，旧的在前）。
	//
	// 观测里原本只有聚合量（墓地有哪些牌、这回合打过没有），没有"发生了什么"的序列：
	// 「先出 A 再出 B」和「先出 B 再出 A」在模型眼里一模一样，而这在规则里常常不等价。
	// 事件本身已经按视角裁剪（PrivateTo 不是本方的事件只剩 kind/side/count），
	// 所以给训练侧的历史窗口和客户端从流里拿到的是同一份东西。
	History []ir.RuntimeEvent `json:"history,omitempty"`
	// Cards 是 "deck" 命令的返回值：一副随机合法卡组。
	Format string `json:"format,omitempty"`
	Cards  []int  `json:"cards,omitempty"`
	// Pool 是 "card_pool" 命令的返回值。
	Pool []envCardInfo `json:"pool,omitempty"`
	// Formats 是 "formats" 命令的返回值：赛制与各自的卡包窗口。
	Formats []envFormatInfo `json:"formats,omitempty"`
	// Lookahead=true 表示这是一次"假想推进"的结果，真实对局没有被改变。
	Lookahead bool `json:"lookahead,omitempty"`
	// OK/Reason 是 deck_check 的判定结果。
	OK     bool   `json:"ok,omitempty"`
	Reason string `json:"reason,omitempty"`
}

// envSession 把一局封在环境里：学习者侧 + 对手侧（内置策略）。
type envSession struct {
	session  *runner.Session
	learner  string
	opponent *ai.Driver
	seed     uint64
	// oracle=true 时，state 事件里额外附带**训练专用**的特权信息（对手手牌内容、
	// 双方牌库的抽牌顺序）。默认关闭：客户端与回放路径永远拿不到这些隐藏信息。
	oracle bool

	// external=true 时不使用内置对手策略：对手侧的决定同样交给训练侧，
	// 于是自对弈、联赛与人类参与都能走同一条路径。
	external bool
	// side 是"当前等着行动的一方"，由最近一次 state 事件决定。
	side string
	// serial 生成唯一动作 ID，保证同一次会话里不会重复。
	serial uint64

	// v2 选择的累积状态：同一 RequestID 期间有效。
	choiceRequestID string
	chosenInstances []string
	chosenLeaders   []string
	chosenOptions   []int
	// choiceSteps 是同一个选择请求上的子动作计数：采样策略可能在 select/deselect
	// 之间来回循环，必须有硬上限，否则一局永远走不完。
	choiceSteps int
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
			current.attachOracle(&event)
			current.attachHistory(&event)
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
			current.attachOracle(&event)
			current.attachHistory(&event)
			send(event)
		case "deck":
			// 随机生成一副合法卡组：训练侧用它组卡组池，避免只练一副镜像卡组。
			format, err := runner.FormatByID(cards, command.Format)
			if err != nil {
				fail(err)
				continue
			}
			rng := ruleset.NewRNG(command.Seed)
			deck, err := ai.RandomDeck(cards, format, rng)
			if err != nil {
				fail(err)
				continue
			}
			send(envEvent{Type: "deck", Format: format.ID, Cards: deck})
		case "lookahead":
			// 假想推进：克隆当前局面、提交候选动作、让对手按内置策略回应，
			// 返回"如果这么打会到哪里"。真实对局不被修改，训练侧可以据此给候选重新排序。
			if current == nil {
				fail(fmt.Errorf("先发送 reset"))
				continue
			}
			event, err := current.lookahead(cards, command.Action)
			if err != nil {
				fail(err)
				continue
			}
			event.Seed = current.seed
			send(event)
		case "deck_check":
			// 用引擎自己的赛制校验判断一副外部卡组（例如从 WBA 卡组库拉来的）能不能用。
			// 卡池会随版本滚动，老卡组会被判为不合法——这一步是唯一真值来源。
			format, err := runner.FormatByID(cards, command.Format)
			if err != nil {
				fail(err)
				continue
			}
			if err := runner.ValidateDeckForFormat(cards, command.Deck, format); err != nil {
				send(envEvent{Type: "deck_check", Format: format.ID, OK: false, Reason: err.Error()})
				continue
			}
			send(envEvent{Type: "deck_check", Format: format.ID, OK: true})
		case "card_pool":
			// 卡池清单：训练侧用它检查外来数据（例如 WBC 回放）里的卡牌是否都在当前卡池里，
			// 以及每张卡属于哪个卡包（用来判断赛制窗口），并拿到结构化卡面特征。
			send(envEvent{Type: "card_pool", Pool: cardPool(cards)})
		case "formats":
			// 赛制与卡包窗口：训练/构筑侧要按赛制筛卡时，以引擎为准而不是自己推规则。
			formats := make([]envFormatInfo, 0, 2)
			for _, format := range runner.Formats(cards) {
				formats = append(formats, envFormatInfo{ID: format.ID, Name: format.Name, Packs: format.Packs})
			}
			send(envEvent{Type: "formats", Formats: formats})
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
	external := false
	switch command.Opponent {
	case "random":
		policy = ai.NewRandom(command.Seed*2 + 1)
	case "external":
		external = true
	case "greedy", "":
	default:
		return nil, fmt.Errorf("未知对手策略 %q（支持 random / greedy / external）", command.Opponent)
	}
	return &envSession{
		session:  session,
		learner:  "own",
		opponent: ai.NewDriver("oppo", policy, ai.Limits{}),
		seed:     command.Seed,
		external: external,
		oracle:   command.Oracle,
		side:     "own",
	}, nil
}

// attachOracle 在训练模式（reset 时传了 oracle=true）下给 state 事件补上特权信息。
// 默认关闭；这样客户端与回放路径在协议层面就拿不到对手手牌内容。
func (e *envSession) attachOracle(event *envEvent) {
	if !e.oracle || event == nil || event.View == nil {
		return
	}
	view, err := e.session.Oracle(8)
	if err != nil {
		return
	}
	event.Oracle = &view
}

// envHistoryLimit 是 state 事件里携带的历史事件条数上限。
//
// 16 条足够覆盖"本回合发生过什么"（一次攻击 + 一两次出牌 + 触发结算），
// 又不至于把协议消息撑大；训练侧只编码最近 HISTORY_EVENTS 条，多余的不影响模型。
const envHistoryLimit = 16

// attachHistory 给 state 事件补上"该视角可见的最近事件"。
//
// 每个决策点都带上尾部窗口，训练侧就不需要自己做状态差分去猜发生了什么；
// 事件顺序与引擎的 Sequence 一致（旧的在前，最近的在后）。
func (e *envSession) attachHistory(event *envEvent) {
	if event == nil || event.View == nil || e.session == nil {
		return
	}
	side := event.Side
	if side != "own" && side != "oppo" {
		side = e.learner
	}
	events := e.session.EventsFor(side)
	if len(events) > envHistoryLimit {
		events = events[len(events)-envHistoryLimit:]
	}
	event.History = events
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
			if pending.PublicTo == e.learner || e.external {
				// v2：选择进入同一套动作列表（select/deselect/confirm）。
				if e.choiceRequestID != pending.RequestID {
					e.resetChoice(pending.RequestID)
				}
				if e.choiceSteps >= maxChoiceSteps {
					return envEvent{}, fmt.Errorf(
						"选择子步骤超过 %d 次（策略可能在 select/deselect 之间循环）request=%s",
						maxChoiceSteps, pending.RequestID)
				}
				e.side = pending.PublicTo
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
				return e.choiceState(e.viewFor(pending.PublicTo), pending.PublicTo, *pending), nil
			}
			// 对手的选择由对手策略回答。
			if _, err := e.opponent.Step(e.session); err != nil {
				return envEvent{}, err
			}
			continue
		}
		active := e.learner
		if view.Turn.Active != "own" {
			active = e.other(e.learner)
		}
		if active == e.learner || e.external {
			legal := e.session.LegalActionsFor(active)
			if len(legal) == 0 {
				return envEvent{}, fmt.Errorf("%s 回合没有合法动作（引擎异常）", active)
			}
			view := e.viewFor(active)
			e.side = active
			return envEvent{
				Type: "state", Side: active, Turn: view.Turn.Number, Phase: view.Phase,
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
// 动作属于"最近一次 state 事件的那一方"（内置对手时永远是学习者，external 时两边都可能）。
func (e *envSession) step(cards *ir.CardPack, index int) error {
	side := e.side
	if side != "own" && side != "oppo" {
		side = e.learner
	}
	view, err := e.session.View(side)
	if err != nil {
		return err
	}
	if view.GameOver {
		return fmt.Errorf("对局已结束")
	}
	if pending := e.session.PendingChoice(); pending != nil && pending.PublicTo == side {
		return e.stepChoice(*pending, index)
	}
	if view.Turn.Active != "own" {
		return fmt.Errorf("现在不是 %s 的回合", side)
	}
	legal := e.session.LegalActionsFor(side)
	if index < 0 || index >= len(legal) {
		return fmt.Errorf("动作下标 %d 越界（合法动作 %d 个）", index, len(legal))
	}
	action := legal[index]
	e.serial++
	result := e.session.SubmitAs(envActionID(int(e.serial)), side, ai.CommandFor(action))
	if result.Status != runner.StatusCompleted && result.Status != runner.StatusSuspended {
		return fmt.Errorf("动作 %s 被拒绝: %s", action.Kind, result.ErrorCode)
	}
	return nil
}

// lookahead 返回"如果现在提交第 index 个动作会到达哪个局面"：
// 克隆当前会话、在克隆上走这一步、让对手按内置策略回应，然后返回克隆的事件。
// 真实会话（revision、RNG、选择累积）完全不受影响。
func (e *envSession) lookahead(cards *ir.CardPack, index int) (envEvent, error) {
	view, err := e.session.View(e.side)
	if err != nil {
		return envEvent{}, err
	}
	if view.GameOver {
		return envEvent{}, fmt.Errorf("对局已结束，无法假想推进")
	}
	if e.session.PendingChoice() != nil {
		return envEvent{}, fmt.Errorf("有待处理选择时不能假想推进主动作")
	}
	legal := e.session.LegalActionsFor(e.side)
	if index < 0 || index >= len(legal) {
		return envEvent{}, fmt.Errorf("候选动作下标 %d 越界（合法动作 %d 个）", index, len(legal))
	}
	clone, err := search.Clone(cards, e.session)
	if err != nil {
		return envEvent{}, err
	}
	shadow := &envSession{
		session:  clone,
		learner:  e.learner,
		opponent: e.opponent,
		seed:     e.seed,
		side:     e.side,
	}
	if err := shadow.step(cards, index); err != nil {
		return envEvent{}, err
	}
	event, err := shadow.advance(cards)
	if err != nil {
		return envEvent{}, err
	}
	event.Lookahead = true
	// 假想局面的历史 = 当前历史 + 这一步（以及对手回应）产生的事件，来自克隆会话。
	shadow.attachHistory(&event)
	return event, nil
}

// viewFor 取某一方视角的状态；出错时退回到自己的视角，避免状态事件缺失。
func (e *envSession) viewFor(side string) runner.StateView {
	if view, err := e.session.View(side); err == nil {
		return view
	}
	view, _ := e.session.View(e.learner)
	return view
}

func (e *envSession) other(side string) string {
	if side == "own" {
		return "oppo"
	}
	return "own"
}

// choiceState 把待处理的选择渲染成一次 state 事件：合法动作是 select/deslect/confirm。
func (e *envSession) choiceState(view runner.StateView, side string, pending runner.ChoiceRequest) envEvent {
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
		Type: "state", Side: side, Turn: view.Turn.Number, Phase: view.Phase,
		View: &view, Legal: e.choiceLegal(pending),
		Choice: &envChoice{
			RequestID: pending.RequestID, Kind: pending.Kind,
			MinSelections: pending.MinSelections, MaxSelections: pending.MaxSelections,
			Selected: e.selectedKeys(), Candidates: candidates,
		},
	}
}

// choiceLegal 给出当前可用的选择动作：还没选过的候选，以及"达到下限但未满上限"时的 confirm。
//
// 只允许"加选"、不允许撤销是刻意的：候选上限是有限的，于是子步骤数天然有界，
// 采样策略不可能在 select/deselect 之间来回循环；同时任何合法组合都仍然可达
// （逐个选中即可，选满上限自动提交）。
func (e *envSession) choiceLegal(pending runner.ChoiceRequest) []runner.LegalAction {
	actor := e.side
	if actor != "own" && actor != "oppo" {
		actor = e.learner
	}
	legal := make([]runner.LegalAction, 0, len(pending.Candidates)+1)
	if e.selectionCount() < pending.MaxSelections {
		for _, candidate := range pending.Candidates {
			key := choiceKey(candidate)
			if e.isSelected(key) {
				continue
			}
			legal = append(legal, runner.LegalAction{Kind: "select", Actor: actor, Source: key})
		}
	}
	if e.selectionCount() >= pending.MinSelections && e.selectionCount() < pending.MaxSelections {
		legal = append(legal, runner.LegalAction{Kind: "confirm", Actor: actor})
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
	e.choiceSteps++
	candidate, ok := findCandidate(pending, action.Source)
	if !ok {
		return runner.ChoiceResponse{}, false, fmt.Errorf("候选 %q 不存在", action.Source)
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
	e.choiceSteps = 0
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

// intrinsicKeywords 汇总一张牌的固有关键词（与 StateView 里实体关键词的口径一致）：
// 固有列表 + 由触发种类推导的 lastwords/engage/triggered。
func intrinsicKeywords(card *ir.Card) []string {
	seen := map[string]bool{}
	keywords := make([]string, 0, len(card.Intrinsic)+2)
	add := func(name string) {
		if name != "" && !seen[name] {
			seen[name] = true
			keywords = append(keywords, name)
		}
	}
	for _, name := range card.Intrinsic {
		add(name)
	}
	for _, ability := range card.Abilities {
		switch ir.TriggerKind(ability.Trigger) {
		case "lastwords":
			add("lastwords")
		case "engage":
			add("engage")
		case "fanfare", "evolve", "superevolve":
		default:
			add("triggered")
		}
	}
	return keywords
}
