package app

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/xiaoyu98628/callum/internal/config"
	"github.com/xiaoyu98628/callum/internal/conversation"
)

func TestFollowUpQueueAndSteerThroughStdio(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	var guided, restored atomic.Bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Stream   bool
			Tools    []json.RawMessage
			Messages []struct{ Role, Content string }
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil || len(input.Messages) == 0 {
			t.Errorf("模型请求无效: %v", err)
			return
		}
		last := input.Messages[len(input.Messages)-1]
		if len(input.Tools) > 0 {
			switch last.Content {
			case "开始任务":
				close(started)
				select {
				case <-release:
				case <-r.Context().Done():
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"choices":[{"index":0,"message":{"role":"assistant","tool_calls":[{"id":"obsolete","type":"function","function":{"name":"request_user_input","arguments":"{\"question\":\"旧问题\",\"options\":[\"A\"]}"}}]},"finish_reason":"tool_calls"}]}`)
				return
			case "改用新要求":
				for _, m := range input.Messages {
					if m.Role == "tool" && strings.Contains(m.Content, "steered") {
						guided.Store(true)
					}
				}
			case "调整后的队列消息":
				for _, m := range input.Messages {
					if m.Role == "user" && m.Content == "改用新要求" {
						restored.Store(true)
					}
				}
			}
		}
		if input.Stream {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"已处理\"},\"finish_reason\":null}]}\n\ndata: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"index":0,"message":{"role":"assistant","content":"已处理"},"finish_reason":"stop"}]}`)
	}))
	t.Cleanup(upstream.Close)
	runtime, _ := runtimeFixture(t, 2)
	modelID := directModel(t, runtime, upstream.URL)
	p := sharedPeer(t, runtime)
	response := p.call(t, "conversation.send", map[string]any{"model_id": modelID, "stream": false, "content": messageBlocks("开始任务")})
	id, runID := resultString(t, response, "conversation_id"), resultString(t, response, "run_id")
	waitSignal(t, started)
	add := func(body string) string {
		t.Helper()
		return resultString(t, p.call(t, "conversation.follow_up_add", map[string]any{
			"conversation_id": id, "expected_run_id": runID, "mode": "queue", "content": messageBlocks(body),
		}), "id")
	}
	guide, queued, removed := add("改用新要求"), add("稍后处理"), add("需要移除")
	for _, action := range []map[string]any{
		{"id": queued, "action": "edit", "text": "调整后的队列消息"},
		{"id": removed, "action": "remove"},
		{"id": guide, "action": "steer", "expected_run_id": runID},
	} {
		action["conversation_id"], action["revision"] = id, 1
		requireOK(t, p.call(t, "conversation.follow_up_change", action))
	}
	close(release)
	p.event(t, "run.completed")
	if !guided.Load() {
		t.Fatal("引导未进入同一执行的下一模型请求，或旧工具没有被跳过")
	}
	response = p.call(t, "conversation.follow_ups", map[string]any{"conversation_id": id})
	requireOK(t, response)
	var items []conversation.FollowUp
	if err := json.Unmarshal(response.Result, &items); err != nil || len(items) != 1 || items[0].ID != queued || items[0].Revision != 2 {
		t.Fatalf("队列状态错误: %s, %v", response.Result, err)
	}
	request := map[string]any{"conversation_id": id, "id": queued, "revision": 1, "after_run_id": runID}
	if got := p.call(t, "conversation.follow_up_start", request); got.Error == nil {
		t.Fatal("旧队列版本被受理")
	}
	request["revision"] = 2
	response = p.call(t, "conversation.follow_up_start", request)
	if resultString(t, response, "run_id") == runID {
		t.Fatal("排队消息未开启新执行")
	}
	p.event(t, "run.completed")
	if !restored.Load() {
		t.Fatal("新执行上下文未恢复引导消息")
	}
	response = p.call(t, "conversation.follow_ups", map[string]any{"conversation_id": id})
	requireOK(t, response)
	if string(response.Result) != "[]" {
		t.Fatalf("已发送项目仍留在待发队列: %s", response.Result)
	}
}

func TestFollowUpsSurviveServiceRestart(t *testing.T) {
	started := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input struct{ Tools []json.RawMessage }
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			t.Errorf("模型请求无效: %v", err)
			return
		}
		if len(input.Tools) != 0 {
			close(started)
			<-r.Context().Done()
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"index":0,"message":{"role":"assistant","content":"标题"},"finish_reason":"stop"}]}`)
	}))
	t.Cleanup(upstream.Close)
	cfg, err := config.Load(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatal(err)
	}
	p := startApp(t, cfg, io.Discard)
	modelID := setupConversationModel(t, p, upstream.URL)
	result := p.call(t, "conversation.send", map[string]any{
		"model_id": modelID, "stream": false, "content": messageBlocks("执行中"),
	})
	conversationID, runID := resultString(t, result, "conversation_id"), resultString(t, result, "run_id")
	waitSignal(t, started)

	path := filepath.Join(t.TempDir(), "document.txt")
	original := []byte("原始附件")
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	queued := resultString(t, p.call(t, "conversation.follow_up_add", map[string]any{
		"conversation_id": conversationID, "expected_run_id": runID, "mode": "queue",
		"content": []map[string]string{{"type": "file", "path": path, "name": "document.txt"}},
	}), "id")
	steer := resultString(t, p.call(t, "conversation.follow_up_add", map[string]any{
		"conversation_id": conversationID, "expected_run_id": runID, "mode": "steer", "content": messageBlocks("待用引导"),
	}), "id")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	p.stop(t)

	reopened := startApp(t, cfg, io.Discard)
	result = reopened.call(t, "conversation.follow_ups", map[string]any{"conversation_id": conversationID})
	requireOK(t, result)
	var items []conversation.FollowUp
	err = json.Unmarshal(result.Result, &items)
	if err != nil || len(items) != 2 || items[0].ID != queued || items[1].ID != steer || items[1].Status != conversation.FollowUpQueued {
		t.Fatalf("停机后队列或引导未恢复: %s, %v", result.Result, err)
	}
	ref := items[0].Content[0].AttachmentRef
	preserved, err := os.ReadFile(filepath.Join(cfg.DataDir, "attachments", filepath.FromSlash(ref)))
	if err != nil || string(preserved) != string(original) {
		t.Fatalf("托管附件未保留: %q, %v", preserved, err)
	}
}
