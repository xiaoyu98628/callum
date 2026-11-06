package agent_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/xiaoyu98628/callum/internal/agent"
	"github.com/xiaoyu98628/callum/internal/conversation"
	"github.com/xiaoyu98628/callum/internal/model"
	"github.com/xiaoyu98628/callum/internal/storage/sqlite"
	store "github.com/xiaoyu98628/callum/internal/storage/sqlite/agent"
	conversationstore "github.com/xiaoyu98628/callum/internal/storage/sqlite/conversation"
	"github.com/xiaoyu98628/callum/internal/storage/sqlite/schema"
	"github.com/xiaoyu98628/callum/internal/tool"
)

func beginQueryRun(t *testing.T, db *sqlite.Database, previous conversation.Execution, create bool, resend string) conversation.Execution {
	t.Helper()
	conversations, err := conversationstore.NewRepository(db.DB)
	if err != nil {
		t.Fatal(err)
	}
	id := previous.Conversation.ID
	if create {
		id = newID(t)
	}
	ex, err := conversations.Begin(t.Context(), conversation.Begin{Create: create, ConversationID: id, RunID: newID(t), UserMessageID: newID(t), AssistantMessageID: newID(t), StepID: newID(t), ResendMessageID: resend, Model: previous.Run.Model, Protocol: "chatcompletions", Content: []model.Block{{Type: "text", Text: "查询隔离"}}})
	if err != nil {
		t.Fatal(err)
	}
	return ex
}

func factStep(t *testing.T, ex conversation.Execution, hash string, outcome tool.Outcome, failed bool) agent.Step {
	t.Helper()
	fields := map[string]any{"operation": tool.OutcomeRecord{Version: tool.OperationVersion, RequestHash: hash, Outcome: outcome}}
	if failed {
		fields["error"] = map[string]string{"kind": "tool_failed"}
	}
	input, err := json.Marshal(map[string]any{"operation_version": tool.OperationVersion, "request_hash": hash})
	if err != nil {
		t.Fatal(err)
	}
	output, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	return agent.Step{ID: newID(t), ConversationID: ex.Conversation.ID, RunID: ex.Run.ID, ParentStepID: &ex.Step.ID, Type: "tool_call", Name: "operation", Status: agent.Completed, Input: input, Output: output, ToolCallID: new(newID(t)), CreatedAt: now, UpdatedAt: now, FinishedAt: &now}
}

func appendFact(t *testing.T, r *store.Repository, ex conversation.Execution, hash string, outcome tool.Outcome, failed bool) agent.Step {
	t.Helper()
	step, err := r.CreateStep(t.Context(), factStep(t, ex, hash, outcome, failed))
	if err != nil {
		t.Fatal(err)
	}
	return step
}

func TestOperationFactsUsesLatestResultAndCurrentBranch(t *testing.T) {
	db, r, ex, _ := fixture(t)
	appendFact(t, r, ex, "cleared", tool.OutcomeUnknown, false)
	appendFact(t, r, ex, "cleared", tool.OutcomeApplied, false)
	partial := appendFact(t, r, ex, "partial", tool.OutcomePartial, false)
	appendFact(t, r, ex, "partial", tool.OutcomeNotApplied, false)
	failed := appendFact(t, r, ex, "failed", tool.OutcomeApplied, true)
	appendFact(t, r, ex, "", tool.OutcomeUnknown, false)
	finishOperationRun(t, r, ex, agent.Completed)
	next := beginQueryRun(t, db, ex, false, "")
	latest := appendFact(t, r, next, "cleared", tool.OutcomeUnknown, false)
	foreign := beginQueryRun(t, db, ex, true, "")
	appendFact(t, r, foreign, "cleared", tool.OutcomeApplied, false)
	appendFact(t, r, foreign, "other_conversation", tool.OutcomeUnknown, false)
	facts, err := r.OperationFacts(t.Context(), ex.Conversation.ID)
	if err != nil || len(facts) != 3 {
		t.Fatalf("操作去重或会话隔离错误: %+v %v", facts, err)
	}
	want := map[string]agent.Step{"cleared": latest, "partial": partial, "failed": failed}
	for i, fact := range facts {
		step, ok := want[fact.RequestHash]
		if !ok || fact.StepID != step.ID || fact.RunID != step.RunID || (i > 0 && facts[i-1].StepID >= fact.StepID) {
			t.Fatalf("没有按当前分支的最新结果返回: %+v", facts)
		}
	}
	finishOperationRun(t, r, next, agent.Completed)
	beginQueryRun(t, db, next, false, next.User.ID)
	facts, err = r.OperationFacts(t.Context(), ex.Conversation.ID)
	if err != nil || len(facts) != 2 {
		t.Fatalf("重发后仍恢复旧分支事实: %+v %v", facts, err)
	}
	for _, fact := range facts {
		if fact.RequestHash == "cleared" {
			t.Fatalf("旧成功之前的未知结果重新进入上下文: %+v", facts)
		}
	}
}

func TestOperationFactsScopesCorruptionAndCancellation(t *testing.T) {
	db, r, ex, _ := fixture(t)
	local := appendFact(t, r, ex, "local", tool.OutcomeUnknown, false)
	foreign := beginQueryRun(t, db, ex, true, "")
	broken := appendFact(t, r, foreign, "foreign", tool.OutcomeUnknown, false)
	invalid := `{"operation":{"version":1,"outcome":"unknown","request_hash":"mismatch"}}`
	if err := db.DB.Model(&schema.Step{}).Where("id=?", broken.ID).Update("output", invalid).Error; err != nil {
		t.Fatal(err)
	}
	if facts, err := r.OperationFacts(t.Context(), ex.Conversation.ID); err != nil || len(facts) != 1 || facts[0].StepID != local.ID {
		t.Fatalf("其他会话的损坏记录影响当前查询: %+v %v", facts, err)
	}
	if err := db.DB.Model(&schema.Step{}).Where("id=?", local.ID).Update("output", invalid).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := r.OperationFacts(t.Context(), ex.Conversation.ID); !errors.Is(err, model.ErrInvalidResponse) {
		t.Fatalf("损坏操作事实被接受: %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := r.OperationFacts(ctx, ex.Conversation.ID); !errors.Is(err, context.Canceled) {
		t.Fatalf("查询没有保留调用方取消: %v", err)
	}
}

func TestOperationFactsEnforcesBoundAfterDeduplication(t *testing.T) {
	db, r, ex, _ := fixture(t)
	rows := make([]schema.Step, 4096)
	for i := range rows {
		step := factStep(t, ex, fmt.Sprintf("operation-%d", i), tool.OutcomeUnknown, false)
		rows[i] = schema.Step{ID: step.ID, ConversationID: step.ConversationID, RunID: step.RunID, ParentStepID: step.ParentStepID, Sequence: int64(i + 3), Type: step.Type, Name: step.Name, Status: step.Status, Input: step.Input, Output: step.Output, ToolCallID: step.ToolCallID, FinishedAt: step.FinishedAt}
	}
	// 批量建立真实存储边界，避免为准备数据反复计算执行内序号。
	if err := db.DB.Omit("Run", "Parent").CreateInBatches(&rows, 128).Error; err != nil {
		t.Fatal(err)
	}
	// 原始步骤超过上限时，重复哈希仍只计入最新的一项事实。
	appendFact(t, r, ex, "operation-0", tool.OutcomeUnknown, false)
	if facts, err := r.OperationFacts(t.Context(), ex.Conversation.ID); err != nil || len(facts) != len(rows) {
		t.Fatalf("合法边界未完整返回: count=%d %v", len(facts), err)
	}
	appendFact(t, r, ex, "overflow", tool.OutcomeUnknown, false)
	if _, err := r.OperationFacts(t.Context(), ex.Conversation.ID); !errors.Is(err, model.ErrInputLimit) {
		t.Fatalf("超过操作事实边界仍返回成功: %v", err)
	}
}

func TestFinishOnlyFailsBuildingContextsFromItsRun(t *testing.T) {
	db, r, ex, _ := fixture(t)
	foreign := beginQueryRun(t, db, ex, true, "")
	var ids []string
	for _, execution := range []conversation.Execution{ex, foreign} {
		step, err := r.CreateStep(t.Context(), agent.Step{ID: newID(t), ConversationID: execution.Conversation.ID, RunID: execution.Run.ID, Type: "context_compression", Name: "生成摘要", Status: agent.Running, Input: json.RawMessage(`{}`), StartedAt: new(int64(1))})
		if err != nil {
			t.Fatal(err)
		}
		row := schema.ContextVersion{ID: newID(t), ConversationID: execution.Conversation.ID, Version: 1, GeneratedByStepID: step.ID, ThroughSequence: execution.User.Sequence, Sources: []conversation.ContextSource{{MessageID: execution.User.ID, RunID: execution.Run.ID, Sequence: execution.User.Sequence, Fingerprint: strings.Repeat("0", 64)}}, Status: "building"}
		if err := db.DB.Omit("Conversation", "GeneratedByStep", "Base").Create(&row).Error; err != nil {
			t.Fatal(err)
		}
		ids = append(ids, row.ID)
	}
	finishOperationRun(t, r, ex, agent.Cancelled)
	for i, id := range ids {
		var row schema.ContextVersion
		if err := db.DB.Where("id=?", id).Take(&row).Error; err != nil {
			t.Fatal(err)
		}
		want := "failed"
		if i == 1 {
			want = "building"
		}
		if row.Status != want {
			t.Fatalf("终态清理越过执行边界: status=%s want=%s", row.Status, want)
		}
	}
}
