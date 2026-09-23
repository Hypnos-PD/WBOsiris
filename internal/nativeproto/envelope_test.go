package nativeproto

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/binary"
	"math/big"
	"math/bits"
	"testing"
)

// 这里的 client* 函数是**客户端方向**的独立实现，照着 WBArts 的记录重写一遍：
// 只有两边都能对上，才能说明本包的服务端方向没错。真实客户端发来的报文是最终判据
// （见 delta/tools/decode_native_request.py 与抓包验证）。

type pcg32 struct {
	state uint64
	inc   uint64
}

func newPCG(stateSeed, streamSeed uint64, advance int) *pcg32 {
	p := &pcg32{inc: streamSeed*2 + 1}
	p.state = stateSeed + p.inc
	p.state = p.state*0x5851f42d4c957f2d + p.inc
	for range advance {
		p.state = p.state*0x5851f42d4c957f2d + p.inc
	}
	return p
}

func (p *pcg32) nextByte() byte {
	old := p.state
	p.state = old*0x5851f42d4c957f2d + p.inc
	xorshifted := uint32(((old >> 18) ^ old) >> 27)
	return byte(bits.RotateLeft32(xorshifted, -int(old>>59)))
}

// 客户端标量规范化：把 32 字节压到 255 位以内并按 X25519 的要求置位。
func normalizeScalar(raw []byte) []byte {
	value := new(big.Int).SetBytes(raw)
	highest := value.BitLen() - 1
	if highest > 254 {
		value.Rsh(value, uint(highest-254))
	} else {
		value.SetBit(value, 254, 1)
	}
	value.SetBit(value, 0, 0)
	value.SetBit(value, 1, 0)
	value.SetBit(value, 2, 0)
	bigEndian := make([]byte, 32)
	value.FillBytes(bigEndian)
	little := make([]byte, 32)
	for i := range little {
		little[i] = bigEndian[31-i]
	}
	return little
}

type clientAuth struct {
	sid     []byte
	uuid    []byte
	authKey []byte
}

// clientEncode 生成一个请求信封：[len][客户端公钥][AES-CTR 元数据][tag][AES-GCM 载荷]
func clientEncode(t *testing.T, commonHeader []byte, auth clientAuth, payload []byte, seed [16]byte, advance byte) []byte {
	t.Helper()
	rng := newPCG(binary.BigEndian.Uint64(seed[:8]), binary.BigEndian.Uint64(seed[8:]), int(advance&0x0f))
	raw := make([]byte, 32)
	for i := range raw {
		raw[i] = rng.nextByte()
	}
	privateKey, err := ecdh.X25519().NewPrivateKey(normalizeScalar(raw))
	if err != nil {
		t.Fatal(err)
	}
	peerBytes := append([]byte(nil), commonHeader[4-4:32]...) // 常量前 32 字节
	reverse(peerBytes)
	peer, err := ecdh.X25519().NewPublicKey(peerBytes)
	if err != nil {
		t.Fatal(err)
	}
	shared, err := privateKey.ECDH(peer)
	if err != nil {
		t.Fatal(err)
	}
	public := privateKey.PublicKey().Bytes()

	commonTail := commonHeader[32:52]
	fieldA, fieldB := auth.sid, auth.uuid
	selector := auth.authKey[34:50]
	preMix := md5Of(fieldA, selector, fieldB)
	gcmKey := md5Of(mix(shared, preMix, commonTail), shared, preMix, commonTail)
	nonce := md5Of(public, fieldA, fieldB)

	metaPlain := make([]byte, 36)
	metaPlain[0] = 1
	copy(metaPlain[2:34], auth.authKey[:32])
	copy(metaPlain[34:], auth.authKey[32:34])
	block, err := aes.NewCipher(shared)
	if err != nil {
		t.Fatal(err)
	}
	metaCiphertext := make([]byte, len(metaPlain))
	cipher.NewCTR(block, public[:aes.BlockSize]).XORKeyStream(metaCiphertext, metaPlain)

	payloadBlock, err := aes.NewCipher(gcmKey)
	if err != nil {
		t.Fatal(err)
	}
	gcm, err := cipher.NewGCMWithNonceSize(payloadBlock, len(nonce))
	if err != nil {
		t.Fatal(err)
	}
	sealed := gcm.Seal(nil, nonce, payload, concat(commonTail, fieldA, fieldB))

	out := make([]byte, 0, wireOverhead+len(payload))
	var length [4]byte
	binary.LittleEndian.PutUint32(length[:], uint32(wireOverhead+len(payload)-4))
	out = append(out, length[:]...)
	out = append(out, public...)
	out = append(out, metaCiphertext...)
	out = append(out, sealed[len(sealed)-tagSize:]...)
	out = append(out, sealed[:len(sealed)-tagSize]...)
	return out
}

// clientDecodeResponse 是客户端方向解响应，用于验证服务端封出来的包能被读懂。
func clientDecodeResponse(t *testing.T, shared, sid, selector []byte, wire []byte) []byte {
	t.Helper()
	if len(wire) < responseHeader {
		t.Fatal("响应太短")
	}
	key := md5Of(shared, sid, selector)
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	gcm, err := cipher.NewGCMWithNonceSize(block, responseNonceSize)
	if err != nil {
		t.Fatal(err)
	}
	sealed := append(append([]byte(nil), wire[responseHeader:]...), wire[4+responseNonceSize:responseHeader]...)
	plain, err := gcm.Open(nil, wire[4:4+responseNonceSize], sealed, concat(sid, selector))
	if err != nil {
		t.Fatalf("客户端解不开服务端的响应: %v", err)
	}
	if declared := binary.LittleEndian.Uint32(wire[:4]); declared != 0 {
		t.Fatalf("本包不压缩响应，长度字段应为 0，实际 %d", declared)
	}
	return plain
}

func reverse(value []byte) {
	for i, j := 0, len(value)-1; i < j; i, j = i+1, j-1 {
		value[i], value[j] = value[j], value[i]
	}
}

func testKeys(t *testing.T) (serverPrivate []byte, commonHeader []byte, auth clientAuth) {
	t.Helper()
	key, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	serverPrivate = key.Bytes()
	// 常量前 32 字节放我们公钥的**反序**——这正是改写客户端内存时写入的形式。
	stored := append([]byte(nil), key.PublicKey().Bytes()...)
	reverse(stored)
	commonHeader = make([]byte, 52)
	copy(commonHeader[:32], stored)
	copy(commonHeader[32:], []byte{
		0x68, 0x54, 0xC8, 0x54, 0xE9, 0x6E, 0x76, 0x2E, 0x4F, 0x18,
		0xC9, 0xF7, 0x37, 0x06, 0x08, 0xBE, 0x43, 0x7E, 0xCB, 0xBA,
	})
	auth = clientAuth{
		sid:     bytes.Repeat([]byte{0x11}, 16),
		uuid:    bytes.Repeat([]byte{0x22}, 16),
		authKey: append(bytes.Repeat([]byte{0x33}, 50), 0),
	}
	auth.authKey = auth.authKey[:50]
	return serverPrivate, commonHeader, auth
}

func TestRequestRoundTrip(t *testing.T) {
	privateKey, commonHeader, auth := testKeys(t)
	payload := []byte{0x82, 0xA1, 'a', 0x01, 0xA1, 'b', 0x02} // 一小段 MessagePack

	wire := clientEncode(t, commonHeader, auth, payload, [16]byte{1, 2, 3}, 5)
	server, err := NewServer(privateKey, commonHeader)
	if err != nil {
		t.Fatal(err)
	}
	request, err := server.DecodeRequest(wire, Auth{SID: auth.sid, UUID: auth.uuid, AuthKey: auth.authKey})
	if err != nil {
		t.Fatalf("服务端应该能解开自己配对公钥的报文: %v", err)
	}
	if !bytes.Equal(request.Plain, payload) {
		t.Fatalf("明文不符\n实际 %x\n期望 %x", request.Plain, payload)
	}
	if head, value := request.CheckAuth(Auth{SID: auth.sid, UUID: auth.uuid, AuthKey: auth.authKey}); !head || !value {
		t.Fatalf("元数据里的凭据片段应与已知凭据一致：head=%v value=%v", head, value)
	}

	// 再用同一次会话封一个响应，并让客户端方向解回来。
	responsePlain := []byte{0x81, 0xA4, 'o', 'k', 0xC3}
	responseWire, err := request.EncodeResponse(responsePlain)
	if err != nil {
		t.Fatal(err)
	}
	got := clientDecodeResponse(t, request.shared, request.sid, request.selector, responseWire)
	if !bytes.Equal(got, responsePlain) {
		t.Fatalf("响应对拍失败：%x != %x", got, responsePlain)
	}
}

func TestWrongKeyFailsClosed(t *testing.T) {
	_, commonHeader, auth := testKeys(t)
	payload := []byte{0x80}
	wire := clientEncode(t, commonHeader, auth, payload, [16]byte{9}, 1)

	// 换一把不同的私钥：必须解密失败，而不是给出垃圾数据。
	other, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	server, err := NewServer(other.Bytes(), commonHeader)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.DecodeRequest(wire, Auth{SID: auth.sid, UUID: auth.uuid, AuthKey: auth.authKey}); err == nil {
		t.Fatal("用错私钥时必须失败（GCM 认证）")
	}
}

func TestRejectsBadInput(t *testing.T) {
	privateKey, commonHeader, auth := testKeys(t)
	server, err := NewServer(privateKey, commonHeader)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.DecodeRequest([]byte{1, 2, 3}, Auth{SID: auth.sid, UUID: auth.uuid, AuthKey: auth.authKey}); err == nil {
		t.Fatal("过短的报文应当被拒绝")
	}
	if _, err := server.DecodeRequest(make([]byte, 200), Auth{SID: auth.sid, UUID: auth.uuid}); err == nil {
		t.Fatal("凭据长度不对应当被拒绝")
	}
	if _, err := NewServer(make([]byte, 31), commonHeader); err == nil {
		t.Fatal("私钥长度不对应当被拒绝")
	}
	if _, err := NewServer(privateKey, make([]byte, 51)); err == nil {
		t.Fatal("常量长度不对应当被拒绝")
	}
}
