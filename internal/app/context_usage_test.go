package app

import (
	"encoding/json"
	"github.com/google/uuid"
	"github.com/xiaoyu98628/callum/internal/agent"
	"github.com/xiaoyu98628/callum/internal/storage/sqlite/schema"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/xiaoyu98628/callum/internal/config"
	"github.com/xiaoyu98628/callum/internal/conversation"
	"github.com/xiaoyu98628/callum/internal/model"
	"github.com/xiaoyu98628/callum/internal/model/tokens"
	"github.com/xiaoyu98628/callum/internal/settings"
)

func readContextUsage(t *testing.T, p *appPeer, mid, cid, draft string) conversation.ContextUsage {
	t.Helper()
	f := p.call(t, "conversation.context_usage", map[string]any{"model_id": mid, "conversation_id": cid, "draft": draft})
	requireOK(t, f)
	var value conversation.ContextUsage
	if err := json.Unmarshal(f.Result, &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func TestContextUsageReadOnlyBudgetDraftAndActualSurviveRestart(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"index":0,"message":{"role":"assistant","content":"受控回答"},"finish_reason":"stop"}],"usage":{"prompt_tokens":201,"total_tokens":208,"custom":"unchanged"}}`)
	}))
	t.Cleanup(server.Close)
	cfg, err := config.Load(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatal(err)
	}
	cfg.MaxConcurrentRuns = 1
	p := startApp(t, cfg, io.Discard)
	mid := setupConversationModel(t, p, server.URL)
	base := readContextUsage(t, p, mid, "", "")
	draft := readContextUsage(t, p, mid, "", strings.Repeat("草稿", 100))
	if base.State != "ready" || base.EstimatedTokens == nil || *base.EstimatedTokens <= 100 || base.InputLimit == nil || *base.InputLimit != 1_000_000-4096-1024 || base.ActualInputTokens != nil {
		t.Fatalf("基础统计无效: %+v", base)
	}
	wantDraft := tokens.Estimate([]model.Message{{Role: model.RoleUser, Content: strings.Repeat("草稿", 100)}}, nil, 0) - 16
	if draft.DraftTokens != wantDraft || *draft.EstimatedTokens != *base.EstimatedTokens+wantDraft || calls.Load() != 0 {
		t.Fatal("草稿被调用模型或统计不同")
	}
	list := p.call(t, "conversation.list", map[string]any{})
	requireOK(t, list)
	if string(list.Result) != "[]" {
		t.Fatalf("只读预览创建了会话: %s", list.Result)
	}
	for _, invalid := range []map[string]any{{"model_id": mid, "unexpected": true}, {"model_id": "invalid"}, {"model_id": mid, "conversation_id": mid, "workspace_id": mid}} {
		if p.call(t, "conversation.context_usage", invalid).Error == nil {
			t.Fatal("接受了非法输入")
		}
	}
	cid := compactSend(t, p, mid, "", "正式问题")
	idle := readContextUsage(t, p, mid, cid, "")
	if idle.EstimatedTokens == nil || *idle.EstimatedTokens <= *base.EstimatedTokens || idle.ActualInputTokens == nil || *idle.ActualInputTokens != 201 || calls.Load() != 1 {
		t.Fatalf("完成后统计无效: %+v calls=%d", idle, calls.Load())
	}
	p.stop(t)
	p = startApp(t, cfg, io.Discard)
	restored := readContextUsage(t, p, mid, cid, "")
	if restored.ActualInputTokens == nil || *restored.ActualInputTokens != 201 || restored.EstimatedTokens == nil || *restored.EstimatedTokens != *idle.EstimatedTokens {
		t.Fatalf("重启统计不一致: %+v", restored)
	}
	provider := resultID(t, p.call(t, "provider.create", map[string]any{"name": "未知容量", "protocol": "chatcompletions", "base_url": server.URL + "/v1"}), "id")
	unknown := resultID(t, p.call(t, "model.create", map[string]any{"provider_id": provider, "display_name": "未知", "remote_model_id": "unknown", "enabled": true}), "id")
	value := readContextUsage(t, p, unknown, "", "")
	if value.InputLimit != nil || value.ContextWindow != nil || value.EstimatedTokens == nil || value.Reason != "capacity_unconfigured" {
		t.Fatalf("伪造了未知容量: %+v", value)
	}
	compression := settings.DefaultCompaction()
	compression.TokenBudget = 50000
	compactionSetting(t, p, compression)
	value = readContextUsage(t, p, unknown, "", "")
	if value.InputLimit == nil || *value.InputLimit != 50000 || value.ContextWindow != nil {
		t.Fatal("未使用显式预算")
	}
}

func TestContextUsageStreamsNotificationsDraftIsolationAndCancellation(t *testing.T) {
	next := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"短\"},\"finish_reason\":null}]}\n\n")
		w.(http.Flusher).Flush()
		select {
		case <-next:
		case <-r.Context().Done():
			return
		}
		chunk, _ := json.Marshal(strings.Repeat("生成中", 300))
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":"+string(chunk)+"},\"finish_reason\":null}]}\n\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	t.Cleanup(server.Close)
	cfg, err := config.Load(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatal(err)
	}
	cfg.MaxConcurrentRuns = 1
	p := startApp(t, cfg, io.Discard)
	mid := setupConversationModel(t, p, server.URL)
	accepted := p.call(t, "conversation.send", map[string]any{"model_id": mid, "stream": true, "content": messageBlocks("流式问题")})
	cid, rid := resultString(t, accepted, "conversation_id"), resultString(t, accepted, "run_id")
	p.event(t, "message.delta")
	first := readContextUsage(t, p, mid, cid, "")
	if first.State != "ready" || first.RunID != rid || first.EstimatedTokens == nil {
		t.Fatalf("没有实时快照: %+v", first)
	}
	compression := settings.DefaultCompaction()
	compression.SafetyMargin = 4096
	compactionSetting(t, p, compression)
	frozen := readContextUsage(t, p, mid, cid, "")
	if frozen.InputLimit == nil || *frozen.InputLimit != *first.InputLimit {
		t.Fatal("活动执行使用了修改后的全局设置")
	}
	if p.call(t, "conversation.context_usage", map[string]any{"conversation_id": cid, "model_id": uuid.Must(uuid.NewV7()).String()}).Error == nil {
		t.Fatal("活动执行允许切换统计模型")
	}
	private := readContextUsage(t, p, mid, cid, "不应持久化的草稿")
	if private.DraftTokens <= 0 || *private.EstimatedTokens <= *first.EstimatedTokens {
		t.Fatal("草稿没有叠加")
	}
	close(next)
	p.event(t, "message.delta")
	second := readContextUsage(t, p, mid, cid, "")
	if second.EstimatedTokens == nil || *second.EstimatedTokens <= *first.EstimatedTokens+1000 || second.Revision <= first.Revision || second.DraftTokens != 0 {
		t.Fatalf("流式快照未更新或草稿污染: %+v", second)
	}
	other := readContextUsage(t, p, mid, "", "")
	if other.RunID != "" || *other.EstimatedTokens >= *second.EstimatedTokens {
		t.Fatal("其他对话混入活动执行")
	}
	event := p.event(t, "conversation.context_usage.updated")
	if strings.Contains(string(event.Params), "不应持久化") || !strings.Contains(string(event.Params), `"context_usage"`) {
		t.Fatalf("通知包含草稿或丢失结构: %s", event.Params)
	}
	requireOK(t, p.call(t, "run.cancel", map[string]any{"run_id": rid}))
	p.event(t, "run.cancelled")
	final := readContextUsage(t, p, mid, cid, "")
	// 结束通知后查询恢复持久化历史，活动缓存不会继续持有取消的执行。
	if final.RunID != "" || final.State != "ready" || final.InputLimit == nil || *final.InputLimit != 1_000_000-4096-4096 {
		t.Fatalf("活动快照未释放: %+v", final)
	}
	if len(readMessages(t, p, cid)) != 2 {
		t.Fatal("预览草稿写入了消息")
	}
}

func TestContextUsageTracksActivatedSummaryWithoutRecountingSources(t *testing.T) {
	_, server := compactionEndpoint(t)
	cfg, err := config.Load(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatal(err)
	}
	cfg.MaxConcurrentRuns = 1
	p := startApp(t, cfg, io.Discard)
	mid := setupConversationModel(t, p, server.URL)
	compression := settings.DefaultCompaction()
	compression.RetainTurns = 1
	compactionSetting(t, p, compression)
	cid := ""
	for range 4 {
		cid = compactSend(t, p, mid, cid, strings.Repeat("完整历史事实", 400))
	}
	before := readContextUsage(t, p, mid, cid, "")
	resultString(t, p.call(t, "conversation.compact", map[string]any{"conversation_id": cid, "model_id": mid}), "run_id")
	p.event(t, "run.completed")
	after := readContextUsage(t, p, mid, cid, "")
	if before.EstimatedTokens == nil || after.EstimatedTokens == nil || *after.EstimatedTokens >= *before.EstimatedTokens/2 || len(readMessages(t, p, cid)) != 8 || len(contextVersions(t, p, cid)) != 1 {
		t.Fatalf("压缩后统计没有切换到摘要: before=%+v after=%+v", before, after)
	}
	p.stop(t)
	p = startApp(t, cfg, io.Discard)
	restored := readContextUsage(t, p, mid, cid, "")
	if restored.EstimatedTokens == nil || *restored.EstimatedTokens != *after.EstimatedTokens {
		t.Fatal("重启后统计摘要来源不一致")
	}
}

func TestContextUsageIncludesHistoryBeyondOnePage(t *testing.T) {
	runtime, _ := runtimeFixture(t, 1)
	mid := directModel(t, runtime, "https://example.invalid")
	empty, err := runtime.Conversations().ContextUsage(t.Context(), conversation.ContextUsageRequest{ModelID: mid})
	if err != nil {
		t.Fatal(err)
	}
	newID := func() string {
		value, err := uuid.NewV7()
		if err != nil {
			t.Fatal(err)
		}
		return value.String()
	}
	cid := newID()
	now := time.Now().UnixMilli()
	snapshot := model.Snapshot{ID: mid, DisplayName: "分页模型", RemoteModelID: "remote"}
	row := schema.Conversation{ID: cid, Title: "有界历史分页", TitleSource: "manual", Model: snapshot, CreatedAt: now, UpdatedAt: now}
	if err := runtime.db.DB.Omit("Workspace").Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	var runs []schema.Run
	var rows []schema.Message
	var messages []model.Message
	for i := range 300 {
		rid := newID()
		runs = append(runs, schema.Run{ID: rid, ConversationID: cid, Model: snapshot, Status: agent.Completed, CreatedAt: now, UpdatedAt: now, FinishedAt: &now})
		for j, role := range []string{"user", "assistant"} {
			text := "保留分页历史，不丢弃较早事实"
			rows = append(rows, schema.Message{ID: newID(), ConversationID: cid, RunID: rid, Sequence: int64(i*2 + j + 1), Role: role, Content: []model.Block{{Type: "text", Text: text}}, Status: agent.Completed, CreatedAt: now, UpdatedAt: now, CompletedAt: &now})
			messages = append(messages, model.Message{Role: model.Role(role), Content: text})
		}
	}
	if err := runtime.db.DB.Omit("Conversation", "Replaces").CreateInBatches(&runs, 100).Error; err != nil {
		t.Fatal(err)
	}
	if err := runtime.db.DB.Omit("Run").CreateInBatches(&rows, 100).Error; err != nil {
		t.Fatal(err)
	}
	value, err := runtime.Conversations().ContextUsage(t.Context(), conversation.ContextUsageRequest{ConversationID: cid, ModelID: mid})
	if err != nil {
		t.Fatal(err)
	}
	want := *empty.EstimatedTokens + tokens.Estimate(messages, nil, 8192) - 16
	if value.EstimatedTokens == nil || *value.EstimatedTokens != want {
		t.Fatalf("超过一页的历史被截断或重复计数: got=%+v want=%d", value, want)
	}
}
