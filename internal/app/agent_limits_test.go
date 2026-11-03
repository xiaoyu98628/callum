package app

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/xiaoyu98628/callum/internal/agent"
	"github.com/xiaoyu98628/callum/internal/config"
)

func setupToolModel(t *testing.T, p *appPeer, url string) string {
	t.Helper()
	mid := setupConversationModel(t, p, url)
	frame := p.call(t, "model.get", map[string]any{"id": mid})
	requireOK(t, frame)
	var value map[string]any
	if err := json.Unmarshal(frame.Result, &value); err != nil {
		t.Fatal(err)
	}
	requireOK(t, p.call(t, "model.update", map[string]any{"id": mid, "provider_id": value["provider_id"], "display_name": "工具模型", "remote_model_id": "remote-model", "enabled": true, "capabilities": map[string]bool{"tools": true}, "model_type": []string{"tools"}}))
	return mid
}
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
	cfg.ApprovalMode = "always"
	p := startApp(t, cfg, io.Discard)
	mid := setupToolModel(t, p, upstream.URL)
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
	cfg.ApprovalMode = "always"
	p := startApp(t, cfg, io.Discard)
	mid := setupToolModel(t, p, upstream.URL)
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
	cfg.ApprovalMode = "always"
	p := startApp(t, cfg, io.Discard)
	mid := setupToolModel(t, p, upstream.URL)
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
