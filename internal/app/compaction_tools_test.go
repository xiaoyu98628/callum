package app

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xiaoyu98628/callum/internal/agent"
	"github.com/xiaoyu98628/callum/internal/agent/events"
	"github.com/xiaoyu98628/callum/internal/conversation/compaction"
	"github.com/xiaoyu98628/callum/internal/model"
	"github.com/xiaoyu98628/callum/internal/settings"
	"github.com/xiaoyu98628/callum/internal/tool"
)

func TestCompactedUnknownOperationStillRequiresApprovalAfterRestart(t *testing.T) {
	rt, cfg := runtimeFixture(t, 1)
	record, err := rt.Settings().Get(t.Context(), settings.CompactionKey)
	if err != nil {
		t.Fatal(err)
	}
	config := settings.DefaultCompaction()
	config.TokenBudget, config.RetainTurns = 8000, 1
	raw, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rt.Settings().Update(t.Context(), settings.CompactionKey, record.Revision, raw); err != nil {
		t.Fatal(err)
	}
	instance := &outcomeTestTool{path: filepath.Join(t.TempDir(), "evidence"), outcome: tool.OutcomeUnknown}
	client := outcomeTestModel(func(_ context.Context, req model.Request) (model.Response, error) {
		if len(req.Tools) == 0 {
			if req.Messages[0].Content == compaction.Prompt {
				// 即便模型摘要错误地声称成功，也不能覆盖服务端未知操作事实。
				return model.Response{Text: `{"goals":["继续任务"],"constraints":[],"decisions":[],"facts":["操作已经成功"],"open_questions":[]}`}, nil
			}
			return model.Response{Text: "标题"}, nil
		}
		last := req.Messages[len(req.Messages)-1]
		if last.Role == model.RoleTool {
			return model.Response{Text: strings.Repeat("回答正文", 200)}, nil
		}
		text := last.Content
		if len(last.Blocks) > 0 {
			text = last.Blocks[0].Text
		}
		if strings.HasPrefix(text, "执行操作") || strings.HasPrefix(text, "重试操作") {
			return model.Response{ToolCalls: []model.ToolCall{{ID: "fixed-call", Name: "operation", Arguments: json.RawMessage(`{}`)}}}, nil
		}
		return model.Response{Text: strings.Repeat("中间回答", 200)}, nil
	})
	id := policyTestID(t)
	svc, _ := outcomeService(t, rt, instance, client, id, nil)
	sub, err := rt.Subscribe(events.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	first := sendOutcome(t, svc, id, "", "执行操作"+strings.Repeat("必须保留约束", 200), nil)
	if terminal := awaitOutcome(t, rt, sub, first.Run.ID, nil); terminal.Type != "run.completed" {
		t.Fatal("首个执行失败")
	}
	next := sendOutcome(t, svc, id, first.Conversation.ID, strings.Repeat("中间问题", 200), nil)
	if terminal := awaitOutcome(t, rt, sub, next.Run.ID, nil); terminal.Type != "run.completed" {
		t.Fatal("中间执行失败")
	}
	// 历史工具结果及独立操作事实计入 Token 预算，第三轮触发压缩。
	accepted := sendOutcome(t, svc, id, first.Conversation.ID, strings.Repeat("第三个问题", 200), nil)
	if terminal := awaitOutcome(t, rt, sub, accepted.Run.ID, nil); terminal.Type != "run.completed" {
		t.Fatalf("工具轨迹压缩失败: %+v", terminal.Data)
	}
	versions, err := svc.ContextVersions(t.Context(), first.Conversation.ID, 50, 0)
	if err != nil || len(versions) != 1 || versions[0].ThroughSequence != 2 {
		t.Fatalf("未覆盖工具轮次: %+v %v", versions, err)
	}
	if err := rt.Close(); err != nil {
		t.Fatal(err)
	}
	restarted, err := New(t.Context(), cfg, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := restarted.Close(); err != nil {
			t.Error(err)
		}
	}()
	retryService, _ := outcomeService(t, restarted, instance, client, id, nil)
	retrySub, err := restarted.Subscribe(events.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	defer retrySub.Close()
	retry := sendOutcome(t, retryService, id, first.Conversation.ID, "重试操作", nil)
	approvals := 0
	terminal := awaitOutcome(t, restarted, retrySub, retry.Run.ID, func(a agent.Approval) {
		approvals++
		if instance.calls.Load() != 1 {
			t.Fatal("压缩或重启导致未知操作自动重试")
		}
		if _, err := restarted.Approvals().Decide(t.Context(), a.ID, agent.ApprovalRejected, nil); err != nil {
			t.Fatal(err)
		}
	})
	if terminal.Type != "run.completed" || approvals != 1 || instance.calls.Load() != 1 {
		t.Fatal("压缩后的未知操作未得到保护")
	}
}

func TestAutomaticCompactionBetweenToolRoundsPreservesCurrentCalls(t *testing.T) {
	rt, _ := runtimeFixture(t, 1)
	record, err := rt.Settings().Get(t.Context(), settings.CompactionKey)
	if err != nil {
		t.Fatal(err)
	}
	cfg := settings.DefaultCompaction()
	cfg.TokenBudget, cfg.RetainTurns = 7000, 1
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rt.Settings().Update(t.Context(), settings.CompactionKey, record.Revision, raw); err != nil {
		t.Fatal(err)
	}
	instance := &outcomeTestTool{path: filepath.Join(t.TempDir(), "evidence"), outcome: tool.OutcomeApplied}
	checked := false
	client := outcomeTestModel(func(_ context.Context, req model.Request) (model.Response, error) {
		if len(req.Tools) == 0 {
			if req.Messages[0].Content == compaction.Prompt {
				return model.Response{Text: `{"goals":["任务"],"constraints":[],"decisions":[],"facts":["历史事实"],"open_questions":[]}`}, nil
			}
			return model.Response{Text: "标题"}, nil
		}
		last := req.Messages[len(req.Messages)-1]
		if last.Role == model.RoleTool {
			count := 0
			summary := false
			for _, message := range req.Messages {
				if message.Role == model.RoleTool {
					count++
				}
				if strings.Contains(message.Content, "conversation_context_summary") {
					summary = true
				}
			}
			if count != 8 || !summary {
				t.Errorf("当前工具链在压缩后不完整: %d summary=%v", count, summary)
			}
			checked = true
			return model.Response{Text: "工具轮次完成"}, nil
		}
		if len(last.Blocks) > 0 && last.Blocks[0].Text == "开始工具轮次" {
			calls := make([]model.ToolCall, 8)
			for i := range calls {
				calls[i] = model.ToolCall{ID: string(rune('a' + i)), Name: "operation", Arguments: json.RawMessage(`{}`)}
			}
			return model.Response{ToolCalls: calls}, nil
		}
		return model.Response{Text: strings.Repeat("历史回答", 200)}, nil
	})
	id := policyTestID(t)
	svc, _ := outcomeService(t, rt, instance, client, id, nil)
	sub, err := rt.Subscribe(events.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	cid := ""
	for range 2 {
		ex := sendOutcome(t, svc, id, cid, strings.Repeat("历史问题", 200), nil)
		cid = ex.Conversation.ID
		if terminal := awaitOutcome(t, rt, sub, ex.Run.ID, nil); terminal.Type != "run.completed" {
			t.Fatal("历史执行失败")
		}
	}
	ex := sendOutcome(t, svc, id, cid, "开始工具轮次", nil)
	if terminal := awaitOutcome(t, rt, sub, ex.Run.ID, nil); terminal.Type != "run.completed" {
		t.Fatalf("工具轮次失败: %+v", terminal)
	}
	// 终态通知先于调度任务完全退出，等待后再读取模型回调中的验证结果。
	rt.manager.Wait(ex.Run.ID)
	if !checked || instance.calls.Load() != 8 {
		t.Fatal("未验证当前工具链")
	}
}
