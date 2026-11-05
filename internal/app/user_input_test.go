package app

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/xiaoyu98628/callum/internal/agent"
	"github.com/xiaoyu98628/callum/internal/config"
)

func TestUserInputThroughStdio(t *testing.T) {
	for _, mode := range []string{"option", "text", "skip", "cancel", "close", "invalid"} {
		t.Run(mode, func(t *testing.T) {
			var replies atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request struct {
					Stream   bool `json:"stream"`
					Messages []struct {
						Role    string `json:"role"`
						Content string `json:"content"`
					} `json:"messages"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
					w.WriteHeader(400)
					return
				}
				if !request.Stream {
					w.Header().Set("Content-Type", "application/json")
					fmt.Fprint(w, `{"choices":[{"index":0,"message":{"role":"assistant","content":""},"finish_reason":"stop"}]}`)
					return
				}
				w.Header().Set("Content-Type", "text/event-stream")
				last := request.Messages[len(request.Messages)-1]
				if last.Role == "tool" || last.Content == "继续对话" {
					replies.Add(1)
					fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"已收到回复\"},\"finish_reason\":null}]}\n\ndata: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
					return
				}
				arguments := `{"question":"另一个任务是否已结束？","options":["已结束，可以复验","仍在进行"]}`
				if mode == "invalid" {
					arguments = `{"question":"问题","options":["重复","重复"]}`
				}
				data, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": "ask-call", "type": "function", "function": map[string]string{"name": agent.UserInputTool, "arguments": arguments}}}}, "finish_reason": nil}}})
				fmt.Fprintf(w, "data: %s\n\ndata: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"tool_calls\"}]}\n\ndata: [DONE]\n\n", data)
			}))
			t.Cleanup(upstream.Close)
			cfg, err := config.Load(filepath.Join(t.TempDir(), "data"))
			if err != nil {
				t.Fatal(err)
			}
			p := startApp(t, cfg, io.Discard)
			modelID := setupConversationModel(t, p, upstream.URL)
			accepted := p.call(t, "conversation.send", map[string]any{"model_id": modelID, "stream": true, "content": messageBlocks("请确认任务")})
			runID := resultString(t, accepted, "run_id")
			conversationID := resultString(t, accepted, "conversation_id")
			if mode == "invalid" {
				p.event(t, "run.completed")
				requireOK(t, p.call(t, "conversation.send", map[string]any{"conversation_id": conversationID, "stream": true, "content": messageBlocks("继续")}))
				p.event(t, "run.completed")
				p.stop(t)
				return
			}
			frame := p.event(t, "user_input.requested")
			var payload struct {
				Input   agent.UserInput `json:"user_input"`
				RunID   string          `json:"run_id"`
				InputID string          `json:"input_id"`
			}
			if err := json.Unmarshal(frame.Params, &payload); err != nil {
				t.Fatal(err)
			}
			if payload.RunID != runID || payload.Input.ID != payload.InputID || payload.Input.Status != "pending" || len(payload.Input.Options) != 2 {
				t.Fatalf("问题归属或快照错误 %s", frame.Params)
			}
			if replies.Load() != 0 {
				t.Fatal("用户回复前已继续模型调用")
			}
			pending := p.call(t, "user_input.list", map[string]any{"run_id": runID, "status": "pending"})
			requireOK(t, pending)
			if !strings.Contains(string(pending.Result), payload.InputID) {
				t.Fatal("查询未恢复等待问题")
			}
			if mode == "cancel" || mode == "close" {
				if mode == "cancel" {
					requireOK(t, p.call(t, "run.cancel", map[string]any{"run_id": runID}))
					p.event(t, "run.cancelled")
				} else {
					p.stop(t)
					p = startApp(t, cfg, io.Discard)
				}
				current := p.call(t, "user_input.get", map[string]any{"id": payload.InputID})
				requireOK(t, current)
				if !strings.Contains(string(current.Result), `"status":"cancelled"`) {
					t.Fatalf("取消问题未保存 %s", current.Result)
				}
				if replies.Load() != 0 {
					t.Fatal("取消后继续调用模型")
				}
				p.stop(t)
				return
			}
			answer := agent.InputAnswer{}
			switch mode {
			case "option":
				answer.OptionIndex = new(0)
			case "text":
				answer.Text = "自己的补充回复"
			case "skip":
				answer.Skipped = true
			}
			if rejected := p.call(t, "user_input.submit", map[string]any{"input_id": payload.InputID, "run_id": runID, "answer": agent.InputAnswer{OptionIndex: new(99)}}); rejected.Error == nil {
				t.Fatal("接受越界选项")
			}
			other := runID[:len(runID)-1] + "f"
			if other == runID {
				other = runID[:len(runID)-1] + "e"
			}
			if rejected := p.call(t, "user_input.submit", map[string]any{"input_id": payload.InputID, "run_id": other, "answer": answer}); rejected.Error == nil {
				t.Fatal("接受跨执行回复")
			}
			params := map[string]any{"input_id": payload.InputID, "run_id": runID, "answer": answer}
			requireOK(t, p.call(t, "user_input.submit", params))
			requireOK(t, p.call(t, "user_input.submit", params))
			if rejected := p.call(t, "user_input.submit", map[string]any{"input_id": payload.InputID, "run_id": runID, "answer": agent.InputAnswer{Text: "冲突改答"}}); rejected.Error == nil {
				t.Fatal("覆盖了已保存答案")
			}
			p.event(t, "run.completed")
			if replies.Load() != 1 {
				t.Fatal("回复重复触发模型调用", replies.Load())
			}
			p.stop(t)
			p = startApp(t, cfg, io.Discard)
			current := p.call(t, "user_input.get", map[string]any{"id": payload.InputID})
			requireOK(t, current)
			expected := "answered"
			if mode == "skip" {
				expected = "skipped"
			}
			if !strings.Contains(string(current.Result), `"status":"`+expected+`"`) {
				t.Fatalf("重启丢失回复 %s", current.Result)
			}
			requireOK(t, p.call(t, "conversation.send", map[string]any{"conversation_id": conversationID, "stream": true, "content": messageBlocks("继续对话")}))
			p.event(t, "run.completed")
			p.stop(t)
		})
	}
}
