package app

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/xiaoyu98628/callum/internal/config"
)

func TestLargeModelResultUsesBoundedUTF8Fragments(t *testing.T) {
	// 各用例独占临时目录、运行时及受控端点，可与其他应用回归并行。
	t.Parallel()
	large := strings.Repeat("源", 3<<20)
	reasoning := strings.Repeat("a", (64<<10)-2) + "🙂" + strings.Repeat("中", 1024)
	reasoningJSON, err := json.Marshal(reasoning)
	if err != nil {
		t.Fatal(err)
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// 原始结果超过单条通信限制，但模型正文很短；步骤通知必须只发送元数据。
		if _, err := io.WriteString(w, `{"provider_extra":"`+large+`","choices":[{"index":0,"message":{"role":"assistant","content":"完成","reasoning_content":`+string(reasoningJSON)+`},"finish_reason":"stop"}]}`); err != nil && r.Context().Err() == nil {
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
	offset := 0
	for !strings.Contains(chunk.Fragment, "源") {
		if chunk.Done || chunk.NextOffset <= offset {
			t.Fatal("未保存原始供应商字段")
		}
		offset = chunk.NextOffset
		page := p.call(t, "step.data", map[string]any{"step_id": stepID, "field": "output", "offset": offset})
		requireOK(t, page)
		if err := json.Unmarshal(page.Result, &chunk); err != nil {
			t.Fatal(err)
		}
	}
	start := offset + strings.Index(chunk.Fragment, "源")
	part := p.call(t, "step.data", map[string]any{"step_id": stepID, "field": "output", "offset": start, "limit": 4})
	requireOK(t, part)
	if err := json.Unmarshal(part.Result, &chunk); err != nil {
		t.Fatal(err)
	}
	if chunk.Fragment != "源" || chunk.NextOffset != start+3 {
		t.Fatalf("UTF-8 字符被截断: %+v", chunk)
	}
	invalid := p.call(t, "step.data", map[string]any{"step_id": stepID, "field": "output", "offset": start + 1, "limit": 4})
	if invalid.Error == nil || invalid.Error.Code != -32602 {
		t.Fatalf("允许从字符中间开始: %+v", invalid)
	}
	var saved strings.Builder
	for offset := 0; ; {
		page := p.call(t, "step.data", map[string]any{"step_id": stepID, "field": "reasoning", "offset": offset})
		requireOK(t, page)
		if err := json.Unmarshal(page.Result, &chunk); err != nil {
			t.Fatal(err)
		}
		if chunk.TotalBytes != len(reasoning) || chunk.NextOffset <= offset || len(chunk.Fragment) > 64<<10 || !utf8.ValidString(chunk.Fragment) {
			t.Fatalf("思考分页无效: bytes=%d offset=%d next=%d", chunk.TotalBytes, offset, chunk.NextOffset)
		}
		saved.WriteString(chunk.Fragment)
		if chunk.Done {
			break
		}
		offset = chunk.NextOffset
	}
	if saved.String() != reasoning {
		t.Fatalf("思考内容不完整: bytes=%d，应为 %d", saved.Len(), len(reasoning))
	}
	requireOK(t, p.call(t, "run.get", map[string]any{"id": rid}))
	p.stop(t)
}
