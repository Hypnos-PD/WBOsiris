// Package nativeproto 实现原版客户端请求/响应外面的那层加密信封。
//
// 客户端与服务端之间的报文不是裸的 MessagePack，而是套了一层信封：
//
//	base64( [u32 小端 载荷长度][32B 客户端临时公钥][36B AES-CTR 元数据][16B GCM tag][AES-GCM 载荷] )
//
// 会话密钥来自 X25519：客户端拿自己的临时私钥，跟一个**写在客户端里的静态公钥**
// 协商。所以有两个方向：
//
//   - 客户端方向：WBArts/server/ranking/codec.go 里有完整实现（对着真服务器跑通过）；
//   - 服务端方向：本包。需要有自己的私钥，且必须让客户端用**我们的**公钥去协商——
//     做法是把客户端里那个常量换成我们的公钥（见 delta/tools/patch_common_header.py）。
//
// 报文里有些字段服务端不直接看得到，但参与密钥派生与认证，必须由调用方提供：
//
//	sid       请求头 Sid（十六进制 16 字节）
//	uuid      客户端凭据的一部分
//	authKey   客户端凭据；其中第 34..50 字节是密钥派生用的 selector
//
// 这些可以从客户端存档里提取（WBArts/tools/svwb-credentials）。
package nativeproto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/md5"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
)

const (
	// 头部结构：4 字节长度 + 32 字节公钥 + 36 字节元数据 + 16 字节 tag。
	publicKeySize = 32
	metaSize      = 36
	tagSize       = 16
	wireOverhead  = 4 + publicKeySize + metaSize + tagSize

	// 响应信封：4 字节声明长度 + 16 字节 nonce + 16 字节 tag。
	responseNonceSize = 16
	responseHeader    = 4 + responseNonceSize + tagSize
)

// Auth 是服务端为了解开报文必须知道的客户端凭据。
type Auth struct {
	SID     []byte // 16 字节
	UUID    []byte // 16 字节
	AuthKey []byte // 50 字节
}

// Server 持有本会话的私钥与客户端常量。
type Server struct {
	private *ecdh.PrivateKey
	// commonTail 是那个 52 字节常量的第 32..52 字节：既参与密钥派生，也是 AAD 的一部分。
	commonTail []byte
}

// NewServer 用我们的 X25519 私钥和客户端的 52 字节常量构造服务端。
func NewServer(privateKey, commonHeader []byte) (*Server, error) {
	if len(privateKey) != 32 {
		return nil, fmt.Errorf("nativeproto: 私钥必须是 32 字节，实际 %d", len(privateKey))
	}
	if len(commonHeader) != 52 {
		return nil, fmt.Errorf("nativeproto: 常量必须是 52 字节，实际 %d", len(commonHeader))
	}
	key, err := ecdh.X25519().NewPrivateKey(privateKey)
	if err != nil {
		return nil, fmt.Errorf("nativeproto: 私钥无效: %w", err)
	}
	return &Server{private: key, commonTail: append([]byte(nil), commonHeader[32:52]...)}, nil
}

// Request 是一次解开后的请求，同时保留封响应所需的状态。
type Request struct {
	// Plain 是明文载荷，即 MessagePack 编码的请求体。
	Plain []byte
	// ClientPublic 是客户端本次的临时公钥（服务端要不要回它由上层决定）。
	ClientPublic []byte
	// AuthHead / AuthValue 是元数据里携带的凭据片段，可用于识别客户端。
	AuthHead  []byte
	AuthValue []byte

	shared   []byte
	sid      []byte
	selector []byte
}

// DecodeRequest 解开一个请求信封。
func (s *Server) DecodeRequest(wire []byte, auth Auth) (*Request, error) {
	if len(wire) < wireOverhead {
		return nil, fmt.Errorf("nativeproto: 报文只有 %d 字节，短于信封开销", len(wire))
	}
	if len(auth.SID) != 16 || len(auth.UUID) != 16 || len(auth.AuthKey) < 50 {
		return nil, errors.New("nativeproto: 凭据长度不对（sid/uuid 各 16 字节，authKey 至少 50）")
	}
	clientPublic := wire[4:36]
	metaCiphertext := wire[36:72]
	tag := wire[72:88]
	payloadCiphertext := wire[88:]

	peer, err := ecdh.X25519().NewPublicKey(clientPublic)
	if err != nil {
		return nil, fmt.Errorf("nativeproto: 客户端公钥无效: %w", err)
	}
	shared, err := s.private.ECDH(peer)
	if err != nil {
		return nil, fmt.Errorf("nativeproto: 协商失败: %w", err)
	}

	block, err := aes.NewCipher(shared)
	if err != nil {
		return nil, err
	}
	metaPlain := make([]byte, metaSize)
	cipher.NewCTR(block, clientPublic[:aes.BlockSize]).XORKeyStream(metaPlain, metaCiphertext)

	selector := auth.AuthKey[34:50]
	preMix := md5Of(auth.SID, selector, auth.UUID)
	gcmKey := md5Of(mix(shared, preMix, s.commonTail), shared, preMix, s.commonTail)
	nonce := md5Of(clientPublic, auth.SID, auth.UUID)
	aad := concat(s.commonTail, auth.SID, auth.UUID)

	payloadBlock, err := aes.NewCipher(gcmKey)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCMWithNonceSize(payloadBlock, len(nonce))
	if err != nil {
		return nil, err
	}
	sealed := append(append([]byte(nil), payloadCiphertext...), tag...)
	plain, err := gcm.Open(nil, nonce, sealed, aad)
	if err != nil {
		// 认证失败分两种，而它们要做的事完全相反，所以这里必须分清楚：
		//
		//   元数据那一步只用到 shared（X25519 的结果），不需要任何凭据。所以
		//   "元数据解出来的凭据片段与 authKey 一致"就等价于"共享密钥是对的"。
		//   那时候还失败，问题就在密钥派生（混淆/nonce/AAD）或 sid/uuid 上；
		//   不一致才是"客户端用的对端公钥不是我们那把"。
		//
		// 不做这个区分的话，两种情况的报错一模一样，排查会往完全错误的方向走——
		// mixed[0]/mixed[index] 那一处错就是这么被冤枉成"公钥没换成我们的"。
		if string(metaPlain[2:34]) != string(auth.AuthKey[:32]) {
			return nil, fmt.Errorf("nativeproto: 载荷认证失败，且元数据里的凭据对不上：客户端用的对端公钥很可能不是我们那把（共享密钥不对）: %w", err)
		}
		return nil, fmt.Errorf("nativeproto: 载荷认证失败，但共享密钥是对的（元数据里的凭据与 authKey 一致），问题在密钥派生或 sid/uuid: %w", err)
	}
	return &Request{
		Plain:        plain,
		ClientPublic: append([]byte(nil), clientPublic...),
		AuthHead:     append([]byte(nil), metaPlain[2:34]...),
		AuthValue:    append([]byte(nil), metaPlain[34:36]...),
		shared:       shared,
		sid:          append([]byte(nil), auth.SID...),
		selector:     append([]byte(nil), selector...),
	}, nil
}

// EncodeResponse 用同一次会话的密钥封一个响应。
//
// 与请求不同，响应的密钥只由 shared / sid / selector 派生，且长度字段留 0 表示
// 载荷未压缩——客户端只有在长度非 0 时才会去 LZ4 解压。
func (r *Request) EncodeResponse(plain []byte) ([]byte, error) {
	key := md5Of(r.shared, r.sid, r.selector)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCMWithNonceSize(block, responseNonceSize)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, responseNonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	sealed := gcm.Seal(nil, nonce, plain, concat(r.sid, r.selector))
	// 布局：长度 + nonce + tag + 密文（客户端按这个顺序读）。
	out := make([]byte, 0, responseHeader+len(plain))
	var declared [4]byte
	binary.LittleEndian.PutUint32(declared[:], 0)
	out = append(out, declared[:]...)
	out = append(out, nonce...)
	out = append(out, sealed[len(sealed)-tagSize:]...)
	out = append(out, sealed[:len(sealed)-tagSize]...)
	return out, nil
}

// CheckAuth 报告元数据里带的凭据片段是否与已知凭据一致。
//
// 这是最省事的"共享密钥算对了没有"的自检：只要它成立，说明客户端确实在跟我们协商。
func (r *Request) CheckAuth(auth Auth) (head, value bool) {
	return string(r.AuthHead) == string(auth.AuthKey[:32]),
		string(r.AuthValue) == string(auth.AuthKey[32:34])
}

func md5Of(parts ...[]byte) []byte {
	digest := md5.New()
	for _, part := range parts {
		digest.Write(part)
	}
	return digest.Sum(nil)
}

// mix 复现客户端那段混淆：从 shared 前 16 字节出发，按 preMix 的每个字节三次寻址改写。
//
// 注意 v0 用的是 mixed[index] 而不是 mixed[0]：这一处差别是**照着 WBArts 的
// server/ranking/codec.go 逐字对出来的**，写错的表现是元数据解得开（那一步只用到
// shared）而载荷一律"认证失败"。原来这里是 mixed[0]，测试之所以没抓住，是因为当时
// 只用本包自己写的客户端方向做对拍——两边同时错就永远对得上。现在有一条把客户端
// 方向的输出钉在**原版 DLL 实测向量**上的测试（oracle_test.go）。
func mix(shared, preMix, commonTail []byte) []byte {
	mixed := append([]byte(nil), shared[:16]...)
	for index, x := range preMix {
		v0 := x ^ mixed[index]
		i0 := int(v0 & 0x0F)
		v1 := x ^ shared[i0]
		i1 := int(v1 & 0x0F)
		v2 := x ^ mixed[i1]
		i2 := int(v2 & 0x0F)
		mixed[i1] ^= v0
		mixed[i2] ^= v1
		v3 := x ^ commonTail[i2]
		i3 := int(v3 & 0x0F)
		mixed[i3] ^= v2
		mixed[i0] ^= v3
	}
	return mixed
}

func concat(parts ...[]byte) []byte {
	size := 0
	for _, part := range parts {
		size += len(part)
	}
	out := make([]byte, 0, size)
	for _, part := range parts {
		out = append(out, part...)
	}
	return out
}
