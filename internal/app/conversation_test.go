package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/xiaoyu98628/callum/internal/agent"
	"github.com/xiaoyu98628/callum/internal/config"
	"github.com/xiaoyu98628/callum/internal/conversation"
	"github.com/xiaoyu98628/callum/internal/storage/sqlite"
)

func resultString(t *testing.T, f wireFrame, key string) string {
	t.Helper()
	requireOK(t, f)
	var values map[string]json.RawMessage
	if err := json.Unmarshal(f.Result, &values); err != nil {
		t.Fatal(err)
	}
	var value string
	if err := json.Unmarshal(values[key], &value); err != nil || value == "" {
		t.Fatalf("缺少 %s: %s", key, f.Result)
	}
	return value
}

func setupConversationModel(t *testing.T, p *appPeer, url string) string {
	t.Helper()
	pid := resultID(t, p.call(t, "provider.create", map[string]any{"name": "测试厂商", "protocol": "chatcompletions", "base_url": url + "/v1", "api_key": "fake-dialogue-key"}), "id")
	return resultID(t, p.call(t, "model.create", map[string]any{"provider_id": pid, "display_name": "测试模型", "remote_model_id": "remote-model", "enabled": true, "context_window": 1000000}), "id")
}

func messageBlocks(text string) []map[string]string {
	return []map[string]string{{"type": "text", "text": text}}
}

func readMessages(t *testing.T, p *appPeer, id string) []conversation.Message {
	t.Helper()
	var rows []conversation.Message
	before := ""
	for {
		frame := p.call(t, "conversation.history", map[string]any{"conversation_id": id, "before_run_id": before, "limit": 25})
		requireOK(t, frame)
		var page conversation.HistoryPage
		if err := json.Unmarshal(frame.Result, &page); err != nil {
			t.Fatal(err)
		}
		rows = append(page.Messages, rows...)
		// 独立压缩页可能没有消息；只按服务端结束标记停止，并拒绝不推进的游标。
		if !page.HasMore {
			return rows
		}
		if page.BeforeRunID == "" || (before != "" && page.BeforeRunID >= before) {
			t.Fatalf("历史分页游标未推进: %q -> %q", before, page.BeforeRunID)
		}
		before = page.BeforeRunID
	}
}

func TestConversationMessagesMethodIsUnavailable(t *testing.T) {
	t.Parallel()
	cfg, err := config.Load(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatal(err)
	}
	p := startApp(t, cfg, io.Discard)
	frame := p.call(t, "conversation.messages", map[string]any{"conversation_id": "019a4f39-0000-7000-8000-000000000001"})
	if frame.Error == nil || frame.Error.Code != -32601 {
		t.Fatalf("已移除接口未返回 method not found: %+v", frame.Error)
	}
	p.stop(t)
}

func TestConversationLifecycleThroughStdio(t *testing.T) {
	finishFirst := make(chan struct{})
	var once sync.Once
	releaseFirst := func() { once.Do(func() { close(finishFirst) }) }
	t.Cleanup(releaseFirst)
	var mu sync.Mutex
	historyCount := map[string]int{}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer fake-dialogue-key" {
			t.Error("未使用配置快照中的测试凭据")
			w.WriteHeader(401)
			return
		}
		var input struct {
			Tools    []json.RawMessage `json:"tools"`
			Stream   bool              `json:"stream"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil || len(input.Messages) == 0 {
			t.Errorf("请求错误: %v", err)
			w.WriteHeader(400)
			return
		}
		if len(input.Tools) == 0 {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-Request-ID", "title-request")
			_, _ = io.WriteString(w, `{"choices":[{"index":0,"message":{"role":"assistant","content":"模型生成的标题"},"finish_reason":"stop"}],"usage":{"total_tokens":3,"custom":{"title":true}}}`)
			return
		}
		text := input.Messages[len(input.Messages)-1].Content
		mu.Lock()
		// 主对话的固定系统提示不属于持久化历史。
		for _, message := range input.Messages {
			if message.Role != "system" {
				historyCount[text]++
			}
		}
		mu.Unlock()
		w.Header().Set("X-Request-ID", "answer-request")
		if input.Stream {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, "data: {\"id\":\"response\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"流式正文\"},\"finish_reason\":null}]}\n\n")
			w.(http.Flusher).Flush()
			select {
			case <-finishFirst:
			case <-r.Context().Done():
				return
			}
			_, _ = io.WriteString(w, "data: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: {\"choices\":[],\"usage\":{\"total_tokens\":11,\"custom\":{\"answer\":true}}}\n\ndata: [DONE]\n\n")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		encoded, _ := json.Marshal("回答：" + text)
		_, _ = fmt.Fprintf(w, `{"provider_extra":"kept","choices":[{"index":0,"message":{"role":"assistant","content":%s},"finish_reason":"stop"}],"usage":{"total_tokens":5}}`, encoded)
	}))
	t.Cleanup(upstream.Close)
	cfg, err := config.Load(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatal(err)
	}
	cfg.MaxConcurrentRuns = 2
	var logs bytes.Buffer
	p := startApp(t, cfg, &logs)
	modelID := setupConversationModel(t, p, upstream.URL)
	question := "  请帮我 review this project 的结构和代码并说明如何调整  "
	accepted := p.call(t, "conversation.send", map[string]any{"model_id": modelID, "content": messageBlocks(question)})
	convID, runID, userID := resultString(t, accepted, "conversation_id"), resultString(t, accepted, "run_id"), resultString(t, accepted, "user_message_id")
	if id, err := uuid.Parse(runID); err != nil || id.Version() != 7 {
		t.Fatalf("执行不是 UUIDv7: %s", runID)
	}
	created := p.event(t, "conversation.created")
	var data struct {
		Conversation conversation.Conversation `json:"conversation"`
	}
	if err := json.Unmarshal(created.Params, &data); err != nil {
		t.Fatal(err)
	}
	want := string([]rune(strings.TrimSpace(question))[:30])
	if data.Conversation.Title != want || data.Conversation.TitleSource != "default" {
		t.Fatalf("默认标题不正确: %+v", data.Conversation)
	}
	delta := p.event(t, "message.delta")
	if !bytes.Contains(delta.Params, []byte(runID)) || !bytes.Contains(delta.Params, []byte(`"step_id"`)) || bytes.Contains(delta.Params, []byte("模型生成的标题")) {
		t.Fatalf("正文事件归属错误: %s", delta.Params)
	}
	updated := p.event(t, "conversation.updated")
	if !bytes.Contains(updated.Params, []byte(`"title_source":"generated"`)) {
		t.Fatalf("标题未异步更新: %s", updated.Params)
	}
	releaseFirst()
	p.event(t, "run.completed")
	rows := readMessages(t, p, convID)
	if len(rows) != 2 || rows[0].ID != userID || rows[0].Sequence != 1 || rows[1].Sequence != 2 || rows[1].Status != agent.Completed || rows[1].CompletedAt == nil {
		t.Fatalf("消息未正确保存: %+v", rows)
	}
	stepsFrame := p.call(t, "step.list", map[string]any{"run_id": runID})
	requireOK(t, stepsFrame)
	var steps []struct {
		ID         string `json:"id"`
		Name       string `json:"name"`
		Status     string `json:"status"`
		UsageBytes int    `json:"usage_bytes"`
	}
	if err := json.Unmarshal(stepsFrame.Result, &steps); err != nil {
		t.Fatal(err)
	}
	if len(steps) != 2 {
		t.Fatalf("标题与回答没有分别记录: %s", stepsFrame.Result)
	}
	for _, step := range steps {
		if step.Status != agent.Completed || step.UsageBytes == 0 {
			t.Fatalf("调用用量或状态丢失: %+v", step)
		}
		usage := p.call(t, "step.data", map[string]any{"step_id": step.ID, "field": "usage"})
		requireOK(t, usage)
		if !bytes.Contains(usage.Result, []byte("custom")) {
			t.Fatalf("usage 未原样保存: %s", usage.Result)
		}
	}
	second := p.call(t, "conversation.send", map[string]any{"conversation_id": convID, "stream": false, "content": messageBlocks("第二个问题")})
	secondRun := resultString(t, second, "run_id")
	secondUser := resultString(t, second, "user_message_id")
	oldAnswer := resultString(t, second, "message_id")
	p.event(t, "run.completed")
	mu.Lock()
	count := historyCount["第二个问题"]
	mu.Unlock()
	if count != 3 {
		t.Fatalf("未组装前两条历史: %d", count)
	}
	resend := p.call(t, "conversation.resend", map[string]any{"conversation_id": convID, "message_id": secondUser, "stream": false, "content": messageBlocks("修改后的问题")})
	newRun := resultString(t, resend, "run_id")
	if newRun == secondRun || resultString(t, resend, "user_message_id") != secondUser || !bytes.Contains(resend.Result, []byte(oldAnswer)) {
		t.Fatalf("重发未保留用户消息并清理旧回答: %s", resend.Result)
	}
	p.event(t, "run.completed")
	rows = readMessages(t, p, convID)
	if len(rows) != 4 || rows[2].Sequence != 3 || rows[2].ID != secondUser || rows[3].Sequence != 4 || rows[3].ID == oldAnswer || rows[3].Content[0].Text != "回答：修改后的问题" {
		t.Fatalf("重发保留了旧分支: %+v", rows)
	}
	bad := p.call(t, "conversation.resend", map[string]any{"conversation_id": convID, "message_id": userID, "content": messageBlocks("不是最后一条")})
	if bad.Error == nil || bad.Error.Code != 1003 {
		t.Fatalf("允许编辑历史分支: %+v", bad)
	}
	oldSteps := p.call(t, "step.list", map[string]any{"run_id": secondRun})
	requireOK(t, oldSteps)
	var old []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(oldSteps.Result, &old); err != nil || len(old) != 1 {
		t.Fatalf("旧执行记录缺失: %s", oldSteps.Result)
	}
	discarded := p.call(t, "step.data", map[string]any{"step_id": old[0].ID, "field": "output"})
	requireOK(t, discarded)
	if !bytes.Contains(discarded.Result, []byte(`"present":false`)) {
		t.Fatalf("旧回答副本仍保留: %s", discarded.Result)
	}
	manual := p.call(t, "conversation.update", map[string]any{"id": convID, "title": "人工标题"})
	requireOK(t, manual)
	p.stop(t)
	p2 := startApp(t, cfg, &logs)
	restored := p2.call(t, "conversation.get", map[string]any{"id": convID})
	requireOK(t, restored)
	if !bytes.Contains(restored.Result, []byte("人工标题")) || len(readMessages(t, p2, convID)) != 4 {
		t.Fatalf("重启后历史丢失: %s", restored.Result)
	}
	p2.stop(t)
	if bytes.Contains([]byte(p.seen.String()), []byte("fake-dialogue-key")) || bytes.Contains(logs.Bytes(), []byte("fake-dialogue-key")) {
		t.Fatal("凭据泄露到事件或日志")
	}
}

func TestConversationCancelIsolationAndRecovery(t *testing.T) {
	started := make(chan struct{}, 2)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Stream   bool `json:"stream"`
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
			return
		}
		if !req.Stream {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"choices":[{"index":0,"message":{"role":"assistant","content":"完成"},"finish_reason":"stop"}]}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"部分结果\"},\"finish_reason\":null}]}\n\n")
		w.(http.Flusher).Flush()
		started <- struct{}{}
		<-r.Context().Done()
	}))
	t.Cleanup(upstream.Close)
	cfg, err := config.Load(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatal(err)
	}
	cfg.MaxConcurrentRuns = 2
	p := startApp(t, cfg, io.Discard)
	mid := setupConversationModel(t, p, upstream.URL)
	workspace := p.call(t, "workspace.create", map[string]any{"name": "项目", "root_path": t.TempDir()})
	wid := resultString(t, workspace, "id")
	accepted := p.call(t, "conversation.send", map[string]any{"workspace_id": wid, "model_id": mid, "content": messageBlocks("等待取消")})
	id, rid := resultString(t, accepted, "conversation_id"), resultString(t, accepted, "run_id")
	p.event(t, "message.delta")
	<-started
	busy := p.call(t, "conversation.send", map[string]any{"conversation_id": id, "content": messageBlocks("重叠发送")})
	if busy.Error == nil || busy.Error.Code != 1006 {
		t.Fatalf("同会话未互斥: %+v", busy)
	}
	blocked := p.call(t, "workspace.delete", map[string]any{"id": wid})
	if blocked.Error == nil || blocked.Error.Code != 1006 {
		t.Fatalf("运行中删除了工作区: %+v", blocked)
	}
	other := p.call(t, "conversation.send", map[string]any{"model_id": mid, "stream": false, "content": messageBlocks("另一个窗口")})
	otherRun := resultString(t, other, "run_id")
	completed := p.event(t, "run.completed")
	if !bytes.Contains(completed.Params, []byte(otherRun)) {
		t.Fatalf("另一会话受阻: %s", completed.Params)
	}
	requireOK(t, p.call(t, "run.cancel", map[string]any{"run_id": rid}))
	p.event(t, "run.cancelled")
	rows := readMessages(t, p, id)
	if len(rows) != 2 || rows[1].Status != agent.Cancelled || rows[1].CompletedAt != nil || rows[1].Content[0].Text != "部分结果" {
		t.Fatalf("取消丢失部分结果: %+v", rows)
	}
	requireOK(t, p.call(t, "workspace.delete", map[string]any{"id": wid}))
	inactive := p.call(t, "conversation.send", map[string]any{"conversation_id": id, "content": messageBlocks("已删除工作区")})
	if inactive.Error == nil || inactive.Error.Code != 1003 {
		t.Fatalf("在已删除工作区执行: %+v", inactive)
	}
	requireOK(t, p.call(t, "workspace.restore", map[string]any{"id": wid}))
	p.stop(t)
	// 在测试独立库中构造进程退出前的活动状态，验证恢复保留已保存内容。
	db, err := sqlite.Open(t.Context(), cfg.DatabasePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.DB.Exec("UPDATE conversation_runs SET status='running',finished_at=NULL WHERE id=?", rid).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.DB.Exec("UPDATE conversation_messages SET status='running' WHERE run_id=? AND role='assistant'", rid).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	p2 := startApp(t, cfg, io.Discard)
	recovered := p2.call(t, "run.get", map[string]any{"id": rid})
	requireOK(t, recovered)
	if !bytes.Contains(recovered.Result, []byte(`"status":"interrupted"`)) {
		t.Fatalf("未确认中断状态: %s", recovered.Result)
	}
	rows = readMessages(t, p2, id)
	if rows[1].Status != agent.Interrupted || rows[1].Content[0].Text != "部分结果" {
		t.Fatalf("恢复覆盖了部分结果: %+v", rows)
	}
	p2.stop(t)
}
