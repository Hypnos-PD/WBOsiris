package runner

import (
	"sync"

	"wbo/internal/ir"
)

// runtimeIndex 是"只随卡池变化"的派生数据：IR 块索引与卡池指纹。
//
// 它们原本在每次新建会话（开局）与每次恢复续局时重算一遍：实测约占
// 新建会话成本的绝大部分（reset 约 90 ms/局），也占恢复续局的 ~99%。
// 搜索、自对弈、断线重连都要频繁走这两条路，所以这里按卡池对象缓存一份。
//
// 卡池在进程内是不可变的（编译一次、只读使用），因此缓存是安全的；
// 缓存以指针为键并限制条目数，避免测试里反复构建卡池时无限增长。
type runtimeIndex struct {
	blocks   map[string][]ir.Effect
	packHash string
}

const runtimeIndexCacheLimit = 4

var (
	runtimeIndexMu     sync.Mutex
	runtimeIndexByPack = map[*ir.CardPack]*runtimeIndex{}
	runtimeIndexOrder  = make([]*ir.CardPack, 0, runtimeIndexCacheLimit)
)

// runtimeIndexFor 返回卡池对应的派生数据，必要时计算并缓存。
// pack 为 nil 时不缓存，只计算一次（兼容只传卡牌索引的老调用）。
func runtimeIndexFor(pack *ir.CardPack, index map[int]*ir.Card) (*runtimeIndex, error) {
	if pack != nil {
		runtimeIndexMu.Lock()
		entry, ok := runtimeIndexByPack[pack]
		runtimeIndexMu.Unlock()
		if ok {
			return entry, nil
		}
	}
	blocks, err := indexBlocks(index)
	if err != nil {
		return nil, err
	}
	packHash, err := runtimeCardPackHash(pack, index)
	if err != nil {
		return nil, err
	}
	entry := &runtimeIndex{blocks: blocks, packHash: packHash}
	if pack == nil {
		return entry, nil
	}
	runtimeIndexMu.Lock()
	if len(runtimeIndexOrder) >= runtimeIndexCacheLimit {
		evicted := runtimeIndexOrder[0]
		runtimeIndexOrder = runtimeIndexOrder[1:]
		delete(runtimeIndexByPack, evicted)
	}
	runtimeIndexByPack[pack] = entry
	runtimeIndexOrder = append(runtimeIndexOrder, pack)
	runtimeIndexMu.Unlock()
	return entry, nil
}
