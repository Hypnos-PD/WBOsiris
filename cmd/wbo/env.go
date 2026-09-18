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
//   - 只有"学习者这一侧需要做主动作"时才吐 state 事件，事件里带完整状态与合法动作列表，
//     动作以**下标**提交，避免训练侧解析动作语义；
//   - 需要选择（target/mode）时由环境内部按 greedy 规则自动回答，v1 不暴露给学习者；
//   - 结束一局吐 done 事件（终止奖励：赢 +1 / 输 -1）。
const (
	envProtocol = "wbo-env/1"
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
				// v1：选择（target/mode）不暴露给学习者，由环境按 greedy 规则回答。
				answer := (&ai.Greedy{}).Answer(view, *pending)
				result := e.session.Resume(answer)
				if result.Status != runner.StatusCompleted && result.Status != runner.StatusSuspended {
					return envEvent{}, fmt.Errorf("自动选择被拒绝: %s", result.ErrorCode)
				}
				continue
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

// step 按下标提交一个主动作。
func (e *envSession) step(cards *ir.CardPack, index int) error {
	view, err := e.session.View(e.learner)
	if err != nil {
		return err
	}
	if view.GameOver {
		return fmt.Errorf("对局已结束")
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
