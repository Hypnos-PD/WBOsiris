package nativebroker

import (
	"encoding/base64"
	"log"
	"time"

	"github.com/vmihailenco/msgpack/v5"

	"wbo/internal/nativeproto"
)

// FixtureHandler 用引导夹具回应客户端。
//
// 它只覆盖"启动阶段"的路由（版本查询、标题、账号初始化……）——这些响应不依赖规则引擎，
// 作用只是让客户端走到主界面。对局相关的路由仍然交回 501，等引擎那一层接上。
//
// 响应同样是加密的：先把夹具编成 MessagePack，再用**本次会话**的密钥封成信封，最后
// 按客户端期望的形式（base64 文本）发出。所以没有解开报文（没有会话状态）时它答不了，
// 会退回未实现。
type FixtureHandler struct {
	fixtures *nativeproto.Fixtures
	logger   *log.Logger
}

// NewFixtureHandler 构造一个夹具处理器。
func NewFixtureHandler(fixtures *nativeproto.Fixtures, logger *log.Logger) *FixtureHandler {
	return &FixtureHandler{fixtures: fixtures, logger: logger}
}

// Route 实现 Handler。
func (h *FixtureHandler) Route(request *Request) (Response, bool) {
	if h == nil || h.fixtures == nil || request == nil {
		return Response{}, false
	}
	if request.Envelope == nil {
		// 没有会话就封不出客户端能解的响应——如实说"没实现"，不要发一个空包。
		return Response{}, false
	}
	fixture, ok := h.fixtures.Lookup(request.Path)
	if !ok {
		return Response{}, false
	}
	// 运行期字段：客户端会用它判断服务器时间是否合理。
	if headers, ok := fixture["data_headers"].(map[string]any); ok {
		headers["servertime"] = time.Now().Unix()
	}
	plain, err := msgpack.Marshal(fixture)
	if err != nil {
		h.logger.Printf("夹具 %s 编码失败：%v", request.Path, err)
		return Response{}, false
	}
	sealed, err := request.Envelope.EncodeResponse(plain)
	if err != nil {
		h.logger.Printf("夹具 %s 封包失败：%v", request.Path, err)
		return Response{}, false
	}
	return Response{
		Status:      200,
		ContentType: "application/octet-stream",
		Body:        []byte(base64.StdEncoding.EncodeToString(sealed)),
	}, true
}
