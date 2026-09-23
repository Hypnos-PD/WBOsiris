package nativeproto

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"testing"
)

// oracleHeaderSize 是客户端构造的那个扩展头的长度（WBArts 里叫 extendedHeaderSize）。
const oracleHeaderSize = 170

// 这两条向量来自 WBArts/server/ranking/codec_test.go：输入是客户端构造的
// 166 字节扩展头 + 载荷，输出是**原版 DLL 实际算出来的**报文（WBArts 把它当作原生
// oracle 用）。拿它钉住我们的客户端方向，等于间接验证了整条密钥派生链——本包自己
// 写的"客户端"和"服务端"共用同一份 KDF，两边同时写错时对拍永远对得上，这条向量不会。
const (
	nativeOraclePacket = "pgAAAF67G35m83dThV6u4dn57/7nQFkaMGobvANKlWmneKVimaEtAd8xJbMkj5MjvQCLz3c2O/YjSG2St9wBJktwlbrfBClOc5i94gcsUXabwOUKL1R5nsPoDTJXfKHG6xA1Wn+kye4TOF2Cp8zxFjtgharP9Bk+Y4it0vccQWaLsNX6H0RpjrPY/SJHbJG22wAlSm+Uud4DKE1yl7zhBitQdZq/5AkuU3iDpXByb2JlqWNvZGVjLTE3MKVjb3VudMyqpGRhdGHEIAABAgMEBQYHCAkKCwwNDg8QERITFBUWFxgZGhscHR4f"
	nativeOracleWire   = "lAAAACUhNopfyi07jg7AFpZEeAIM7AkO3jCoYG9ADWL083h3365UPpeZ9FA8UKc28ldBP20uMglim8tIfB1rtBApXp6jtpbRnJaSnEav5FybF6mXSOU9cdwpRHDs3f0fa8XtYPcbH76xbSI6sg2yLLZObp1ZHyEjeXXhgkP0L6RqfU/vcKs50Ob5hndx+7qmZk4yz4rKTj0="
)

// TestEncodeMatchesNativeOracle 要求我们的客户端方向逐字节复现原版 DLL 的输出。
//
// 覆盖到的每一步都是容易写错的：临时私钥的 PCG32 生成与标量规范化、对端公钥的
// MPI 翻转、shared 之后的混淆（mix）、元数据 AES-CTR、载荷 AES-GCM 的 nonce/AAD、
// 以及报文的四段布局。任何一步不一致，这里都会给出第一处不同的偏移。
func TestEncodeMatchesNativeOracle(t *testing.T) {
	packet := mustDecodeBase64(t, nativeOraclePacket)
	want := mustDecodeBase64(t, nativeOracleWire)
	if len(packet) < oracleHeaderSize {
		t.Fatalf("向量里的包只有 %d 字节", len(packet))
	}
	auth := Auth{
		SID:     packet[56:72],
		UUID:    packet[72:88],
		AuthKey: packet[120:170],
	}
	client, err := NewClient(auth, packet[4:56])
	if err != nil {
		t.Fatal(err)
	}
	var seed [16]byte
	copy(seed[:], packet[88:104])
	advance := byte(packet[56] & 0x0f)
	got, _, err := client.EncodeRequestWithSeed(packet[oracleHeaderSize:], seed, advance)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("与原版 DLL 的输出不一致：长度 %d vs %d，第一处不同在偏移 %d",
			len(got), len(want), firstDifference(got, want))
	}
}

// TestDecodeRequestAgainstReferenceEncoder 用**另一份实现**（照 WBArts 的
// codec.go 逐字重排的参考编码器）造请求，再要求服务端解得出来——这条不依赖本包
// 自己的客户端方向，能独立抓住 KDF 写错的情况（mixed[i] 写成 mixed[0] 就是被它
// 抓住的那一类）。
func TestDecodeRequestAgainstReferenceEncoder(t *testing.T) {
	auth := Auth{
		SID:     []byte("0123456789abcdef"),
		UUID:    []byte("fedcba9876543210"),
		AuthKey: mustDecodeHex(t, "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff0011223344556677"),
	}
	private, err := ecdhPrivateFromHex(t, "9ca2d461ae2aab01c2168a0438a1859d8c180e058daa2f40ecc608e0a2917ccf")
	if err != nil {
		t.Fatal(err)
	}
	// 常量：前 32 字节是"对端公钥"（这里是我们的公钥，按客户端的大端 MPI 约定存放）。
	commonHeader := append(reverseCopy(private.PublicKey().Bytes()),
		mustDecodeHex(t, "6854C854E96E762E4F18C9F7370608BE437ECBBA")...)
	payload := []byte{0x81, 0xa4, 'u', 'u', 'i', 'd', 0xa0} // {"uuid": ""}
	wire := referenceEncode(t, payload, auth, commonHeader)

	server, err := NewServer(private.Bytes(), commonHeader)
	if err != nil {
		t.Fatal(err)
	}
	request, err := server.DecodeRequest(wire, auth)
	if err != nil {
		t.Fatalf("服务端解不开参考编码器造的请求: %v", err)
	}
	if !bytes.Equal(request.Plain, payload) {
		t.Fatalf("解出来的载荷不对：%x", request.Plain)
	}
}

func mustDecodeBase64(t *testing.T, value string) []byte {
	t.Helper()
	raw, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// referenceEncode 是 WBArts/server/ranking/codec.go 里 Encode 的逐字重排：它不调用
// 本包的生产实现，只共用底层原语。混淆那一步写成 referenceMix（mixed[index]），
// 与生产实现分开维护——生产实现写错时这条测试才会失败。
func referenceEncode(t *testing.T, payload []byte, auth Auth, commonHeader []byte) []byte {
	t.Helper()
	var seed [16]byte
	for index := range seed {
		seed[index] = byte(index * 7)
	}
	advance := byte(3)
	rng := newPCG(binary.BigEndian.Uint64(seed[:8]), binary.BigEndian.Uint64(seed[8:]), int(advance))
	raw := make([]byte, 32)
	for index := range raw {
		raw[index] = rng.nextByte()
	}
	private, err := ecdh.X25519().NewPrivateKey(normalizeScalar(raw))
	if err != nil {
		t.Fatal(err)
	}
	peer, err := ecdh.X25519().NewPublicKey(reverseCopy(commonHeader[:32]))
	if err != nil {
		t.Fatal(err)
	}
	shared, err := private.ECDH(peer)
	if err != nil {
		t.Fatal(err)
	}
	public := private.PublicKey().Bytes()
	commonTail := commonHeader[32:52]
	fieldA, fieldB := auth.SID, auth.UUID
	selector := auth.AuthKey[34:50]
	preMix := md5Of(fieldA, selector, fieldB)
	mixed := referenceMix(shared, preMix, commonTail)
	gcmKey := md5Of(mixed, shared, preMix, commonTail)
	nonce := md5Of(public, fieldA, fieldB)

	metaPlain := make([]byte, metaSize)
	metaPlain[0] = 1
	copy(metaPlain[2:34], auth.AuthKey[:32])
	copy(metaPlain[34:36], auth.AuthKey[32:34])
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
	wire := make([]byte, 0, wireOverhead+len(payload))
	var length [4]byte
	binary.LittleEndian.PutUint32(length[:], uint32(wireOverhead+len(payload)-4))
	wire = append(wire, length[:]...)
	wire = append(wire, public...)
	wire = append(wire, metaCiphertext...)
	wire = append(wire, sealed[len(sealed)-tagSize:]...)
	wire = append(wire, sealed[:len(sealed)-tagSize]...)
	return wire
}

// referenceMix 与生产实现只差一个下标：v0 用的是 mixed[index]。
func referenceMix(shared, preMix, commonTail []byte) []byte {
	mixed := append([]byte(nil), shared[:16]...)
	for index, x := range preMix {
		v0 := x ^ mixed[index]
		i0 := int(v0 & 0x0f)
		v1 := x ^ shared[i0]
		i1 := int(v1 & 0x0f)
		v2 := x ^ mixed[i1]
		i2 := int(v2 & 0x0f)
		mixed[i1] ^= v0
		mixed[i2] ^= v1
		v3 := x ^ commonTail[i2]
		i3 := int(v3 & 0x0f)
		mixed[i3] ^= v2
		mixed[i0] ^= v3
	}
	return mixed
}

func reverseCopy(value []byte) []byte {
	out := append([]byte(nil), value...)
	reverseBytes(out)
	return out
}

func mustDecodeHex(t *testing.T, value string) []byte {
	t.Helper()
	raw, err := hex.DecodeString(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func ecdhPrivateFromHex(t *testing.T, value string) (*ecdh.PrivateKey, error) {
	t.Helper()
	return ecdh.X25519().NewPrivateKey(mustDecodeHex(t, value))
}

func firstDifference(got, want []byte) int {
	limit := len(got)
	if len(want) < limit {
		limit = len(want)
	}
	for index := 0; index < limit; index++ {
		if got[index] != want[index] {
			return index
		}
	}
	return limit
}
