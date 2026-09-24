package nativebroker

import (
	"encoding/json"
	"fmt"

	"github.com/vmihailenco/msgpack/v5"
)

// describePlain 把解开的请求体（MessagePack）写成一行人能读的 JSON。
//
// 只用于日志：解不开、太长、或者不是 MessagePack 都只回一句说明，绝不因为"记日志"
// 这件事影响请求的处理。牌组编辑那种几 KB 的请求也只截断显示。
func describePlain(plain []byte) string {
	const limit = 4096
	var value any
	if err := msgpack.Unmarshal(plain, &value); err != nil {
		return fmt.Sprintf("（不是 MessagePack：%v，原始 %d 字节）", err, len(plain))
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("（无法转成 JSON：%v）", err)
	}
	if len(encoded) > limit {
		return string(encoded[:limit]) + "…（已截断）"
	}
	return string(encoded)
}
