package nativeproto

import (
	"bytes"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/hex"
	"testing"
)

// 原始常量（1.9.1.18238）。前 32 字节是对端公钥，其余 20 字节参与密钥派生与认证。
const originalCommonHeaderHex = "5EBB1B7E66F37753855EAEE1D9F9EFFEE740591A306A1BBC034A9569A778A562" +
	"6854C854E96E762E4F18C9F7370608BE437ECBBA"

// testSetup 造一套自洽的密钥与凭据：把客户端常量前 32 字节换成我们公钥的反序——
// 这正是改写客户端内存时写入的形式。
func testSetup(t *testing.T) (serverPrivate, patchedHeader, originalHeader []byte, auth Auth) {
	t.Helper()
	key, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	serverPrivate = key.Bytes()
	stored := append([]byte(nil), key.PublicKey().Bytes()...)
	reverseBytes(stored)
	originalHeader, err = hex.DecodeString(originalCommonHeaderHex)
	if err != nil {
		t.Fatal(err)
	}
	patchedHeader = append([]byte(nil), originalHeader...)
	copy(patchedHeader[:32], stored)
	auth = Auth{
		SID:     bytes.Repeat([]byte{0x11}, 16),
		UUID:    bytes.Repeat([]byte{0x22}, 16),
		AuthKey: bytes.Repeat([]byte{0x33}, 50),
	}
	return serverPrivate, patchedHeader, originalHeader, auth
}

func TestRequestResponseRoundTrip(t *testing.T) {
	privateKey, header, _, auth := testSetup(t)
	payload := []byte{0x82, 0xA1, 'a', 0x01, 0xA1, 'b', 0x02} // 一小段 MessagePack

	client, err := NewClient(auth, header)
	if err != nil {
		t.Fatal(err)
	}
	wire, session, err := client.EncodeRequest(payload)
	if err != nil {
		t.Fatal(err)
	}
	server, err := NewServer(privateKey, header)
	if err != nil {
		t.Fatal(err)
	}
	request, err := server.DecodeRequest(wire, auth)
	if err != nil {
		t.Fatalf("服务端应该能解开配对公钥的报文: %v", err)
	}
	if !bytes.Equal(request.Plain, payload) {
		t.Fatalf("明文不符：%x != %x", request.Plain, payload)
	}
	if head, value := request.CheckAuth(auth); !head || !value {
		t.Fatalf("元数据里的凭据片段应与已知凭据一致：head=%v value=%v", head, value)
	}

	// 服务端封响应，客户端方向解回来。
	responsePlain := []byte{0x81, 0xA4, 'o', 'k', 0xC3}
	responseWire, err := request.EncodeResponse(responsePlain)
	if err != nil {
		t.Fatal(err)
	}
	got, err := session.DecodeResponse(responseWire)
	if err != nil {
		t.Fatalf("客户端方向应该能解开服务端的响应: %v", err)
	}
	if !bytes.Equal(got, responsePlain) {
		t.Fatalf("响应对拍失败：%x != %x", got, responsePlain)
	}
}

func TestWrongKeyFailsClosed(t *testing.T) {
	_, header, _, auth := testSetup(t)
	client, err := NewClient(auth, header)
	if err != nil {
		t.Fatal(err)
	}
	wire, _, err := client.EncodeRequest([]byte{0x80})
	if err != nil {
		t.Fatal(err)
	}
	other, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	server, err := NewServer(other.Bytes(), header)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.DecodeRequest(wire, auth); err == nil {
		t.Fatal("用错私钥必须失败（GCM 认证拦下），不能给出垃圾数据")
	}
}

// 客户端仍然用**原始**常量（没被换过）时，我们解不开——这是"必须换公钥"的前提，
// 也是判断"这次抓到的包是不是用我们的公钥加的密"的依据。
func TestOriginalKeyCannotBeDecoded(t *testing.T) {
	privateKey, _, originalHeader, auth := testSetup(t)
	client, err := NewClient(auth, originalHeader)
	if err != nil {
		t.Fatal(err)
	}
	wire, _, err := client.EncodeRequest([]byte{0x80})
	if err != nil {
		t.Fatal(err)
	}
	server, err := NewServer(privateKey, originalHeader)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.DecodeRequest(wire, auth); err == nil {
		t.Fatal("用原公钥协商的报文不该被我们的私钥解开")
	}
}

func TestRejectsBadInput(t *testing.T) {
	privateKey, header, _, auth := testSetup(t)
	server, err := NewServer(privateKey, header)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.DecodeRequest([]byte{1, 2, 3}, auth); err == nil {
		t.Fatal("过短的报文应当被拒绝")
	}
	if _, err := server.DecodeRequest(make([]byte, 256), Auth{SID: auth.SID, UUID: auth.UUID}); err == nil {
		t.Fatal("凭据长度不对应当被拒绝")
	}
	if _, err := NewServer(make([]byte, 31), header); err == nil {
		t.Fatal("私钥长度不对应当被拒绝")
	}
	if _, err := NewServer(privateKey, make([]byte, 51)); err == nil {
		t.Fatal("常量长度不对应当被拒绝")
	}
	// 长度字段非 0 表示响应是压缩的；本实现不支持，必须明确拒绝而不是当成未压缩。
	fake := make([]byte, responseHeader+16)
	fake[0] = 1
	if _, err := (&Session{shared: make([]byte, 32), sid: auth.SID, selector: auth.AuthKey[34:50]}).DecodeResponse(fake); err == nil {
		t.Fatal("压缩响应应当被明确拒绝")
	}
}
