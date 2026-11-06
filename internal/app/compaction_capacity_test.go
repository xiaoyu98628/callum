package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/xiaoyu98628/callum/internal/agent"
	"github.com/xiaoyu98628/callum/internal/agent/events"
	"github.com/xiaoyu98628/callum/internal/conversation/compaction"
	"github.com/xiaoyu98628/callum/internal/conversation/history"
	"github.com/xiaoyu98628/callum/internal/model"
	"github.com/xiaoyu98628/callum/internal/settings"
	"github.com/xiaoyu98628/callum/internal/storage/sqlite/schema"
	"github.com/xiaoyu98628/callum/internal/tool"
)

const capacitySummary = `{"goals":["完成任务"],"constraints":["保留当前要求"],"decisions":[],"facts":["早期工具批次已完成，真实操作以服务端审计为准"],"open_questions":[]}`

func capacitySettings(t *testing.T, rt *Runtime, enabled bool, budget int64) {
	t.Helper()
	record, err := rt.Settings().Get(t.Context(), settings.CompactionKey)
	if err != nil {
		t.Fatal(err)
	}
	cfg := settings.DefaultCompaction()
	cfg.Enabled, cfg.TokenBudget = enabled, budget
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rt.Settings().Update(t.Context(), settings.CompactionKey, record.Revision, raw); err != nil {
		t.Fatal(err)
	}
}

func capacityTerminal(t *testing.T, rt *Runtime, sub *events.Subscription, id string) agent.Event {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	for {
		event, err := sub.Next(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if event.RunID != id {
			continue
		}
		if event.Type == "approval.requested" {
			t.Fatal("出现非预期审批")
		}
		if event.Type == "run.completed" || event.Type == "run.failed" || event.Type == "run.cancelled" {
			rt.manager.Wait(id)
			return event
		}
	}
}

func requirePairedInput(t *testing.T, req model.Request) {
	t.Helper()
	if err := history.Check(req.Messages); err != nil {
		t.Errorf("超限请求发给模型: %v", err)
	}
	pending := map[string]bool{}
	for _, message := range req.Messages {
		if message.Role == model.RoleTool {
			if !pending[message.ToolCallID] {
				t.Error("工具结果缺少配对调用")
			}
			delete(pending, message.ToolCallID)
		} else {
			if len(pending) > 0 {
				t.Error("调用结果尚未齐全就插入其他消息")
			}
			for _, call := range message.ToolCalls {
				pending[call.ID] = true
			}
		}
	}
	if len(pending) > 0 {
		t.Error("模型请求含未完成工具批次")
	}
}

func TestSingleExecutionCompactsMessageBoundaryAndRestoresAfterRestart(t *testing.T) {
	t.Parallel()
	rt, cfg := runtimeFixture(t, 1)
	capacitySettings(t, rt, true, 900000)
	instance := &outcomeTestTool{path: filepath.Join(t.TempDir(), "evidence"), outcome: tool.OutcomeApplied}
	var rounds, summaries atomic.Int32
	client := outcomeTestModel(func(_ context.Context, req model.Request) (model.Response, error) {
		requirePairedInput(t, req)
		if len(req.Tools) == 0 {
			if req.Messages[0].Content == compaction.Prompt {
				summaries.Add(1)
				return model.Response{Text: capacitySummary, Usage: json.RawMessage(`{"total_tokens":17}`)}, nil
			}
			return model.Response{Text: "标题"}, nil
		}
		found := false
		for _, m := range req.Messages {
			for _, b := range m.Blocks {
				if b.Text == "当前要求：完整完成所有操作" {
					found = true
				}
			}
		}
		if rounds.Load() < 210 {
			if !found {
				t.Error("压缩丢失当前用户要求")
			}
			round := rounds.Add(1)
			return model.Response{ToolCalls: []model.ToolCall{{ID: fmt.Sprintf("a-%d", round), Name: "operation", Arguments: json.RawMessage(`{}`)}, {ID: fmt.Sprintf("b-%d", round), Name: "operation", Arguments: json.RawMessage(`{}`)}}}, nil
		}
		return model.Response{Text: "所有操作完成"}, nil
	})
	id := policyTestID(t)
	svc, runs := outcomeService(t, rt, instance, client, id, nil)
	sub, err := rt.Subscribe(events.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	ex := sendOutcome(t, svc, id, "", "当前要求：完整完成所有操作", nil)
	if terminal := capacityTerminal(t, rt, sub, ex.Run.ID); terminal.Type != "run.completed" {
		t.Fatalf("长执行失败: %+v", terminal.Data)
	}
	checkpoint, err := runs.LatestCheckpoint(t.Context(), ex.Conversation.ID, ex.Run.ID)
	if err != nil || checkpoint == nil || checkpoint.BaseID == "" || summaries.Load() < 2 || instance.calls.Load() != 420 {
		t.Fatalf("未连续压缩或重复执行: checkpoint=%+v summaries=%d calls=%d err=%v", checkpoint, summaries.Load(), instance.calls.Load(), err)
	}
	var sources int64
	if err := rt.db.DB.Model(&schema.Step{}).Where("run_id=? AND type='tool_call' AND json_type(output,'$.context_message')='object'", ex.Run.ID).Count(&sources).Error; err != nil || sources != 420 {
		t.Fatalf("压缩删除了源步骤: %d %v", sources, err)
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
	var checked atomic.Bool
	resumed := outcomeTestModel(func(_ context.Context, req model.Request) (model.Response, error) {
		if len(req.Tools) == 0 {
			return model.Response{Text: "标题"}, nil
		}
		requirePairedInput(t, req)
		for _, m := range req.Messages {
			if strings.Contains(m.Content, "execution_context_summary") {
				checked.Store(true)
			}
		}
		return model.Response{Text: "继续完成"}, nil
	})
	resumedSvc, _ := outcomeService(t, restarted, instance, resumed, id, nil)
	resumedSub, err := restarted.Subscribe(events.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	defer resumedSub.Close()
	next := sendOutcome(t, resumedSvc, id, ex.Conversation.ID, "继续", nil)
	if terminal := capacityTerminal(t, restarted, resumedSub, next.Run.ID); terminal.Type != "run.completed" || !checked.Load() || instance.calls.Load() != 420 {
		t.Fatal("重启未恢复摘要或重放了已完成工具")
	}
}

func TestLegacyOversizedTraceIsCompactedBeforeRestore(t *testing.T) {
	t.Parallel()
	rt, _ := runtimeFixture(t, 1)
	capacitySettings(t, rt, true, 900000)
	instance := &outcomeTestTool{path: filepath.Join(t.TempDir(), "evidence"), outcome: tool.OutcomeApplied}
	var checked atomic.Bool
	client := outcomeTestModel(func(_ context.Context, req model.Request) (model.Response, error) {
		requirePairedInput(t, req)
		if req.Messages[0].Content == compaction.Prompt {
			return model.Response{Text: capacitySummary}, nil
		}
		if len(req.Tools) > 0 {
			for _, m := range req.Messages {
				if strings.Contains(m.Content, "execution_context_summary") {
					checked.Store(true)
				}
			}
		}
		return model.Response{Text: "完成"}, nil
	})
	id := policyTestID(t)
	svc, _ := outcomeService(t, rt, instance, client, id, nil)
	sub, err := rt.Subscribe(events.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	ex := sendOutcome(t, svc, id, "", "历史要求", nil)
	if terminal := capacityTerminal(t, rt, sub, ex.Run.ID); terminal.Type != "run.completed" {
		t.Fatal(terminal.Data)
	}
	// 模拟旧版本留下的 600 条工具轨迹。仅写独立测试数据库，不执行实际工具。
	now := time.Now().UnixMilli()
	var rows []schema.Step
	for i := range 300 {
		parentID, callID := policyTestID(t), fmt.Sprintf("legacy-%d", i)
		parentOutput, _ := json.Marshal(map[string]any{"context_message": model.Message{Role: model.RoleAssistant, ToolCalls: []model.ToolCall{{ID: callID, Name: "operation", Arguments: json.RawMessage(`{}`)}}}})
		childOutput, _ := json.Marshal(map[string]any{"context_message": model.Message{Role: model.RoleTool, ToolCallID: callID, Content: strings.Repeat("历史工具结果", 40)}})
		rows = append(rows, schema.Step{ID: parentID, ConversationID: ex.Conversation.ID, RunID: ex.Run.ID, Sequence: int64(i*2 + 2), Type: "model_call", Name: "历史调用", Status: agent.Completed, Input: json.RawMessage(`{"purpose":"answer"}`), Output: parentOutput, CreatedAt: now, UpdatedAt: now, FinishedAt: &now}, schema.Step{ID: policyTestID(t), ConversationID: ex.Conversation.ID, RunID: ex.Run.ID, Sequence: int64(i*2 + 3), ParentStepID: &parentID, ToolCallID: &callID, Type: "tool_call", Name: "历史结果", Status: agent.Completed, Input: json.RawMessage(`{}`), Output: childOutput, CreatedAt: now, UpdatedAt: now, FinishedAt: &now})
	}
	if err := rt.db.DB.Omit("Run", "Parent").CreateInBatches(rows, 100).Error; err != nil {
		t.Fatal(err)
	}
	next := sendOutcome(t, svc, id, ex.Conversation.ID, "继续历史任务", nil)
	if terminal := capacityTerminal(t, rt, sub, next.Run.ID); terminal.Type != "run.completed" || !checked.Load() || instance.calls.Load() != 0 {
		t.Fatalf("恢复阶段仍被旧轨迹阻塞: %+v", terminal.Data)
	}
}

func TestHistoryBeyondPreparationPageIsConsumedWithoutDroppingTurns(t *testing.T) {
	t.Parallel()
	rt, _ := runtimeFixture(t, 1)
	capacitySettings(t, rt, true, 900000)
	instance := &outcomeTestTool{path: filepath.Join(t.TempDir(), "evidence"), outcome: tool.OutcomeApplied}
	var summaries atomic.Int32
	client := outcomeTestModel(func(_ context.Context, req model.Request) (model.Response, error) {
		requirePairedInput(t, req)
		if req.Messages[0].Content == compaction.Prompt {
			summaries.Add(1)
			return model.Response{Text: capacitySummary}, nil
		}
		return model.Response{Text: "完成"}, nil
	})
	id := policyTestID(t)
	svc, _ := outcomeService(t, rt, instance, client, id, nil)
	sub, err := rt.Subscribe(events.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	ex := sendOutcome(t, svc, id, "", "原始目标", nil)
	if terminal := capacityTerminal(t, rt, sub, ex.Run.ID); terminal.Type != "run.completed" {
		t.Fatal(terminal.Data)
	}
	now := time.Now().UnixMilli()
	// 旧库历史可超过准备窗口。每条仍属于实际来源执行，生成摘要时校验连续来源范围。
	var rows []schema.Message
	var runRows []schema.Run
	for i := range 300 {
		runID := policyTestID(t)
		runRows = append(runRows, schema.Run{ID: runID, ConversationID: ex.Conversation.ID, Model: ex.Run.Model, Status: agent.Completed, CreatedAt: now, UpdatedAt: now, FinishedAt: &now})
		for j, role := range []string{"user", "assistant"} {
			rows = append(rows, schema.Message{ID: policyTestID(t), ConversationID: ex.Conversation.ID, RunID: runID, Sequence: int64(i*2 + j + 3), Role: role, Content: []model.Block{{Type: "text", Text: fmt.Sprintf("原始轮次%d-%d-", i, j) + strings.Repeat("历史事实", 20)}}, Status: agent.Completed, CreatedAt: now, UpdatedAt: now, CompletedAt: &now})
		}
	}
	if err := rt.db.DB.Omit("Conversation", "Replaces").CreateInBatches(runRows, 100).Error; err != nil {
		t.Fatal(err)
	}
	if err := rt.db.DB.Omit("Conversation", "Run").CreateInBatches(rows, 100).Error; err != nil {
		t.Fatal(err)
	}
	next := sendOutcome(t, svc, id, ex.Conversation.ID, "当前要求不可省略", nil)
	if terminal := capacityTerminal(t, rt, sub, next.Run.ID); terminal.Type != "run.completed" || summaries.Load() < 2 {
		t.Fatalf("历史分页压缩失败: %+v summaries=%d", terminal.Data, summaries.Load())
	}
	var count int64
	if err := rt.db.DB.Model(&schema.Message{}).Where("conversation_id=?", ex.Conversation.ID).Count(&count).Error; err != nil || count != 604 {
		t.Fatalf("分页丢失原文: %d %v", count, err)
	}
}

// byteLimitedClient 使用明确的协议字节上限，Token 预算保持充足，验证预检触发与工具执行相互独立。
type byteLimitedClient struct {
	outcomeTestModel
	limit int
}

func (c byteLimitedClient) ValidateInput(_ context.Context, req model.Request) error {
	raw, err := json.Marshal(req)
	if err != nil {
		return err
	}
	if len(raw) > c.limit {
		return &model.InputLimitError{Dimension: "请求字节", Actual: int64(len(raw)), Limit: int64(c.limit)}
	}
	return nil
}

func TestProtocolBytesTriggerCompactionBelowTokenAndMessageLimits(t *testing.T) {
	t.Parallel()
	rt, _ := runtimeFixture(t, 1)
	capacitySettings(t, rt, true, 900000)
	instance := &outcomeTestTool{path: filepath.Join(t.TempDir(), "evidence"), outcome: tool.OutcomeApplied}
	var rounds, summaries atomic.Int32
	var sawCheckpoint atomic.Bool
	client := byteLimitedClient{limit: 16 << 10}
	client.outcomeTestModel = outcomeTestModel(func(ctx context.Context, req model.Request) (model.Response, error) {
		if err := client.ValidateInput(ctx, req); err != nil {
			t.Errorf("超限请求仍发出: %v", err)
		}
		requirePairedInput(t, req)
		if req.Messages[0].Content == compaction.Prompt {
			summaries.Add(1)
			return model.Response{Text: capacitySummary}, nil
		}
		if len(req.Tools) == 0 {
			return model.Response{Text: "标题"}, nil
		}
		for _, m := range req.Messages {
			if strings.Contains(m.Content, "execution_context_summary") {
				sawCheckpoint.Store(true)
			}
		}
		if round := rounds.Add(1); round <= 120 {
			return model.Response{ToolCalls: []model.ToolCall{{ID: fmt.Sprintf("byte-%d", round), Name: "operation", Arguments: json.RawMessage(`{}`)}}}, nil
		}
		return model.Response{Text: "完成"}, nil
	})
	id := policyTestID(t)
	svc, _ := outcomeService(t, rt, instance, client, id, nil)
	sub, err := rt.Subscribe(events.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	ex := sendOutcome(t, svc, id, "", "保留当前要求", nil)
	if terminal := capacityTerminal(t, rt, sub, ex.Run.ID); terminal.Type != "run.completed" || summaries.Load() < 2 || !sawCheckpoint.Load() || instance.calls.Load() != 120 {
		t.Fatalf("字节预检未触发压缩: %+v summaries=%d calls=%d", terminal.Data, summaries.Load(), instance.calls.Load())
	}
}

func TestFailedCheckpointKeepsActiveSummaryAndOriginalOperations(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"malformed", "no_reduction", "cancelled"} {
		t.Run(mode, func(t *testing.T) {
			rt, _ := runtimeFixture(t, 1)
			capacitySettings(t, rt, true, 900000)
			instance := &outcomeTestTool{path: filepath.Join(t.TempDir(), "evidence"), outcome: tool.OutcomeApplied}
			var summaries, rounds atomic.Int32
			client := byteLimitedClient{limit: 16 << 10}
			client.outcomeTestModel = outcomeTestModel(func(_ context.Context, req model.Request) (model.Response, error) {
				if req.Messages[0].Content == compaction.Prompt {
					if summaries.Add(1) == 1 {
						return model.Response{Text: capacitySummary}, nil
					}
					switch mode {
					case "malformed":
						return model.Response{Text: "摘要结构不合法"}, nil
					case "cancelled":
						return model.Response{}, context.Canceled
					default:
						raw, _ := json.Marshal(map[string][]string{"goals": {strings.Repeat("x", 14000), strings.Repeat("y", 14000)}, "constraints": {}, "decisions": {}, "facts": {}, "open_questions": {}})
						return model.Response{Text: string(raw)}, nil
					}
				}
				if len(req.Tools) == 0 {
					return model.Response{Text: "标题"}, nil
				}
				round := rounds.Add(1)
				return model.Response{ToolCalls: []model.ToolCall{{ID: fmt.Sprintf("failure-%d", round), Name: "operation", Arguments: json.RawMessage(`{}`)}}}, nil
			})
			id := policyTestID(t)
			svc, runs := outcomeService(t, rt, instance, client, id, nil)
			sub, err := rt.Subscribe(events.Filter{})
			if err != nil {
				t.Fatal(err)
			}
			defer sub.Close()
			ex := sendOutcome(t, svc, id, "", "继续任务", nil)
			terminal := capacityTerminal(t, rt, sub, ex.Run.ID)
			if terminal.Type == "run.completed" || summaries.Load() != 2 {
				t.Fatalf("失败摘要仍继续: %+v summaries=%d", terminal.Data, summaries.Load())
			}
			checkpoint, err := runs.LatestCheckpoint(t.Context(), ex.Conversation.ID, ex.Run.ID)
			if err != nil || checkpoint == nil || checkpoint.BaseID != "" {
				t.Fatalf("失败替换有效检查点: %+v %v", checkpoint, err)
			}
			var count int64
			if err := rt.db.DB.Model(&schema.Step{}).Where("run_id=? AND type='tool_call' AND json_type(output,'$.context_message')='object'", ex.Run.ID).Count(&count).Error; err != nil || count != int64(instance.calls.Load()) {
				t.Fatal("摘要失败丢失实际工具审计")
			}
		})
	}
}

func TestCancelledCheckpointCannotEraseUnknownOperationOnContinuation(t *testing.T) {
	t.Parallel()
	rt, cfg := runtimeFixture(t, 1)
	capacitySettings(t, rt, true, 900000)
	instance := &outcomeTestTool{path: filepath.Join(t.TempDir(), "evidence"), outcome: tool.OutcomeUnknown}
	var rounds, summaries atomic.Int32
	client := byteLimitedClient{limit: 16 << 10}
	client.outcomeTestModel = outcomeTestModel(func(_ context.Context, req model.Request) (model.Response, error) {
		if req.Messages[0].Content == compaction.Prompt {
			summaries.Add(1)
			return model.Response{Text: `{"goals":["继续任务"],"constraints":[],"decisions":[],"facts":["操作已经成功，无需核实"],"open_questions":[]}`}, nil
		}
		if len(req.Tools) == 0 {
			return model.Response{Text: "标题"}, nil
		}
		round := rounds.Add(1)
		if round > 100 {
			return model.Response{}, context.Canceled
		}
		name := "missing_tool"
		if round == 1 {
			name = "operation"
		}
		return model.Response{ToolCalls: []model.ToolCall{{ID: fmt.Sprintf("uncertain-%d", round), Name: name, Arguments: json.RawMessage(`{}`)}}}, nil
	})
	id := policyTestID(t)
	svc, runs := outcomeService(t, rt, instance, client, id, nil)
	sub, err := rt.Subscribe(events.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	ex := sendOutcome(t, svc, id, "", "执行任务", nil)
	if terminal := capacityTerminal(t, rt, sub, ex.Run.ID); terminal.Type != "run.cancelled" || summaries.Load() == 0 {
		t.Fatalf("未建立被取消执行的有效摘要: %+v", terminal.Data)
	}
	checkpoint, err := runs.LatestCheckpoint(t.Context(), ex.Conversation.ID, ex.Run.ID)
	if err != nil || checkpoint == nil {
		t.Fatal("已提交检查点因取消丢失")
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
	var checked atomic.Bool
	retryClient := outcomeTestModel(func(_ context.Context, req model.Request) (model.Response, error) {
		if len(req.Tools) == 0 {
			return model.Response{Text: "标题"}, nil
		}
		if req.Messages[len(req.Messages)-1].Role == model.RoleTool {
			return model.Response{Text: "已停止重试"}, nil
		}
		hasSummary, hasFact := false, false
		for _, m := range req.Messages {
			hasSummary = hasSummary || strings.Contains(m.Content, "execution_context_summary")
			hasFact = hasFact || strings.Contains(m.Content, "server_operation_facts")
		}
		checked.Store(hasSummary && hasFact)
		return model.Response{ToolCalls: []model.ToolCall{{ID: "retry", Name: "operation", Arguments: json.RawMessage(`{}`)}}}, nil
	})
	retrySvc, _ := outcomeService(t, restarted, instance, retryClient, id, nil)
	retrySub, err := restarted.Subscribe(events.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	defer retrySub.Close()
	next := sendOutcome(t, retrySvc, id, ex.Conversation.ID, "继续核实", nil)
	approvals := 0
	terminal := awaitOutcome(t, restarted, retrySub, next.Run.ID, func(a agent.Approval) {
		approvals++
		if _, err := restarted.Approvals().Decide(t.Context(), a.ID, agent.ApprovalRejected, nil); err != nil {
			t.Fatal(err)
		}
	})
	if terminal.Type != "run.completed" || !checked.Load() || approvals != 1 || instance.calls.Load() != 1 {
		t.Fatal("摘要或重启导致未知操作失去保护或被自动重试")
	}
}
