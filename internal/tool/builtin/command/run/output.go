package run

import (
	"encoding/base64"
	"encoding/json"
	"slices"
	"sync"
	"unicode/utf8"

	"github.com/xiaoyu98628/callum/internal/tool"
)

const retainedBytes = 16 << 10
const encodedStreamBytes = 24 << 10

type outputData struct {
	Encoding  string `json:"encoding"`
	Data      string `json:"data"`
	Bytes     int64  `json:"bytes"`
	Truncated bool   `json:"truncated"`
}

type outputBuffer struct {
	data  []byte
	total int64
}

type capture struct {
	mu              sync.Mutex
	stdout, stderr  outputBuffer
	emit            func(tool.OutputChunk) error
	cancel          func()
	emitErr         error
	events, emitted int
	streamTruncated bool
}

// write 持续排空管道；保留和推送分别有界，截断不会让子进程阻塞在输出上。
func (c *capture) write(stream string, data []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	buffer := &c.stdout
	if stream == "stderr" {
		buffer = &c.stderr
	}
	buffer.total += int64(len(data))
	keep := min(len(data), retainedBytes-len(buffer.data))
	buffer.data = append(buffer.data, data[:keep]...)
	if c.emit == nil || c.emitErr != nil {
		return
	}
	keep = min(len(data), tool.MaxOutputBytes-c.emitted)
	if c.events >= tool.MaxOutputEvents {
		keep = 0
	}
	if keep < len(data) {
		c.streamTruncated = true
	}
	if keep == 0 {
		return
	}
	c.events++
	c.emitted += keep
	if err := c.emit(tool.OutputChunk{Stream: stream, Data: data[:keep]}); err != nil {
		c.emitErr = err
		c.cancel()
	}
}

func (c *capture) failure() error { c.mu.Lock(); defer c.mu.Unlock(); return c.emitErr }

func (b outputBuffer) result() outputData {
	text := utf8.Valid(b.data)
	makeResult := func(n int) outputData {
		data := b.data[:n]
		encoding, value := "base64", ""
		if text {
			for !utf8.Valid(data) {
				data = data[:len(data)-1]
			}
			encoding, value = "utf-8", string(data)
		} else {
			value = base64.StdEncoding.EncodeToString(data)
		}
		return outputData{encoding, value, b.total, int64(len(data)) < b.total}
	}
	// 按实际 JSON 编码长度限制，转义字符或二进制输出也不能突破工具结果上限。
	low, high := 0, len(b.data)
	for low < high {
		middle := (low + high + 1) / 2
		raw, _ := json.Marshal(makeResult(middle)) // 仅包含字符串、布尔与整数。
		if len(raw) <= encodedStreamBytes {
			low = middle
		} else {
			high = middle - 1
		}
	}
	return makeResult(low)
}

func (c *capture) results() (outputData, outputData, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return (outputBuffer{slices.Clone(c.stdout.data), c.stdout.total}).result(), (outputBuffer{slices.Clone(c.stderr.data), c.stderr.total}).result(), c.streamTruncated
}
