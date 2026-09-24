package nativebroker

import (
	"encoding/base64"
	"github.com/vmihailenco/msgpack/v5"

	"wbo/internal/nativeproto"
)

// encodePayload 按请求的形态回响应：
//
//   - 有信封（正常客户端）：封成信封，再按请求的编码（base64 文本 / 二进制）回；
//   - 没有信封（客户端被改成不压缩，直接发明文 MessagePack）：回明文 MessagePack。
//     客户端那边既然省掉了压缩与加密，也就不会再解开信封。
func encodePayload(request *Request, payload map[string]any) ([]byte, error) {
	plain, err := msgpack.Marshal(payload)
	if err != nil {
		return nil, err
	}
	if request.PlainMessagePack {
		return plain, nil
	}
	return sealResponse(request.Envelope, plain, request.BinaryBody)
}

// sealResponse 把明文封成客户端期待的形式：加密信封 + 与请求相同的编码。
//
// **响应体是 base64 文本，不是二进制**——这一点有实际代价：第一版档案处理器直接回了
// 二进制信封，客户端的表现是弹"服务器发生错误（错误代码 3）"，看起来像业务数据不对，
// 实际是它在对响应体做 base64 解码。请求体也是 base64 文本，两边一致。
//
// 例外是"客户端被打了压缩直通补丁"那条路：它的请求体是二进制，base64 那一步在客户端
// 内部也被跳过了，所以响应也得回二进制（binary=true）。跟着请求走，不要写死。
func sealResponse(envelope *nativeproto.Request, plain []byte, binary bool) ([]byte, error) {
	sealed, err := envelope.EncodeResponse(plain)
	if err != nil {
		return nil, err
	}
	if binary {
		return sealed, nil
	}
	encoded := make([]byte, base64.StdEncoding.EncodedLen(len(sealed)))
	base64.StdEncoding.Encode(encoded, sealed)
	return encoded, nil
}
