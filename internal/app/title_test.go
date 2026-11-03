package app

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"

	"github.com/xiaoyu98628/callum/internal/config"
	"github.com/xiaoyu98628/callum/internal/run"
)

func TestManualTitleWinsAndIncompleteAnswerKeepsUsage(t *testing.T) {
	allowTitle, allowAnswer := make(chan struct{}), make(chan struct{})
	var titleOnce, answerOnce sync.Once
	releaseTitle := func() { titleOnce.Do(func() { close(allowTitle) }) }
	releaseAnswer := func() { answerOnce.Do(func() { close(allowAnswer) }) }
	t.Cleanup(releaseTitle)
	t.Cleanup(releaseAnswer)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Stream   bool `json:"stream"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
			return
		}
		if req.Messages[0].Role == "system" {
			select {
			case <-allowTitle:
			case <-r.Context().Done():
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"choices":[{"index":0,"message":{"role":"assistant","content":"不应覆盖的标题"},"finish_reason":"stop"}]}`)
			return
		}
		if req.Messages[len(req.Messages)-1].Content == "触发鉴权失败" {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-Request-ID", "auth-request")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"error":{"message":"fake-dialogue-key","type":"authentication_error"}}`)
			return
		}
		if req.Stream {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"正文\"},\"finish_reason\":null}]}\n\n")
			w.(http.Flusher).Flush()
			select {
			case <-allowAnswer:
			case <-r.Context().Done():
				return
			}
			_, _ = io.WriteString(w, "data: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"index":0,"message":{"role":"assistant","content":"未完成内容"},"finish_reason":"length"}],"usage":{"total_tokens":9,"vendor":"kept"}}`)
	}))
	t.Cleanup(upstream.Close)
	cfg, err := config.Load(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatal(err)
	}
	cfg.MaxConcurrentRuns = 2
	p := startApp(t, cfg, io.Discard)
	mid := setupConversationModel(t, p, upstream.URL)
	accepted := p.call(t, "conversation.send", map[string]any{"model_id": mid, "content": messageBlocks("标题测试")})
	convID := resultString(t, accepted, "conversation_id")
	p.event(t, "message.delta")
	requireOK(t, p.call(t, "conversation.update", map[string]any{"id": convID, "title": "人工改名"}))
	releaseTitle()
	for {
		frame := p.event(t, "step.updated")
		if bytes.Contains(frame.Params, []byte("生成会话标题")) && bytes.Contains(frame.Params, []byte(`"status":"completed"`)) {
			break
		}
	}
	releaseAnswer()
	p.event(t, "run.completed")
	got := p.call(t, "conversation.get", map[string]any{"id": convID})
	requireOK(t, got)
	if !bytes.Contains(got.Result, []byte(`"title":"人工改名"`)) || !bytes.Contains(got.Result, []byte(`"title_source":"manual"`)) {
		t.Fatalf("生成标题覆盖了人工标题: %s", got.Result)
	}
	failed := p.call(t, "conversation.send", map[string]any{"conversation_id": convID, "stream": false, "content": messageBlocks("触发截断")})
	rid := resultString(t, failed, "run_id")
	p.event(t, "run.failed")
	rows := readMessages(t, p, convID)
	if len(rows) != 4 || rows[3].Status != run.Failed || rows[3].CompletedAt != nil || rows[3].Content[0].Text != "未完成内容" {
		t.Fatalf("不完整回答被当成成功: %+v", rows)
	}
	stepID := resultStringFromSteps(t, p.call(t, "step.list", map[string]any{"run_id": rid}))
	usage := p.call(t, "step.data", map[string]any{"step_id": stepID, "field": "usage"})
	requireOK(t, usage)
	if !bytes.Contains(usage.Result, []byte("kept")) {
		t.Fatalf("异常结束丢失已返回 usage: %s", usage.Result)
	}
	auth := p.call(t, "conversation.send", map[string]any{"conversation_id": convID, "stream": false, "content": messageBlocks("触发鉴权失败")})
	authRun := resultString(t, auth, "run_id")
	authFailure := p.event(t, "run.failed")
	if !bytes.Contains(authFailure.Params, []byte("HTTP 401")) || bytes.Contains(authFailure.Params, []byte("fake-dialogue-key")) {
		t.Fatalf("鉴权错误丢失状态或泄露错误体: %s", authFailure.Params)
	}
	authStep := resultStringFromSteps(t, p.call(t, "step.list", map[string]any{"run_id": authRun}))
	details := p.call(t, "step.data", map[string]any{"step_id": authStep, "field": "output"})
	requireOK(t, details)
	if !bytes.Contains(details.Result, []byte("request_error")) || !bytes.Contains(details.Result, []byte("401")) || !bytes.Contains(details.Result, []byte("auth-request")) || bytes.Contains(details.Result, []byte("fake-dialogue-key")) {
		t.Fatalf("未保存受控供应商错误信息: %s", details.Result)
	}
	p.stop(t)
}

func resultStringFromSteps(t *testing.T, f wireFrame) string {
	t.Helper()
	requireOK(t, f)
	var rows []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(f.Result, &rows); err != nil || len(rows) != 1 {
		t.Fatalf("步骤数量不正确: %s", f.Result)
	}
	return rows[0].ID
}
