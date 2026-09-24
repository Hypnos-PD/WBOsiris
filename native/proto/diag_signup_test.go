package nativeproto

import (
	"bytes"
	"crypto/ecdh"
	"crypto/md5"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// 注册（/Account/signUp）那条信封解不开：共享密钥与 authKey 都对得上，只有 KDF 的
// 输入对不上。这条临时诊断把 sid/uuid/selector 的候选组合穷举一遍，找出客户端注册时
// 到底用了什么。跑完就删，不进仓库。
func TestDiagSignUpEnvelope(t *testing.T) {
	const (
		captureDir = "/home/aspharos/wbo-cap-signup"
		keyFile    = "/home/aspharos/stateSignUp/session-key"
		authBase64 = "cGfMEcF57uS+DpXzovgT9YK4TmdJQ3sF0YcEgxuOwYIn6ejXlCGn9q3gom1utK19qn8="
	)
	private, err := os.ReadFile(keyFile)
	if err != nil {
		t.Skip(err)
	}
	key, err := ecdh.X25519().NewPrivateKey(private)
	if err != nil {
		t.Fatal(err)
	}
	authKey, _ := base64.StdEncoding.DecodeString(authBase64)
	commonTail, _ := hex.DecodeString("6854C854E96E762E4F18C9F7370608BE437ECBBA")
	wire := bodyOf(t, captureDir, 4)

	zero := make([]byte, 16)
	clientIDBig := make([]byte, 16)
	binary.BigEndian.PutUint64(clientIDBig[8:], 659376525264)
	clientIDLittle := make([]byte, 16)
	binary.LittleEndian.PutUint64(clientIDLittle[:8], 659376525264)
	clientIDASCII := append([]byte("659376525264"), make([]byte, 5)...)
	savedUUID, _ := hex.DecodeString("4466cfd0323b4a2580e17056accada88")
	md5OfAuth := func() []byte { sum := md5.Sum(authKey); return append([]byte(nil), sum[:16]...) }
	headers := headersOf(t, captureDir, 4)

	sids := map[string][]byte{"头里的 Sid": headers, "上一个 Sid": mustHex(t, "5a7784e0cd065093aa7b4d638bce451b"), "全零": zero}
	uuids := map[string][]byte{
		"存档": savedUUID, "全零": zero, "client_id 大端": clientIDBig,
		"client_id 小端": clientIDLittle, "client_id 十进制": clientIDASCII,
		"头里的 Sid": headers, "authKey 前 16": authKey[:16], "md5(authKey)": md5OfAuth(),
	}
	selectors := map[string][]byte{
		"authKey[34:50]": authKey[34:50], "全零": zero, "authKey[32:48]": authKey[32:48],
		"authKey[35:51]": authKey[35:51], "md5(authKey)": md5OfAuth(),
	}

	server := &Server{private: key, commonTail: commonTail}
	var tried, hits int
	for sidName, sid := range sids {
		for uuidName, uuid := range uuids {
			for selectorName, selector := range selectors {
				keyed := append([]byte(nil), authKey...)
				copy(keyed[34:50], selector)
				tried++
				request, err := server.DecodeRequest(wire, Auth{SID: sid, UUID: uuid, AuthKey: keyed})
				if err != nil {
					continue
				}
				hits++
				t.Logf("命中：sid=%s uuid=%s selector=%s → %d 字节: %s", sidName, uuidName, selectorName,
					len(request.Plain), string(request.Plain[:min(200, len(request.Plain))]))
			}
		}
	}
	t.Logf("共试 %d 组，命中 %d 组", tried, hits)
}

func mustHex(t *testing.T, value string) []byte {
	t.Helper()
	decoded, err := hex.DecodeString(value)
	if err != nil {
		t.Fatal(err)
	}
	return decoded
}

// headersOf 取某条请求的 Sid 头（十六进制 → 16 字节）。
func headersOf(t *testing.T, captureDir string, sequence int) []byte {
	t.Helper()
	raw, err := os.ReadFile(captureDir + "/requests.jsonl")
	if err != nil {
		t.Skip(err)
	}
	for _, line := range bytes.Split(raw, []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		var record struct {
			Sequence int                 `json:"sequence"`
			Headers  map[string][]string `json:"headers"`
		}
		if json.Unmarshal(line, &record) != nil || record.Sequence != sequence {
			continue
		}
		for name, values := range record.Headers {
			if strings.EqualFold(name, "Sid") && len(values) > 0 {
				return mustHex(t, values[0])
			}
		}
	}
	return nil
}

// bodyOf 读第 n 条请求的原始 body（base64 文本）并解出信封。
func bodyOf(t *testing.T, captureDir string, sequence int) []byte {
	t.Helper()
	path := captureDir + "/bodies/" + padSequence(sequence) + ".bin"
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Skip(err)
	}
	wire, err := base64.StdEncoding.DecodeString(string(raw))
	if err != nil {
		t.Fatalf("body 不是 base64: %v", err)
	}
	return wire
}

func padSequence(sequence int) string {
	text := "000000"
	digits := []byte{}
	for value := sequence; value > 0; value /= 10 {
		digits = append([]byte{byte('0' + value%10)}, digits...)
	}
	if len(digits) == 0 {
		digits = []byte{'0'}
	}
	return text[:len(text)-len(digits)] + string(digits)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
