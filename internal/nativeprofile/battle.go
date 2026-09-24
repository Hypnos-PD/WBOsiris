package nativeprofile

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"time"
)

// 练习对局的入口：`/Practice/battleStart`。
//
// 这一条和别的档案路由不同——它要决定"这一局怎么开"：先后手、stage、两边的牌组，
// 并且客户端拿到响应里的 `battle_url` 之后会**立刻**去连对局通道。所以这里做三件事：
//
//  1. 校验两边都是本地已有的完整牌组（40 张、error_state 为 0、格式与请求一致）；
//  2. 定先后手与 stage，把响应按 delta 的 `local_native_battle.py` 那套形状拼出来；
//  3. **先**把这一局的初始快照落盘，再把响应交出去——顺序反了，对局层会读到上一局。
//
// 洗牌与起手是我们（服务端）定的：客户端只认快照，不认官方发牌算法。delta 用的是
// `time.time_ns()` 当种子 + `random.Random`，这里同样是"每次不同、但可复现"。
const (
	// DefaultBattleURL 是对局通道的默认地址。delta 用的是 50171；WBO 的对局层
	// 还没起，先用同一个约定，由调用方决定实际监听哪里。
	DefaultBattleURL = "http://127.0.0.1:50171"
	// practiceStageID 来自 delta 对 `ast_battle_tutorial_10002` 夹具的实测（stage 6），
	// 不是猜的。
	practiceStageID = 6
	// handSize 是起手张数（先手四张，与客户端一开局显示的手牌一致）。
	handSize = 4
)

// BattleSnapshot 是这一局的初始状态。
//
// 字段名与 delta 的 `local_native_battle.py` 快照保持一致（`schema`/`first`/`stage`/
// `decks` 那些），这样两边的对局层可以对着读。牌组整份留在里面，是因为后面对局层
// 要用它回放、校验与结算。
type BattleSnapshot struct {
	Schema      int              `json:"schema"`
	Source      string           `json:"source"`
	ID          int64            `json:"id"`
	Seed        int64            `json:"seed"`
	First       int64            `json:"first"`
	Stage       int64            `json:"stage"`
	Phase       string           `json:"phase"`
	Decks       []map[string]any `json:"decks"`
	Hands       [][]int64        `json:"hands"`
	DrawPiles   [][]int64        `json:"draw_piles"`
	Turn        int64            `json:"turn"`
	TurnCounts  []int64          `json:"turn_counts"`
	PP          []int64          `json:"pp"`
	MaxPP       []int64          `json:"max_pp"`
	Life        []int64          `json:"life"`
	Active      int64            `json:"active"`
	CreatedUnix int64            `json:"created_unix"`
}

// SetBattleURL 覆盖响应里给客户端的对局通道地址（空值忽略，保持默认）。
func (p *Profile) SetBattleURL(url string) {
	if url != "" {
		p.battleURL = url
	}
}

// BattleURL 报告当前使用的对局通道地址。
func (p *Profile) BattleURL() string {
	if p.battleURL == "" {
		return DefaultBattleURL
	}
	return p.battleURL
}

// battleDir 是快照目录（与 profile.json 同级的 battle/）。没有落盘路径时返回空。
func (p *Profile) battleDir() string {
	if p.store == nil || p.store.path == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(p.store.path), "battle")
}

// startPracticeBattle 是 `/Practice/battleStart` 的本体。
func (p *Profile) startPracticeBattle(state *State, request map[string]any) (map[string]any, error) {
	practiceType, err := asInt(request["practice_type"])
	if err != nil {
		return nil, err
	}
	// 只有"用本地已有牌组当对手"这一种练习模式能离线跑：另一种要官方预设牌组，
	// 我们没有那份数据，猜只会给出一个看起来对、其实不对的对局。
	if practiceType != p.enumValue("PracticeType", "Myself") {
		return nil, ProfileError{Reason: fmt.Sprintf("练习模式 %d 不支持（只支持用本地已有牌组当对手）", practiceType)}
	}

	deckID, deckFormat, targetID, targetDeckFormat, err := battleDeckFields(request)
	if err != nil {
		return nil, err
	}
	decks := make([]map[string]any, 0, 2)
	for _, spec := range []struct {
		id     int64
		format int64
		label  string
	}{
		{deckID, deckFormat, "玩家牌组"},
		{targetID, targetDeckFormat, "对手牌组"},
	} {
		deck, err := lookupDeck(state, spec.id)
		if err != nil {
			return nil, err
		}
		if err := p.validateDeck(deck); err != nil {
			return nil, err
		}
		errorState, _ := asInt(deck["error_state"])
		if errorState != 0 {
			return nil, ProfileError{Reason: spec.label + "不是完整的 40 张"}
		}
		if actual, _ := asInt(deck["deck_format"]); actual != spec.format {
			return nil, ProfileError{Reason: fmt.Sprintf("%s的格式与请求不一致（%d != %d）", spec.label, actual, spec.format)}
		}
		decks = append(decks, deck)
	}

	order, err := asInt(request["battle_order"])
	if err != nil {
		return nil, err
	}
	if order < 0 || order > 2 {
		return nil, ProfileError{Reason: fmt.Sprintf("先后手 %d 不认识", order)}
	}

	seed := time.Now().UnixNano()
	// 先后手与洗牌分开取种子：两者用同一个随机源会让"随机先后手"的第一局和第二局
	// 出现同样的起手。
	orderRandom := rand.New(rand.NewSource(seed))
	first := order
	if first == 0 {
		first = int64(1 + orderRandom.Intn(2))
	}
	shuffle := rand.New(rand.NewSource(seed ^ 0x5f5f5f5f))

	snapshot := &BattleSnapshot{
		Schema:      2,
		Source:      "wbo-native-profile",
		ID:          time.Now().UnixMilli(),
		Seed:        seed,
		First:       first,
		Stage:       practiceStageID,
		Phase:       "mulligan",
		Decks:       decks,
		Turn:        0,
		TurnCounts:  []int64{0, 0},
		PP:          []int64{0, 0},
		MaxPP:       []int64{0, 0},
		Life:        []int64{20, 20},
		Active:      first - 1,
		CreatedUnix: time.Now().Unix(),
	}
	for _, deck := range decks {
		cards, err := battleDeckCards(deck)
		if err != nil {
			return nil, err
		}
		shuffle.Shuffle(len(cards), func(i, j int) { cards[i], cards[j] = cards[j], cards[i] })
		if len(cards) < handSize {
			return nil, ProfileError{Reason: "牌组张数不足，发不出起手"}
		}
		snapshot.Hands = append(snapshot.Hands, append([]int64{}, cards[:handSize]...))
		snapshot.DrawPiles = append(snapshot.DrawPiles, append([]int64{}, cards[handSize:]...))
	}

	// 先落盘再回响应：客户端拿到 battle_id 就会去连 battle_url。
	if err := p.writeBattleSnapshot(snapshot); err != nil {
		return nil, err
	}

	response, err := p.copyRouteData("/Practice/battleStart")
	if err != nil {
		return nil, err
	}
	response["battle_id"] = snapshot.ID
	response["battle_url"] = p.BattleURL()
	info, err := p.copyTemplate("BattleStartInfo")
	if err != nil {
		return nil, err
	}
	info["first_turn_client_id"] = first
	info["stage_id"] = snapshot.Stage
	response["battle_info"] = info

	users := make([]any, 0, len(decks))
	for side, deck := range decks {
		user, err := p.copyTemplate("BattleStartUserInfo")
		if err != nil {
			return nil, err
		}
		classID, _ := asInt(deck["class_id"])
		leaderSkin, err := p.battleLeaderSkin(deck, shuffle)
		if err != nil {
			return nil, err
		}
		user["client_id"] = int64(side + 1)
		user["user_name"] = battleUserName(side)
		user["rank"] = int64(1)
		// 纹章 id 的规律（210000001 + 职业 × 1000）是从 delta 的
		// local_native_battle.py 抄的，对得上客户端显示的默认纹章。
		user["emblem_id"] = 210000001 + classID*1000
		user["class_id"] = classID
		user["leader_skin_id"] = leaderSkin
		user["cpu_level"] = int64(0)
		users = append(users, user)
	}
	response["battle_users"] = users
	return response, nil
}

// battleDeckFields 取出请求里的四张牌组相关字段。
func battleDeckFields(request map[string]any) (deckID, deckFormat, targetID, targetDeckFormat int64, err error) {
	if deckID, err = asInt(request["deck_id"]); err != nil {
		return
	}
	if deckFormat, err = asInt(request["deck_format"]); err != nil {
		return
	}
	if targetID, err = asInt(request["target_id"]); err != nil {
		return
	}
	targetDeckFormat, err = asInt(request["target_deck_format"])
	return
}

// battleDeckCards 把牌组的 40 张摊平成一串卡 id。
func battleDeckCards(deck map[string]any) ([]int64, error) {
	var cards []int64
	for _, entry := range deckEntries(deck) {
		id, err := asInt(entry["id"])
		if err != nil {
			return nil, err
		}
		count, err := asInt(entry["num"])
		if err != nil {
			return nil, err
		}
		if count < 0 || count > 3 {
			return nil, ProfileError{Reason: fmt.Sprintf("卡片 %d 的张数 %d 不合法", id, count)}
		}
		for i := int64(0); i < count; i++ {
			cards = append(cards, id)
		}
	}
	return cards, nil
}

// battleLeaderSkin 决定这一侧用哪个主战者皮肤：牌组指定了就照用，"随机"则从牌组
// 允许的列表里挑一个。
func (p *Profile) battleLeaderSkin(deck map[string]any, source *rand.Rand) (int64, error) {
	random, _ := deck["is_random_leader_skin"].(bool)
	if !random {
		return asInt(deck["leader_skin_id"])
	}
	raw, _ := deck["leader_skin_id_list"].([]any)
	candidates := make([]int64, 0, len(raw))
	for _, item := range raw {
		id, err := asInt(item)
		if err != nil {
			continue
		}
		candidates = append(candidates, id)
	}
	if len(candidates) == 0 {
		return asInt(deck["leader_skin_id"])
	}
	return candidates[source.Intn(len(candidates))], nil
}

func battleUserName(side int) string {
	if side == 0 {
		return "离线玩家"
	}
	return "练习电脑"
}

// writeBattleSnapshot 原子写入这一局的快照。
func (p *Profile) writeBattleSnapshot(snapshot *BattleSnapshot) error {
	directory := p.battleDir()
	if directory == "" {
		// 没有落盘路径（测试或纯内存模式）：不写文件，但也不假装写过了。
		return nil
	}
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return err
	}
	encoded, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return err
	}
	pending := filepath.Join(directory, "current.pending")
	if err := os.WriteFile(pending, append(encoded, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(pending, filepath.Join(directory, "current.json"))
}

// LoadBattleSnapshot 读回当前这一局（对局层与调试用）。
func (p *Profile) LoadBattleSnapshot() (*BattleSnapshot, error) {
	directory := p.battleDir()
	if directory == "" {
		return nil, ProfileError{Reason: "没有落盘路径，读不到对局快照"}
	}
	raw, err := os.ReadFile(filepath.Join(directory, "current.json"))
	if err != nil {
		return nil, err
	}
	var snapshot BattleSnapshot
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return nil, err
	}
	return &snapshot, nil
}
