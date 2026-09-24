package nativebroker

import (
	"log"
	"time"

	"github.com/vmihailenco/msgpack/v5"

	"wbo/internal/nativeprofile"
	"wbo/internal/nativeproto"
)

// Chain 按顺序试每个处理器，第一个说"实现了"的胜出。
//
// 档案路由与启动夹具是两件事：夹具是固定响应，档案要按请求算。分开写、串起来用，
// 比在一个函数里 if/else 到底好读。
func Chain(handlers ...Handler) Handler {
	return chained(handlers)
}

type chained []Handler

func (c chained) Route(request *Request) (Response, bool) {
	for _, handler := range c {
		if handler == nil {
			continue
		}
		if response, ok := handler.Route(request); ok {
			return response, true
		}
	}
	return Response{}, false
}

// ProfileHandler 回答玩家档案相关的路由。
//
// 它现在只做一件事，但这件事不做客户端就走不下去：/Load/index 里的
// owned_base_card_ids 必须是玩家真实持有的 base 卡 id。启动夹具里这一项是空数组，
// 实测客户端拿到空收藏会**退回标题界面**——每次点击都重跑一遍启动链，然后回到标题。
//
// 其余档案路由（/Card/getList、/Deck/getList……）按 delta 的做法办：套用
// /Load/index 那个响应的外壳（data_headers 是通用的），只换 data。这一段随后按
// 客户端实际问到的顺序补，不预先猜。
type ProfileHandler struct {
	profile  *nativeprofile.Profile
	fixtures *nativeproto.Fixtures
	logger   *log.Logger
}

// NewProfileHandler 构造档案处理器。profile 为空时它什么也不做。
func NewProfileHandler(profile *nativeprofile.Profile, fixtures *nativeproto.Fixtures,
	logger *log.Logger) *ProfileHandler {
	return &ProfileHandler{profile: profile, fixtures: fixtures, logger: logger}
}

// Route 实现 Handler。
func (h *ProfileHandler) Route(request *Request) (Response, bool) {
	if h == nil || h.profile == nil || h.fixtures == nil || request == nil {
		return Response{}, false
	}
	if request.Envelope == nil {
		// 没有会话就封不出客户端解得开的响应。
		return Response{}, false
	}
	route := nativeproto.NormalizeRoute(request.Path)
	var payload map[string]any
	switch route {
	case "/Load/index":
		fixture, ok := h.fixtures.Lookup(route)
		if !ok {
			return Response{}, false
		}
		data, ok := fixture["data"].(map[string]any)
		if !ok {
			return Response{}, false
		}
		data["owned_base_card_ids"] = h.profile.OwnedBaseCardIDs()
		payload = fixture
	default:
		if !h.profile.HandledRoute(route) {
			return Response{}, false
		}
		// 档案路由的响应外壳沿用 /Load/index 那一份：data_headers 是通用的，
		// 只有 data 按请求算——这是 delta 的做法，客户端也接受。
		envelope, ok := h.fixtures.Lookup("/Load/index")
		if !ok {
			return Response{}, false
		}
		answer, err := h.profile.Handle(route, decodeRequest(request.Plain))
		if err != nil {
			// 请求不合法就如实拒绝：伪造一个"成功"会让客户端进到更奇怪的状态。
			h.logger.Printf("档案 %s 拒绝：%v", route, err)
			return Response{}, false
		}
		if answer == nil {
			return Response{}, false
		}
		envelope["data"] = answer
		payload = envelope
	}
	if payload == nil {
		return Response{}, false
	}
	if headers, ok := payload["data_headers"].(map[string]any); ok {
		headers["servertime"] = time.Now().Unix()
	}
	plain, err := msgpack.Marshal(payload)
	if err != nil {
		h.logger.Printf("档案 %s 编码失败：%v", route, err)
		return Response{}, false
	}
	body, err := sealResponse(request.Envelope, plain)
	if err != nil {
		h.logger.Printf("档案 %s 封包失败：%v", route, err)
		return Response{}, false
	}
	return Response{Status: 200, ContentType: "application/octet-stream", Body: body}, true
}
