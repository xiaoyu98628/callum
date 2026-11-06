package run

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/xiaoyu98628/callum/internal/tool"
)

func TestOutputEncodingAndEscapedSize(t *testing.T) {
	for _, data := range [][]byte{[]byte(strings.Repeat("\x00\"\\", 10000)), []byte(strings.Repeat("中文", 10000)), bytes.Repeat([]byte{255}, 20000)} {
		buffer := outputBuffer{data: data[:min(len(data), retainedBytes)], total: int64(len(data))}
		value := buffer.result()
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if len(raw) > encodedStreamBytes || !value.Truncated || value.Bytes != int64(len(data)) {
			t.Fatalf("编码后越界或丢失截断标识: size=%d value=%+v", len(raw), value)
		}
		if value.Encoding == "base64" {
			decoded, err := base64.StdEncoding.DecodeString(value.Data)
			if err != nil || !bytes.HasPrefix(data, decoded) {
				t.Fatal("二进制输出损坏")
			}
		} else if !strings.HasPrefix(string(data), value.Data) {
			t.Fatal("文本前缀改变")
		}
	}
}

func TestOutputEventCountAndFailureStayBounded(t *testing.T) {
	calls := 0
	collector := &capture{emit: func(tool.OutputChunk) error { calls++; return nil }, cancel: func() {}}
	for range tool.MaxOutputEvents + 10 {
		collector.write("stdout", []byte("x"))
	}
	if calls != tool.MaxOutputEvents || !collector.streamTruncated || collector.stdout.total != int64(tool.MaxOutputEvents+10) {
		t.Fatal("碎片事件未限制或未继续排空")
	}
	failure := errors.New("emit failure")
	cancelled := false
	collector = &capture{emit: func(tool.OutputChunk) error { return failure }, cancel: func() { cancelled = true }}
	collector.write("stderr", []byte("first"))
	collector.write("stderr", []byte("second"))
	if !cancelled || !errors.Is(collector.failure(), failure) || string(collector.stderr.data) != "firstsecond" {
		t.Fatal("发布失败没有取消或丢失已读取内容")
	}
}
