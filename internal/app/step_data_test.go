package app

import (
	"encoding/json"
	"github.com/xiaoyu98628/callum/internal/config"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestLargeModelResultUsesBoundedUTF8Fragments(t *testing.T) {
	large := strings.Repeat("中", 3<<20)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// 原始结果超过单条通信限制，但模型正文很短；步骤通知必须只发送元数据。
		if _, err := io.WriteString(w, `{"provider_extra":"`+large+`","choices":[{"index":0,"message":{"role":"assistant","content":"完成"},"finish_reason":"stop"}]}`); err != nil && r.Context().Err() == nil {
			t.Error(err)
		}
	}))
	t.Cleanup(upstream.Close)
	cfg, err := config.Load(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatal(err)
	}
	cfg.MaxConcurrentRuns = 1
	p := startApp(t, cfg, io.Discard)
	mid := setupConversationModel(t, p, upstream.URL)
	accepted := p.call(t, "conversation.send", map[string]any{"model_id": mid, "stream": false, "content": messageBlocks("原始结果测试")})
	rid := resultString(t, accepted, "run_id")
	p.event(t, "run.completed")
	stepID := resultStringFromSteps(t, p.call(t, "step.list", map[string]any{"run_id": rid}))
	first := p.call(t, "step.data", map[string]any{"step_id": stepID, "field": "output"})
	requireOK(t, first)
	var chunk struct {
		Fragment   string `json:"fragment"`
		TotalBytes int    `json:"total_bytes"`
		Done       bool   `json:"done"`
		NextOffset int    `json:"next_offset"`
	}
	if err := json.Unmarshal(first.Result, &chunk); err != nil {
		t.Fatal(err)
	}
	if chunk.TotalBytes <= 8<<20 || chunk.Done || len(chunk.Fragment) > 64<<10 || !utf8.ValidString(chunk.Fragment) {
		t.Fatalf("结果没有分段: bytes=%d next=%d", chunk.TotalBytes, chunk.NextOffset)
	}
	start := strings.Index(chunk.Fragment, "中")
	if start < 0 {
		t.Fatal("未保存原始供应商字段")
	}
	part := p.call(t, "step.data", map[string]any{"step_id": stepID, "field": "output", "offset": start, "limit": 4})
	requireOK(t, part)
	if err := json.Unmarshal(part.Result, &chunk); err != nil {
		t.Fatal(err)
	}
	if chunk.Fragment != "中" || chunk.NextOffset != start+3 {
		t.Fatalf("UTF-8 字符被截断: %+v", chunk)
	}
	invalid := p.call(t, "step.data", map[string]any{"step_id": stepID, "field": "output", "offset": start + 1, "limit": 4})
	if invalid.Error == nil || invalid.Error.Code != -32602 {
		t.Fatalf("允许从字符中间开始: %+v", invalid)
	}
	requireOK(t, p.call(t, "run.get", map[string]any{"id": rid}))
	p.stop(t)
}
