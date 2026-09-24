package nativebroker

import (
	"encoding/base64"

	"wbo/internal/nativeproto"
)

// sealResponse 把明文封成客户端期待的形式：加密信封 + base64 文本。
//
// **响应体是 base64 文本，不是二进制**——这一点有实际代价：第一版档案处理器直接回了
// 二进制信封，客户端的表现是弹"服务器发生错误（错误代码 3）"，看起来像业务数据不对，
// 实际是它在对响应体做 base64 解码。请求体也是 base64 文本，两边一致。
func sealResponse(envelope *nativeproto.Request, plain []byte) ([]byte, error) {
	sealed, err := envelope.EncodeResponse(plain)
	if err != nil {
		return nil, err
	}
	encoded := make([]byte, base64.StdEncoding.EncodedLen(len(sealed)))
	base64.StdEncoding.Encode(encoded, sealed)
	return encoded, nil
}
