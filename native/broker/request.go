package nativebroker

import (
	"github.com/vmihailenco/msgpack/v5"
)

// decodeRequest 把解开的请求体（MessagePack）读成 map。
//
// 解不开就回空 map：档案层看到缺字段会按"请求不合法"拒绝，而这里不该因为解析失败
// 把整条请求打成 500。
func decodeRequest(plain []byte) map[string]any {
	if len(plain) == 0 {
		return map[string]any{}
	}
	var value map[string]any
	if err := msgpack.Unmarshal(plain, &value); err != nil || value == nil {
		return map[string]any{}
	}
	return value
}
