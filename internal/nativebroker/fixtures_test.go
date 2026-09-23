package nativebroker

import (
	"bytes"
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"io"
	"log"
	"net/http"
	"strings"
	"testing"

	"github.com/vmihailenco/msgpack/v5"

	"wbo/internal/nativeproto"
)

// 客户端常量的后 20 字节（参与密钥派生与认证，不随公钥替换而变）。
const commonTailHex = "6854C854E96E762E4F18C9F7370608BE437ECBBA"

// 造一套自洽的会话：客户端常量被换成了服务端公钥（即客户端内存改写后的状态）。
func testSession(t *testing.T) (serverKey, header []byte, auth nativeproto.Auth) {
	t.Helper()
	key, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	serverKey = key.Bytes()
	stored := key.PublicKey().Bytes()
	for i, j := 0, len(stored)-1; i < j; i, j = i+1, j-1 {
		stored[i], stored[j] = stored[j], stored[i]
	}
	tail, err := hex.DecodeString(commonTailHex)
	if err != nil {
		t.Fatal(err)
	}
	header = append(append([]byte(nil), stored...), tail...)
	auth = nativeproto.Auth{
		SID:     bytes.Repeat([]byte{0x11}, 16),
		UUID:    bytes.Repeat([]byte{0x22}, 16),
		AuthKey: bytes.Repeat([]byte{0x33}, 50),
	}
	return serverKey, header, auth
}

// 端到端：客户端方向造一个加密请求 → broker 解开并按夹具应答 → 客户端解出响应。
//
// 这是"本地服务器真的回答了客户端"的最小完整证据链，覆盖传输（h2c）、信封（两端）、
// 以及夹具路由。
func TestFixtureHandlerAnswersBootstrapRoute(t *testing.T) {
	serverKey, header, auth := testSession(t)
	client, err := nativeproto.NewClient(auth, header)
	if err != nil {
		t.Fatal(err)
	}
	wire, session, err := client.EncodeRequest([]byte{0x80})
	if err != nil {
		t.Fatal(err)
	}
	server, err := nativeproto.NewServer(serverKey, header)
	if err != nil {
		t.Fatal(err)
	}
	fixtures, err := nativeproto.LoadFixtures()
	if err != nil {
		t.Fatal(err)
	}
	logger := log.New(io.Discard, "", 0)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ready := make(chan string, 1)
	go func() {
		_ = Serve(ctx, Options{
			Listen:  "127.0.0.1:0",
			Decoder: server,
			Auth:    auth,
			Handler: NewFixtureHandler(fixtures, logger),
			Log:     logger,
			Ready:   func(address string) { ready <- address },
		})
	}()
	address := <-ready

	request, err := http.NewRequest(http.MethodPost, "http://"+address+"/cygames/Version/info",
		strings.NewReader(base64.StdEncoding.EncodeToString(wire)))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Sid", hex.EncodeToString(auth.SID))
	response, err := h2cClient().Do(request)
	if err != nil {
		t.Fatalf("请求失败：%v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("引导路由应当被应答，实际 %d", response.StatusCode)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(body)))
	if err != nil {
		t.Fatalf("响应体应当是 base64 文本：%v", err)
	}
	plain, err := session.DecodeResponse(sealed)
	if err != nil {
		t.Fatalf("客户端方向应该能解开响应：%v", err)
	}
	var decoded map[string]any
	if err := msgpack.Unmarshal(plain, &decoded); err != nil {
		t.Fatalf("响应应当是 MessagePack：%v", err)
	}
	headers, ok := decoded["data_headers"].(map[string]any)
	if !ok {
		t.Fatalf("响应缺少 data_headers：%v", decoded)
	}
	if code, ok := headers["result_code"]; !ok || toInt(code) != 1 {
		t.Fatalf("result_code 应当是 1，实际 %v", headers["result_code"])
	}
	data, ok := decoded["data"].(map[string]any)
	if !ok || len(data) == 0 {
		t.Fatalf("响应缺少 data：%v", decoded)
	}
	if _, ok := data["resource_version"]; !ok {
		t.Fatalf("/Version/info 的 data 应当有 resource_version：%v", data)
	}
}

// 没有会话（没解开报文）时，夹具处理器不能凭空造响应——它应当退回未实现，
// 让 broker 如实回 501。
func TestFixtureHandlerNeedsSession(t *testing.T) {
	fixtures, err := nativeproto.LoadFixtures()
	if err != nil {
		t.Fatal(err)
	}
	handler := NewFixtureHandler(fixtures, log.New(io.Discard, "", 0))
	if _, ok := handler.Route(&Request{Path: "/cygames/Version/info"}); ok {
		t.Fatal("没有会话状态时不该应答")
	}
	if _, ok := handler.Route(&Request{Path: "/cygames/Unknown/route", Envelope: &nativeproto.Request{}}); ok {
		t.Fatal("夹具里没有的路由不该应答")
	}
}

func TestNormalizeRouteStripsClientPrefix(t *testing.T) {
	cases := map[string]string{
		"/cygames/Version/info": "/Version/info",
		"/Version/info":         "/Version/info",
		"/api/Card/getList":     "/Card/getList",
		"/":                     "/",
	}
	for input, want := range cases {
		if got := nativeproto.NormalizeRoute(input); got != want {
			t.Errorf("NormalizeRoute(%q) = %q，期望 %q", input, got, want)
		}
	}
}

func toInt(value any) int64 {
	switch typed := value.(type) {
	case uint8:
		return int64(typed)
	case uint16:
		return int64(typed)
	case uint32:
		return int64(typed)
	case uint:
		return int64(typed)
	case int8:
		return int64(typed)
	case int16:
		return int64(typed)
	case int32:
		return int64(typed)
	case int64:
		return typed
	case uint64:
		return int64(typed)
	case int:
		return int64(typed)
	}
	return -1
}
