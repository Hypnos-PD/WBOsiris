package nativebroker

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/vmihailenco/msgpack/v5"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	"wbo/internal/nativeproto"
)

// Request 是一次已经读完整的请求，交给路由处理。
type Request struct {
	Method  string
	Path    string
	Query   string
	Headers http.Header
	Body    []byte
	// Plain 只在配置了 Decoder 且解开成功时非空，是客户端请求的明文（MessagePack）。
	Plain []byte
	// Envelope 是解开后的信封信息（凭据片段、客户端公钥等），便于诊断。
	Envelope *nativeproto.Request
	// BinaryBody 表示这次请求体是**原始二进制**而不是 base64 文本。
	//
	// 正常客户端发 base64 文本；但把客户端里那个"压缩请求体"的函数改成直通
	// （delta 的做法，也是我们绕开它 LZ4 崩溃的唯一办法）之后，base64 这一步也跟着
	// 没了，body 直接是二进制。响应必须按同样的形式回，否则客户端解不开。
	BinaryBody bool
	// PlainMessagePack 表示这次请求体就是明文 MessagePack，**没有信封**。
	//
	// 客户端里那个"压缩请求体"的函数被改成直通之后（delta 的做法：`mov rax,rdx;ret`），
	// 压缩、加密、base64 三步一起没了——实测客户端直接发
	// `82 a4 "uuid" a0 a9 "client_id" 01` 这样的裸 MessagePack。那种情况下服务端
	// 就用明文应答（同样不带信封），既省掉整套密码学，也不用再去猜注册包的密钥派生。
	PlainMessagePack bool
}

// Response 是路由的结果。ContentType 留空时按 MessagePack 返回。
type Response struct {
	Status      int
	ContentType string
	Body        []byte
}

// Handler 处理一条已经实现的路由。ok 为 false 表示"还没实现"。
//
// 先有接口再有实现：第一条真实流量录下来之后，实现哪条路由由数据决定，而不是
// 由我们猜。
type Handler interface {
	Route(request *Request) (response Response, ok bool)
}

// Decoder 把请求体按客户端信封解开。
//
// 抽成接口是为了让 broker 能脱离真实加密被测试——收发与路由本来就不该依赖密码学细节。
// *nativeproto.Server 就是它的实现。
type Decoder interface {
	DecodeRequest(wire []byte, auth nativeproto.Auth) (*nativeproto.Request, error)
	// DecodeEnvelope 在载荷解不开时也能给出"只差载荷"的那份请求：响应密钥只由
	// shared/sid/selector 派生，夹具类路由不需要请求体，照样答得出来。
	DecodeEnvelope(wire []byte, auth nativeproto.Auth) (*nativeproto.Request, error)
}

// Options 是启动参数。
type Options struct {
	// Listen 是监听地址，默认 127.0.0.1:50172（与客户端改写目标一致）。
	Listen string
	// CaptureDir 非空时把每个请求落盘。
	CaptureDir string
	// Handler 可以为空，此时所有路由都按未实现处理。
	Handler Handler
	// Decoder 非空时，会尝试把 POST 请求体按客户端信封解开。
	//
	// 客户端报文是加密的：能解开的前提是客户端拿**我们的**公钥协商过（即它的常量
	// 已被换成我们的公钥）。解不开不是错误——那说明这一版客户端还没被换过。
	Decoder Decoder
	// Auth 是解开报文所需的客户端凭据（sid 从请求头取，其余由调用方提供）。
	Auth nativeproto.Auth
	// Log 可以注入日志器，方便测试闭嘴。
	Log *log.Logger
	// Ready 在监听成功后回调一次，参数是真实监听地址（测试里端口是 0）。
	Ready func(address string)
}

const defaultListen = "127.0.0.1:50172"

// bodyLimit 是单次请求体的上限。客户端上传的是牌组和对局消息，几 MiB 足够，
// 这里给到 64 MiB 是为了不让一次异常请求把进程拖垮。
const bodyLimit = 64 << 20

// Serve 启动 h2c 服务并阻塞到 ctx 结束。
//
// 必须是 h2c（明文 HTTP/2）：客户端被改写到 http:// 之后，仍然按 HTTP/2 先验知识
// 直连，没有 TLS 握手也没有 Upgrade 协商。
func Serve(ctx context.Context, options Options) error {
	if options.Listen == "" {
		options.Listen = defaultListen
	}
	logger := options.Log
	if logger == nil {
		logger = log.Default()
	}
	sink, err := newCapture(options.CaptureDir)
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", options.Listen)
	if err != nil {
		return err
	}
	defer listener.Close()
	if options.Ready != nil {
		options.Ready(listener.Addr().String())
	}
	logger.Printf("native broker 监听 %s（h2c）", listener.Addr())
	if options.CaptureDir != "" {
		logger.Printf("请求录制到 %s", options.CaptureDir)
	}

	server := &http.Server{
		Handler: h2c.NewHandler(&gateway{
			handler: options.Handler,
			capture: sink,
			logger:  logger,
			decoder: options.Decoder,
			auth:    options.Auth,
		}, &http2.Server{}),
		// 客户端的流式对局连接是长连接，别让空闲超时把对局掐了。
		IdleTimeout:       30 * time.Minute,
		ReadHeaderTimeout: 30 * time.Second,
		BaseContext:       func(net.Listener) context.Context { return ctx },
	}
	finished := make(chan error, 1)
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			finished <- err
			return
		}
		finished <- nil
	}()
	select {
	case err := <-finished:
		return err
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return server.Shutdown(shutdown)
	}
}

type gateway struct {
	handler Handler
	capture *capture
	logger  *log.Logger
	decoder Decoder
	auth    nativeproto.Auth
}

func (g *gateway) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	// 请求体必须完整读出来：既要录制，也要交给路由，读两次不如读一次。
	body, err := io.ReadAll(io.LimitReader(request.Body, bodyLimit))
	if err != nil {
		http.Error(writer, "read failed", http.StatusBadRequest)
		return
	}
	_ = request.Body.Close()
	message := &Request{
		Method:  request.Method,
		Path:    request.URL.Path,
		Query:   request.URL.RawQuery,
		Headers: request.Header.Clone(),
		Body:    body,
	}
	// 能解就解：解开的明文是后续实现路由的前提。解不开只记一行，不影响录制与回应。
	if g.decoder != nil && len(body) > 0 {
		sid, err := decodeSID(request.Header.Get("Sid"))
		switch {
		case err != nil:
			g.logger.Printf("信封：Sid 头无法解析：%v", err)
		default:
			auth := g.auth
			auth.SID = sid
			wire, binary := unwrapBody(body)
			message.BinaryBody = binary
			// 先按信封解（正常客户端）；解不开再看是不是"裸 MessagePack"——客户端里
			// 那个压缩函数被改成直通之后，压缩/加密/base64 三步一起没了，body 就是
			// 明文的请求体本身。顺序不能反：明文判断只看首字节，撞上信封的几率虽小，
			// 但"先解信封"永远更可靠。
			if decoded, err := decodeBody(g.decoder, wire, auth); err == nil {
				message.Plain = decoded.Plain
				message.Envelope = decoded
				head, value := decoded.CheckAuth(auth)
				g.logger.Printf("信封：解出 %d 字节明文（凭据自检 head=%v value=%v）",
					len(decoded.Plain), head, value)
				g.logger.Printf("  请求 %s → %s", message.Path, describePlain(decoded.Plain))
			} else if isPlainMessagePack(wire) {
				message.Plain = wire
				message.PlainMessagePack = true
				g.logger.Printf("信封：没有信封（明文 MessagePack，%d 字节）", len(wire))
				g.logger.Printf("  请求 %s → %s", message.Path, describePlain(wire))
			} else {
				g.logger.Printf("信封：解不开（%v）", err)
				// 载荷解不开不代表答不上话：响应密钥只由 shared/sid/selector 派生，
				// 这三样在解载荷之前就齐了。夹具类路由（不含请求体）照样能给出正确
				// 的响应——注册那条请求就是这种情况。
				if partial, _ := g.decoder.DecodeEnvelope(wire, auth); partial != nil {
					message.Plain = nil
					message.Envelope = partial
					g.logger.Printf("  请求 %s → 只凭元数据应答（请求体没解开）", message.Path)
				}
			}
		}
	}
	response, handled := Response{}, false
	if g.handler != nil {
		response, handled = g.handler.Route(message)
	}
	if !handled {
		// 未实现就明确说未实现，不要伪造成功：第一次跑的目的是看清客户端问什么，
		// 一个诚实的 501 比一个编出来的响应更有用。
		response = Response{Status: http.StatusNotImplemented, Body: nil}
	}
	if response.Status == 0 {
		response.Status = http.StatusOK
	}
	contentType := response.ContentType
	if contentType == "" {
		contentType = "application/x-msgpack"
	}
	writer.Header().Set("Content-Type", contentType)
	writer.WriteHeader(response.Status)
	if len(response.Body) > 0 {
		_, _ = writer.Write(response.Body)
	}
	// CONNECT 是代理请求：路径为空，目标在 Host 里。Unity 自带的 HTTP 客户端
	// （libcurl）会认 HTTP(S)_PROXY 环境变量，走这条路就能把真实主机名读出来，
	// 不需要解密任何东西。
	target := message.Path + querySuffix(message.Query)
	if message.Method == http.MethodConnect {
		target = request.Host
	}
	g.logger.Printf("%s %s → %d（%d 字节请求，%s）", message.Method, target,
		response.Status, len(body), handledTag(handled))
	if g.capture != nil {
		record := &Record{
			Method:   message.Method,
			Path:     message.Path,
			Query:    message.Query,
			Protocol: request.Proto,
			Headers:  message.Headers,
			Status:   response.Status,
			Handled:  handled,
		}
		if err := g.capture.write(record, body, response.Body); err != nil {
			g.logger.Printf("录制失败：%v", err)
		}
	}
}

func querySuffix(query string) string {
	if query == "" {
		return ""
	}
	return "?" + query
}

// decodeSID 把请求头里的 Sid（32 位十六进制）转成 16 字节。
func decodeSID(value string) ([]byte, error) {
	raw, err := hex.DecodeString(value)
	if err != nil {
		return nil, err
	}
	if len(raw) != 16 {
		return nil, fmt.Errorf("Sid 应为 16 字节，实际 %d", len(raw))
	}
	return raw, nil
}

// unwrapBody 认出请求体是 base64 文本还是原始二进制。
//
// 正常客户端发 base64 文本；客户端被打了"压缩直通"补丁之后（见 Request.BinaryBody）
// 就是二进制。两种都要认，否则换了客户端就"什么都收不到"。
func unwrapBody(body []byte) ([]byte, bool) {
	trimmed := bytes.TrimSpace(body)
	wire := make([]byte, base64.StdEncoding.DecodedLen(len(trimmed)))
	count, err := base64.StdEncoding.Strict().Decode(wire, trimmed)
	if err == nil {
		return wire[:count], false
	}
	return trimmed, true
}

// isPlainMessagePack 判断这段字节是不是一个可以解开的 MessagePack 对象。
//
// 判据很保守：能解成 map[string]any 才算。加密信封是随机字节，几乎不可能通过。
func isPlainMessagePack(raw []byte) bool {
	if len(raw) == 0 || raw[0] < 0x80 || raw[0] > 0x8F { // 只认固定长度的 map
		return false
	}
	var value map[string]any
	return msgpack.Unmarshal(raw, &value) == nil && value != nil
}

// decodeBody 解开一个已经拆掉外层编码的报文。
//
// 解不开时会用"全零 uuid"再试一次：**注册（/Account/signUp）那次请求就是这个形状**
// ——客户端还没有账号，密钥派生里用的是一个全零 uuid，而调用方手里只有配对后那份
// 凭据。实测注册包里 "元数据里的凭据与 authKey 一致、共享密钥也对"，只是 uuid 不同，
// 所以这里补一次重试是有依据的，不是碰运气。
func decodeBody(decoder Decoder, wire []byte, auth nativeproto.Auth) (*nativeproto.Request, error) {
	request, err := decoder.DecodeRequest(wire, auth)
	if err == nil {
		return request, nil
	}
	fresh := auth
	fresh.UUID = make([]byte, 16)
	if retried, retryErr := decoder.DecodeRequest(wire, fresh); retryErr == nil {
		return retried, nil
	}
	return nil, err
}

func handledTag(handled bool) string {
	if handled {
		return "已实现"
	}
	return "未实现"
}

// Describe 给出一条请求的一行摘要，便于在日志里扫。
func Describe(request *Request) string {
	return fmt.Sprintf("%s %s%s (%d 字节, %s)", request.Method, request.Path, querySuffix(request.Query),
		len(request.Body), strings.TrimSpace(request.Headers.Get("Content-Type")))
}
