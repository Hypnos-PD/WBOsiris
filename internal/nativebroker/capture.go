// Package nativebroker 是本地的"服务端"：原版客户端把请求发到这里。
//
// 它现在只做两件事——把收到的每一个请求完整录下来，以及把已经实现的路由交给
// 调用方。录制的意义在于：客户端的目标地址、请求体结构、调用顺序都可以从真实流量
// 里读出来，不必再靠逆向猜测。
package nativebroker

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Record 是一次请求的索引条目。请求体单独存成原始字节，避免在 JSON 里做有损编码。
type Record struct {
	Sequence  int         `json:"sequence"`
	Time      time.Time   `json:"time"`
	Method    string      `json:"method"`
	Path      string      `json:"path"`
	Query     string      `json:"query,omitempty"`
	Protocol  string      `json:"protocol"`
	Headers   http.Header `json:"headers"`
	Bytes     int         `json:"bytes"`
	Body      string      `json:"body,omitempty"` // 原始请求体的相对路径
	Status    int         `json:"status"`
	Handled   bool        `json:"handled"`
	Responded int         `json:"respondedBytes"`
}

// capture 把请求按顺序落盘：一份可读的 JSONL 索引，加一份原始请求体。
type capture struct {
	directory string
	guard     sync.Mutex
	sequence  int
}

func newCapture(directory string) (*capture, error) {
	if directory == "" {
		return nil, nil
	}
	if err := os.MkdirAll(filepath.Join(directory, "bodies"), 0o755); err != nil {
		return nil, err
	}
	return &capture{directory: directory}, nil
}

func (c *capture) write(record *Record, body []byte, response []byte) error {
	c.guard.Lock()
	defer c.guard.Unlock()
	c.sequence++
	record.Sequence = c.sequence
	record.Time = time.Now()
	record.Bytes = len(body)
	record.Responded = len(response)
	if len(body) > 0 {
		name := fmt.Sprintf("bodies/%06d.bin", c.sequence)
		if err := os.WriteFile(filepath.Join(c.directory, filepath.FromSlash(name)), body, 0o644); err != nil {
			return err
		}
		record.Body = name
	}
	encoded, err := json.Marshal(record)
	if err != nil {
		return err
	}
	handle, err := os.OpenFile(filepath.Join(c.directory, "requests.jsonl"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer handle.Close()
	if _, err := handle.Write(append(encoded, '\n')); err != nil {
		return err
	}
	return nil
}
