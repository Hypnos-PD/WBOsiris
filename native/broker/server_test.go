package nativebroker

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"wbo/native/proto"

	"golang.org/x/net/http2"
)

// h2cClient 说明文 HTTP/2 先验知识，和客户端被改写之后的行为一致。
func h2cClient() *http.Client {
	return &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http2.Transport{
			AllowHTTP: true,
			DialTLSContext: func(ctx context.Context, network, address string, _ *tls.Config) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, network, address)
			},
		},
	}
}

type stubHandler struct {
	seen []string
}

func (s *stubHandler) Route(request *Request) (Response, bool) {
	s.seen = append(s.seen, request.Path)
	if request.Path == "/Version/info" {
		return Response{Body: []byte{0x81, 0xa1, 'x', 0x01}}, true
	}
	return Response{}, false
}

func startServer(t *testing.T, handler Handler, captureDir string) (string, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	ready := make(chan string, 1)
	go func() {
		if err := Serve(ctx, Options{
			Listen:     "127.0.0.1:0",
			CaptureDir: captureDir,
			Handler:    handler,
			Log:        log.New(io.Discard, "", 0),
			Ready:      func(address string) { ready <- address },
		}); err != nil {
			t.Logf("Serve 返回：%v", err)
		}
	}()
	select {
	case address := <-ready:
		return address, cancel
	case <-time.After(5 * time.Second):
		cancel()
		t.Fatal("broker 没有在 5 秒内监听")
		return "", cancel
	}
}

func TestBrokerSpeaksPlainHTTP2(t *testing.T) {
	handler := &stubHandler{}
	address, cancel := startServer(t, handler, "")
	defer cancel()

	response, err := h2cClient().Post("http://"+address+"/Version/info", "application/x-msgpack", nil)
	if err != nil {
		t.Fatalf("h2c 请求失败：%v", err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("已实现路由应该返回 200，实际 %d", response.StatusCode)
	}
	if len(body) == 0 {
		t.Fatal("响应体不应该为空")
	}
	if response.ProtoMajor != 2 {
		t.Fatalf("必须是 HTTP/2，实际 %s", response.Proto)
	}
	if len(handler.seen) != 1 || handler.seen[0] != "/Version/info" {
		t.Fatalf("路由没有收到请求：%v", handler.seen)
	}
}

func TestUnimplementedRouteIsHonest(t *testing.T) {
	address, cancel := startServer(t, nil, "")
	defer cancel()
	response, err := h2cClient().Post("http://"+address+"/Deck/getList", "application/x-msgpack", nil)
	if err != nil {
		t.Fatalf("请求失败：%v", err)
	}
	defer response.Body.Close()
	// 未实现必须是明确失败，不能伪造成功——第一次跑就是要看清客户端问什么。
	if response.StatusCode != http.StatusNotImplemented {
		t.Fatalf("未实现路由应该返回 501，实际 %d", response.StatusCode)
	}
}

func TestCaptureRecordsRequestAndBody(t *testing.T) {
	directory := t.TempDir()
	address, cancel := startServer(t, &stubHandler{}, directory)
	defer cancel()

	payload := []byte{0x82, 0xa1, 'a', 0x01, 0xa1, 'b', 0xc3}
	response, err := h2cClient().Post("http://"+address+"/RoomMatch/createRoom?x=1", "application/x-msgpack", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("请求失败：%v", err)
	}
	response.Body.Close()

	raw, err := os.ReadFile(filepath.Join(directory, "requests.jsonl"))
	if err != nil {
		t.Fatalf("没有录制文件：%v", err)
	}
	var record Record
	if err := json.Unmarshal(raw, &record); err != nil {
		t.Fatalf("录制条目不是合法 JSON：%v", err)
	}
	if record.Path != "/RoomMatch/createRoom" || record.Query != "x=1" {
		t.Fatalf("路径或查询串不对：%+v", record)
	}
	if record.Protocol != "HTTP/2.0" {
		t.Fatalf("应该记下协议版本，实际 %q", record.Protocol)
	}
	if record.Status != http.StatusNotImplemented || record.Handled {
		t.Fatalf("未实现的路由应该记为未处理：%+v", record)
	}
	body, err := os.ReadFile(filepath.Join(directory, filepath.FromSlash(record.Body)))
	if err != nil {
		t.Fatalf("没有录到请求体：%v", err)
	}
	if string(body) != string(payload) {
		t.Fatalf("请求体被改动：%x", body)
	}
}

// 一个假的解码器：只验证 broker 有没有把请求体递进解码、把明文与信封带出来。
// 真正的加解密在 native/proto 里单独测。
type stubDecoder struct {
	gotBody []byte
	gotAuth nativeproto.Auth
	plain   []byte
	err     error
}

// DecodeEnvelope 在测试桩里与 DecodeRequest 同义：桩的失败就是"整个解不开"。
func (s *stubDecoder) DecodeEnvelope(wire []byte, auth nativeproto.Auth) (*nativeproto.Request, error) {
	return s.DecodeRequest(wire, auth)
}

func (s *stubDecoder) DecodeRequest(wire []byte, auth nativeproto.Auth) (*nativeproto.Request, error) {
	s.gotBody = wire
	s.gotAuth = auth
	if s.err != nil {
		return nil, s.err
	}
	return &nativeproto.Request{Plain: s.plain, AuthHead: []byte("head"), AuthValue: []byte("va")}, nil
}

func TestBrokerDecodesEnvelopeWhenConfigured(t *testing.T) {
	decoder := &stubDecoder{plain: []byte{0x81, 0xA1, 'k', 0x01}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ready := make(chan string, 1)
	go func() {
		_ = Serve(ctx, Options{
			Listen:  "127.0.0.1:0",
			Decoder: decoder,
			Auth:    nativeproto.Auth{UUID: make([]byte, 16), AuthKey: make([]byte, 50)},
			Log:     log.New(io.Discard, "", 0),
			Ready:   func(address string) { ready <- address },
		})
	}()
	address := <-ready

	// 客户端发的是 base64 文本形式的报文，Sid 头是 16 字节十六进制。
	// 随便一段像信封的字节（首字节不是 MessagePack 的 map，避免被当成明文）。
	wire := []byte{0x2a, 0x00, 0x00, 0x00, 0x11, 0x22, 0x33, 0x44}
	body := base64.StdEncoding.EncodeToString(wire)
	request, err := http.NewRequest(http.MethodPost, "http://"+address+"/cygames/Version/info", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Sid", "00112233445566778899aabbccddeeff")
	response, err := h2cClient().Do(request)
	if err != nil {
		t.Fatalf("请求失败：%v", err)
	}
	response.Body.Close()

	if !bytes.Equal(decoder.gotBody, wire) {
		t.Fatalf("解码器拿到的应是解 base64 后的报文：%x", decoder.gotBody)
	}
	if len(decoder.gotAuth.SID) != 16 || decoder.gotAuth.SID[0] != 0x00 {
		t.Fatalf("Sid 头没有正确转成 16 字节：%x", decoder.gotAuth.SID)
	}
}

func TestBrokerSurvivesUndecodableBody(t *testing.T) {
	// 解不开（比如客户端还没被换成我们的公钥）时必须照常录制与回应，不能中断。
	decoder := &stubDecoder{err: errors.New("载荷认证失败")}
	directory := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ready := make(chan string, 1)
	go func() {
		_ = Serve(ctx, Options{
			Listen:     "127.0.0.1:0",
			CaptureDir: directory,
			Decoder:    decoder,
			Auth:       nativeproto.Auth{UUID: make([]byte, 16), AuthKey: make([]byte, 50)},
			Log:        log.New(io.Discard, "", 0),
			Ready:      func(address string) { ready <- address },
		})
	}()
	address := <-ready

	request, err := http.NewRequest(http.MethodPost, "http://"+address+"/cygames/Version/info", strings.NewReader("AAAA"))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Sid", "00112233445566778899aabbccddeeff")
	response, err := h2cClient().Do(request)
	if err != nil {
		t.Fatalf("请求失败：%v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNotImplemented {
		t.Fatalf("解不开不影响路由判定，仍应是 501，实际 %d", response.StatusCode)
	}
	if _, err := os.Stat(filepath.Join(directory, "requests.jsonl")); err != nil {
		t.Fatalf("解不开也必须照常录制：%v", err)
	}
}
