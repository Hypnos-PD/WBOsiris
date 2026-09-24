package nativeproto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"math/big"
	"math/bits"
)

// 客户端方向的信封实现。
//
// 服务端（本包主体）是 WBO 真正要用的那一半；这一半存在的理由是**可验证性**：
//
//   - 测试里需要造出"客户端会发出的报文"，才能验证服务端解得对、封得能被读懂；
//   - 报文格式的权威实现是 WBArts 的 server/ranking/codec.go（对着真服务器跑通过），
//     这里照着它重写一遍，两边对拍才有意义。
//
// 它不参与运行时服务，也不代表 WBO 去连官方服务器。

// Client 用客户端凭据与常量构造一个报文生产/消费端。
type Client struct {
	commonHeader []byte
	auth         Auth
}

// Session 是一次会话中双方共享的状态（响应密钥由它派生）。
type Session struct {
	shared   []byte
	sid      []byte
	selector []byte
}

// NewClient 构造客户端方向。commonHeader 必须与客户端实际使用的那份一致：
// 前 32 字节是"对端公钥"，服务端要解开就必须持有其私钥。
func NewClient(auth Auth, commonHeader []byte) (*Client, error) {
	if len(auth.SID) != 16 || len(auth.UUID) != 16 || len(auth.AuthKey) < 50 {
		return nil, fmt.Errorf("nativeproto: 客户端凭据长度不对")
	}
	if len(commonHeader) != 52 {
		return nil, fmt.Errorf("nativeproto: 常量必须是 52 字节")
	}
	return &Client{commonHeader: append([]byte(nil), commonHeader...), auth: auth}, nil
}

// Shared 供测试或诊断使用：返回本次会话的共享密钥。
func (s *Session) Shared() []byte { return s.shared }

// EncodeRequest 生成一个请求信封。
func (c *Client) EncodeRequest(payload []byte) ([]byte, *Session, error) {
	// 客户端每次请求都换一把临时私钥，由报文头里的随机数派生（PCG32 + 标量规范化）。
	seed := make([]byte, 16)
	if _, err := rand.Read(seed); err != nil {
		return nil, nil, err
	}
	var fixed [16]byte
	copy(fixed[:], seed)
	return c.EncodeRequestWithSeed(payload, fixed, byte(fixed[0]&0x0f))
}

// EncodeRequestWithSeed 用给定随机数生成请求——同一个种子得到同一把临时私钥，
// 便于复现问题。
func (c *Client) EncodeRequestWithSeed(payload []byte, seed [16]byte, advance byte) ([]byte, *Session, error) {
	rng := newPCG(binary.BigEndian.Uint64(seed[:8]), binary.BigEndian.Uint64(seed[8:]), int(advance&0x0f))
	raw := make([]byte, 32)
	for i := range raw {
		raw[i] = rng.nextByte()
	}
	privateKey, err := ecdh.X25519().NewPrivateKey(normalizeScalar(raw))
	if err != nil {
		return nil, nil, err
	}
	// 常量前 32 字节按大端 MPI 存放，取出来要翻转才是 X25519 的公钥字节。
	peerBytes := append([]byte(nil), c.commonHeader[:32]...)
	reverseBytes(peerBytes)
	peer, err := ecdh.X25519().NewPublicKey(peerBytes)
	if err != nil {
		return nil, nil, fmt.Errorf("nativeproto: 常量里的对端公钥无效: %w", err)
	}
	shared, err := privateKey.ECDH(peer)
	if err != nil {
		return nil, nil, err
	}
	public := privateKey.PublicKey().Bytes()

	commonTail := c.commonHeader[32:52]
	fieldA, fieldB := c.auth.SID, c.auth.UUID
	selector := c.auth.AuthKey[34:50]
	preMix := md5Of(fieldA, selector, fieldB)
	gcmKey := md5Of(mix(shared, preMix, commonTail), shared, preMix, commonTail)
	nonce := md5Of(public, fieldA, fieldB)

	metaPlain := make([]byte, metaSize)
	metaPlain[0] = 1
	copy(metaPlain[2:34], c.auth.AuthKey[:32])
	copy(metaPlain[34:36], c.auth.AuthKey[32:34])
	block, err := aes.NewCipher(shared)
	if err != nil {
		return nil, nil, err
	}
	metaCiphertext := make([]byte, len(metaPlain))
	cipher.NewCTR(block, public[:aes.BlockSize]).XORKeyStream(metaCiphertext, metaPlain)

	payloadBlock, err := aes.NewCipher(gcmKey)
	if err != nil {
		return nil, nil, err
	}
	gcm, err := cipher.NewGCMWithNonceSize(payloadBlock, len(nonce))
	if err != nil {
		return nil, nil, err
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
	return wire, &Session{shared: shared, sid: fieldA, selector: selector}, nil
}

// DecodeResponse 用会话状态解开一个响应。
func (s *Session) DecodeResponse(wire []byte) ([]byte, error) {
	if len(wire) < responseHeader {
		return nil, fmt.Errorf("nativeproto: 响应只有 %d 字节", len(wire))
	}
	key := md5Of(s.shared, s.sid, s.selector)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCMWithNonceSize(block, responseNonceSize)
	if err != nil {
		return nil, err
	}
	// 布局是 长度 + nonce + tag + 密文，GCM 要的是密文‖tag。
	body := wire[responseHeader:]
	tag := wire[4+responseNonceSize : responseHeader]
	sealed := concat(body, tag)
	plain, err := gcm.Open(nil, wire[4:4+responseNonceSize], sealed, concat(s.sid, s.selector))
	if err != nil {
		return nil, fmt.Errorf("nativeproto: 响应认证失败: %w", err)
	}
	if declared := binary.LittleEndian.Uint32(wire[:4]); declared != 0 {
		// 客户端只在长度非 0 时才去 LZ4 解压；本实现不压缩响应。
		return nil, fmt.Errorf("nativeproto: 此实现不处理压缩响应（长度字段 %d）", declared)
	}
	return plain, nil
}

func reverseBytes(value []byte) {
	for i, j := 0, len(value)-1; i < j; i, j = i+1, j-1 {
		value[i], value[j] = value[j], value[i]
	}
}

// pcg32 与 normalizeScalar 复现客户端生成临时私钥的方式。
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
