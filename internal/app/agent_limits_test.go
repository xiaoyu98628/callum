package app

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/xiaoyu98628/callum/internal/agent"
	"github.com/xiaoyu98628/callum/internal/config"
	"github.com/xiaoyu98628/callum/internal/inference"
)

func setupToolWorkspace(t *testing.T, p *appPeer) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("text"), 0600); err != nil {
		t.Fatal(err)
	}
	return resultString(t, p.call(t, "workspace.create", map[string]any{"name": "工作区", "root_path": root}), "id")
}

func TestAgentRejectsRepeatedOperationAndLimitsLoop(t *testing.T) {
	var requests atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"index":0,"message":{"role":"assistant","tool_calls":[{"id":"call","type":"function","function":{"name":"read_file","arguments":"{\"path\":\"a.txt\"}"}}]},"finish_reason":"tool_calls"}],"usage":{"total_tokens":1}}`)
	}))
	t.Cleanup(upstream.Close)
	cfg, err := config.Load(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatal(err)
	}
	cfg.MaxConcurrentRuns = 1
	p := startApp(t, cfg, io.Discard)
	setApprovalPolicy(t, p, "always")
	mid := setupConversationModel(t, p, upstream.URL)
	wid := setupToolWorkspace(t, p)
	accepted := p.call(t, "conversation.send", map[string]any{"model_id": mid, "workspace_id": wid, "stream": false, "content": messageBlocks("读取")})
	runID := resultString(t, accepted, "run_id")
	event := p.event(t, "approval.requested")
	var payload struct {
		ID string `json:"approval_id"`
	}
	if err := json.Unmarshal(event.Params, &payload); err != nil {
		t.Fatal(err)
	}
	requireOK(t, p.call(t, "approval.decide", map[string]any{"approval_id": payload.ID, "decision": "rejected"}))
	failed := p.event(t, "run.failed")
	var result struct {
		Run agent.Run `json:"run"`
	}
	if err := json.Unmarshal(failed.Params, &result); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 8 || result.Run.ErrorKind == nil || *result.Run.ErrorKind != "iteration_limit" {
		t.Fatalf("未限制模型循环: %d %+v", requests.Load(), result.Run)
	}
	records := p.call(t, "approval.list", map[string]any{"run_id": runID})
	requireOK(t, records)
	var rows []agent.Approval
	if err := json.Unmarshal(records.Result, &rows); err != nil || len(rows) != 1 {
		t.Fatalf("相同拒绝操作重新申请审批: %s %v", records.Result, err)
	}
	p.stop(t)
}

func TestPausedAgentIsolationAndShutdown(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Messages []struct{ Role, Content string } `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			t.Error(err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if input.Messages[0].Role == "system" || input.Messages[len(input.Messages)-1].Content == "直接回答" {
			_, _ = io.WriteString(w, `{"choices":[{"index":0,"message":{"role":"assistant","content":"回答"},"finish_reason":"stop"}]}`)
			return
		}
		_, _ = io.WriteString(w, `{"choices":[{"index":0,"message":{"role":"assistant","tool_calls":[{"id":"call","type":"function","function":{"name":"read_file","arguments":"{\"path\":\"a.txt\"}"}}]},"finish_reason":"tool_calls"}]}`)
	}))
	t.Cleanup(upstream.Close)
	cfg, err := config.Load(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatal(err)
	}
	cfg.MaxConcurrentRuns = 2
	p := startApp(t, cfg, io.Discard)
	setApprovalPolicy(t, p, "always")
	mid := setupConversationModel(t, p, upstream.URL)
	wid := setupToolWorkspace(t, p)
	first := p.call(t, "conversation.send", map[string]any{"model_id": mid, "workspace_id": wid, "stream": false, "content": messageBlocks("读取")})
	firstID := resultString(t, first, "run_id")
	convID := resultString(t, first, "conversation_id")
	event := p.event(t, "approval.requested")
	var payload struct {
		ID string `json:"approval_id"`
	}
	if err := json.Unmarshal(event.Params, &payload); err != nil {
		t.Fatal(err)
	}
	busy := p.call(t, "conversation.send", map[string]any{"conversation_id": convID, "content": messageBlocks("重复")})
	if busy.Error == nil || busy.Error.Code != 1006 {
		t.Fatal("等待审批时允许同会话并发")
	}
	second := p.call(t, "conversation.send", map[string]any{"model_id": mid, "workspace_id": wid, "stream": false, "content": messageBlocks("直接回答")})
	secondID := resultString(t, second, "run_id")
	completed := p.event(t, "run.completed")
	var terminal struct {
		ID string `json:"run_id"`
	}
	if err := json.Unmarshal(completed.Params, &terminal); err != nil || terminal.ID != secondID {
		t.Fatal("审批阻塞了其他会话")
	}
	state := p.call(t, "run.get", map[string]any{"id": firstID})
	requireOK(t, state)
	var paused agent.Run
	if err := json.Unmarshal(state.Result, &paused); err != nil || paused.Status != agent.AwaitingApproval {
		t.Fatal("等待审批状态丢失")
	}
	p.stop(t)
	restored := startApp(t, cfg, io.Discard)
	record := restored.call(t, "approval.get", map[string]any{"id": payload.ID})
	requireOK(t, record)
	var a agent.Approval
	if err := json.Unmarshal(record.Result, &a); err != nil || a.Status != agent.ApprovalCancelled {
		t.Fatalf("关闭后待办未取消: %s", record.Result)
	}
	restored.stop(t)
}

func TestIncompleteToolStreamDoesNotExecute(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call\",\"type\":\"function\",\"function\":{\"name\":\"read_file\",\"arguments\":\"{\\\"path\\\":\"}}]}}]}\n\ndata: {\"choices\":[],\"usage\":{\"total_tokens\":2}}\n\ndata: [DONE]\n\n")
	}))
	t.Cleanup(upstream.Close)
	cfg, err := config.Load(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatal(err)
	}
	cfg.MaxConcurrentRuns = 1
	p := startApp(t, cfg, io.Discard)
	setApprovalPolicy(t, p, "always")
	mid := setupConversationModel(t, p, upstream.URL)
	wid := setupToolWorkspace(t, p)
	accepted := p.call(t, "conversation.send", map[string]any{"model_id": mid, "workspace_id": wid, "stream": true, "content": messageBlocks("读取")})
	runID := resultString(t, accepted, "run_id")
	p.event(t, "run.failed")
	records := p.call(t, "approval.list", map[string]any{"run_id": runID})
	requireOK(t, records)
	var approvals []agent.Approval
	if err := json.Unmarshal(records.Result, &approvals); err != nil || len(approvals) != 0 {
		t.Fatal("不完整参数触发审批")
	}
	stepsFrame := p.call(t, "step.list", map[string]any{"run_id": runID})
	requireOK(t, stepsFrame)
	var steps []agent.StepSummary
	if err := json.Unmarshal(stepsFrame.Result, &steps); err != nil || len(steps) != 1 || steps[0].Type != "model_call" || steps[0].Status != agent.Failed || steps[0].UsageBytes == 0 {
		t.Fatalf("不完整参数触发工具步骤或丢失 usage: %s", stepsFrame.Result)
	}
	p.stop(t)
}

func TestAgentStreamCumulativeTextLimit(t *testing.T) {
	for _, overLimit := range []bool{false, true} {
		t.Run(map[bool]string{false: "exact_limit", true: "over_limit"}[overLimit], func(t *testing.T) {
			initial, prefix := strings.Repeat("a", 600_000), "保留"
			remaining := inference.MaxTextBytes - len(initial) - len(prefix)
			if overLimit {
				remaining++
			}
			last := strings.Repeat("b", remaining)
			var requests atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("X-Request-ID", "limit-request")
				switch requests.Add(1) {
				case 1:
					writeFileToolResponse(t, w, true, map[string]any{"content": initial, "tool_calls": []any{map[string]any{"index": 0, "id": "read", "type": "function", "function": map[string]string{"name": "read_file", "arguments": `{"path":"a.txt"}`}}}}, "tool_calls")
				case 2:
					w.Header().Set("Content-Type", "text/event-stream")
					if _, err := io.WriteString(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"保留\"},\"finish_reason\":null}]}\n\n"); err != nil {
						t.Error(err)
						return
					}
					writeFileToolResponse(t, w, true, map[string]any{"content": last}, "stop")
				default:
					t.Error("文本超限后仍继续调用模型")
					w.WriteHeader(http.StatusInternalServerError)
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
			wid := setupToolWorkspace(t, p)
			accepted := p.call(t, "conversation.send", map[string]any{"model_id": mid, "workspace_id": wid, "stream": true, "content": messageBlocks("读取并回答")})
			runID, convID := resultString(t, accepted, "run_id"), resultString(t, accepted, "conversation_id")
			terminal, wantText, wantBytes := "run.completed", prefix+last, inference.MaxTextBytes
			if overLimit {
				terminal, wantText, wantBytes = "run.failed", prefix, len(initial)+len(prefix)
			}
			frame := p.event(t, terminal)
			var final struct {
				Run agent.Run `json:"run"`
			}
			if err := json.Unmarshal(frame.Params, &final); err != nil {
				t.Fatal(err)
			}
			if final.Run.ID != runID || (overLimit && (final.Run.ErrorKind == nil || *final.Run.ErrorKind != "output_limit")) {
				t.Fatalf("执行终态错误: %s", frame.Params)
			}
			messages := readMessages(t, p, convID)
			if len(messages) != 2 || len(messages[1].Content) != 1 || messages[1].Content[0].Text != wantText || messages[1].Status != final.Run.Status {
				t.Fatal("保存的正文与通过累计限额的增量不一致")
			}
			deltaBytes := 0
			for _, event := range p.pending {
				if event.Method == "message.delta" {
					var value struct{ Delta struct{ Text string } }
					if err := json.Unmarshal(event.Params, &value); err != nil {
						t.Fatal(err)
					}
					deltaBytes += len(value.Delta.Text)
				}
				if event.Method == "message.updated" && overLimit {
					var value struct {
						Message struct{ Content []struct{ Text string } }
					}
					if err := json.Unmarshal(event.Params, &value); err != nil {
						t.Fatal(err)
					}
					for _, block := range value.Message.Content {
						if strings.Contains(block.Text, "b") {
							t.Fatal("被拒收的增量通过消息更新返回")
						}
					}
				}
			}
			if deltaBytes != wantBytes || requests.Load() != 2 {
				t.Fatalf("累计增量或调用轮数错误: bytes=%d requests=%d", deltaBytes, requests.Load())
			}
			stepsFrame := p.call(t, "step.list", map[string]any{"run_id": runID})
			requireOK(t, stepsFrame)
			var steps []agent.StepSummary
			if err := json.Unmarshal(stepsFrame.Result, &steps); err != nil || len(steps) != 3 {
				t.Fatalf("模型和工具步骤不完整: %v", err)
			}
			step := steps[2]
			if step.Status != final.Run.Status || step.UsageBytes == 0 || step.ProviderRequestID == nil || *step.ProviderRequestID != "limit-request" {
				t.Fatalf("模型步骤终态或元数据丢失: %+v", step)
			}
			for _, field := range []string{"output", "usage"} {
				data := p.call(t, "step.data", map[string]any{"step_id": step.ID, "field": field})
				requireOK(t, data)
				var value struct{ Fragment string }
				if err := json.Unmarshal(data.Result, &value); err != nil {
					t.Fatal(err)
				}
				if (field == "output" && !strings.Contains(value.Fragment, strings.Repeat("b", 64))) || (field == "usage" && !strings.Contains(value.Fragment, `"total_tokens":1`)) {
					t.Fatalf("原始响应或实际 usage 丢失: %s", field)
				}
			}
			p.stop(t)
		})
	}
}

func TestAgentRejectsAudioResponse(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(map[bool]string{false: "generate", true: "stream"}[stream], func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				writeFileToolResponse(t, w, stream, map[string]any{"content": "", "audio": map[string]string{"id": "audio-1", "data": "ZmFrZQ=="}}, "stop")
			}))
			t.Cleanup(upstream.Close)
			cfg, err := config.Load(filepath.Join(t.TempDir(), "data"))
			if err != nil {
				t.Fatal(err)
			}
			cfg.MaxConcurrentRuns = 1
			p := startApp(t, cfg, io.Discard)
			mid := setupConversationModel(t, p, upstream.URL)
			accepted := p.call(t, "conversation.send", map[string]any{"model_id": mid, "stream": stream, "content": messageBlocks("回答")})
			frame := p.event(t, "run.failed")
			var final struct{ Run agent.Run }
			if err := json.Unmarshal(frame.Params, &final); err != nil || final.Run.ErrorKind == nil || *final.Run.ErrorKind != "unsupported" {
				t.Fatalf("音频响应没有明确失败: %s %v", frame.Params, err)
			}
			messages := readMessages(t, p, resultString(t, accepted, "conversation_id"))
			if len(messages) != 2 || messages[1].Status != agent.Failed {
				t.Fatal("不支持的响应被保存为成功回答")
			}
			p.stop(t)
			for _, event := range p.pending {
				if event.Method == "run.completed" {
					t.Fatal("音频响应仍发出成功终态")
				}
			}
		})
	}
}

// setApprovalPolicy 通过公开通信接口修改持久化配置，验证不会依赖启动时注入。
func setApprovalPolicy(t *testing.T, p *appPeer, choice string) {
	t.Helper()
	current := p.call(t, "settings.get", map[string]any{"key": "execution"})
	requireOK(t, current)
	var value struct {
		Revision int64 `json:"revision"`
	}
	if err := json.Unmarshal(current.Result, &value); err != nil {
		t.Fatal(err)
	}
	requireOK(t, p.call(t, "settings.update", map[string]any{"key": "execution", "revision": value.Revision,
		"value": map[string]any{"approval_policy": choice, "approvals_reviewer": "user"}}))
}
