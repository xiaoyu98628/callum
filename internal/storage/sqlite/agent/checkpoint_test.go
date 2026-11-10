package agent_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/xiaoyu98628/callum/internal/agent"
	"github.com/xiaoyu98628/callum/internal/conversation"
	"github.com/xiaoyu98628/callum/internal/conversation/compaction"
	"github.com/xiaoyu98628/callum/internal/model"
	store "github.com/xiaoyu98628/callum/internal/storage/sqlite/agent"
	"github.com/xiaoyu98628/callum/internal/storage/sqlite/schema"
	"github.com/xiaoyu98628/callum/internal/tool"
)

func storedCheckpoint(t *testing.T, r *store.Repository, ex conversation.Execution, through int64, fact, status string) (agent.Step, agent.ContextCheckpoint) {
	t.Helper()
	summary, err := compaction.Validate(`{"goals":[],"constraints":[],"decisions":[],"facts":["` + fact + `"],"open_questions":[]}`)
	if err != nil {
		t.Fatal(err)
	}
	id := newID(t)
	value := agent.ContextCheckpoint{Version: 1, ID: id, ConversationID: ex.Conversation.ID, SourceRunID: ex.Run.ID, Through: through, Sources: []agent.BatchSource{{StepID: ex.Step.ID, Through: through, Fingerprint: strings.Repeat("0", 64)}}, Summary: summary, Stats: json.RawMessage(`{}`)}
	output, err := json.Marshal(map[string]any{"checkpoint": value})
	if err != nil {
		t.Fatal(err)
	}
	step, err := r.CreateStep(t.Context(), agent.Step{ID: id, ConversationID: ex.Conversation.ID, RunID: ex.Run.ID, Type: "context_compression", Name: "已保存检查点", Status: status, Input: json.RawMessage(`{}`), Output: output, FinishedAt: new(int64(3))})
	if err != nil {
		t.Fatal(err)
	}
	return step, value
}

func TestCheckpointQuerySelectsLatestPerVisibleRun(t *testing.T) {
	db, r, ex, child := operationFixture(t, true)
	saveOperation(t, r, child, tool.OutcomeApplied, true)
	storedCheckpoint(t, r, ex, child.Sequence, "最新摘要", agent.Completed)
	old, _ := storedCheckpoint(t, r, ex, ex.Step.Sequence, "旧摘要", agent.Completed)
	// 旧版本已被新范围覆盖；查询应在数据库中选定最新版本后才解码。
	if err := db.DB.Model(&schema.Step{}).Where("id=?", old.ID).Update("output", `{"checkpoint":{"version":0}}`).Error; err != nil {
		t.Fatal(err)
	}
	storedCheckpoint(t, r, ex, child.Sequence+100, "失败摘要", agent.Failed)
	otherType, _ := storedCheckpoint(t, r, ex, child.Sequence+100, "其他步骤类型", agent.Completed)
	if err := db.DB.Model(&schema.Step{}).Where("id=?", otherType.ID).Update("type", "model_call").Error; err != nil {
		t.Fatal(err)
	}
	finishOperationRun(t, r, ex, agent.Failed)
	next := beginQueryRun(t, db, ex, false, "")
	next.Step.Status, next.Step.FinishedAt = agent.Completed, new(int64(3))
	next.Step.Output = contextOutput(t, model.Message{Role: model.RoleAssistant, Content: "第二执行回答"}, nil)
	if err := r.SaveStep(t.Context(), next.Step); err != nil {
		t.Fatal(err)
	}
	storedCheckpoint(t, r, next, next.Step.Sequence, "第二执行摘要", agent.Completed)
	finishOperationRun(t, r, next, agent.Completed)
	foreign := beginQueryRun(t, db, ex, true, "")
	foreignStep, shadow := storedCheckpoint(t, r, foreign, child.Sequence+200, "其他会话", agent.Completed)
	shadow.SourceRunID = ex.Run.ID
	output, err := json.Marshal(map[string]any{"checkpoint": shadow})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.DB.Model(&schema.Step{}).Where("id=?", foreignStep.ID).Update("output", string(output)).Error; err != nil {
		t.Fatal(err)
	}
	traces, err := r.ContextTranscript(t.Context(), ex.Conversation.ID, 0)
	if err != nil || len(traces) != 2 || len(traces[ex.Run.ID]) != 1 || len(traces[next.Run.ID]) != 1 {
		t.Fatalf("未按来源执行恢复检查点: %+v %v", traces, err)
	}
	if !strings.Contains(traces[ex.Run.ID][0].Content, "最新摘要") || !strings.Contains(traces[next.Run.ID][0].Content, "第二执行摘要") {
		t.Fatalf("选择了旧版本或其他会话检查点: %+v", traces)
	}
	traces, err = r.ContextTranscript(t.Context(), ex.Conversation.ID, 0, next.User.Sequence)
	if err != nil || len(traces) != 1 || len(traces[ex.Run.ID]) != 1 {
		t.Fatalf("检查点查询越过历史上界: %+v %v", traces, err)
	}
}

func TestCheckpointActivationRejectsChangedSourceAndPreservesPrevious(t *testing.T) {
	db, r, ex, child := operationFixture(t, true)
	saveOperation(t, r, child, tool.OutcomeApplied, true)
	// call-b 没有完成，活动批次不能被摘要覆盖。
	if batch, err := r.ContextBatch(t.Context(), ex.Conversation.ID, ex.Run.ID, 0); err != nil || batch != nil {
		t.Fatalf("未完成批次可被压缩: %+v %v", batch, err)
	}
	second, err := r.CreateStep(t.Context(), agent.Step{ID: newID(t), ConversationID: ex.Conversation.ID, RunID: ex.Run.ID, ParentStepID: &ex.Step.ID, ToolCallID: new("call-b"), Type: "tool_call", Name: "operation", Status: agent.Completed, Input: json.RawMessage(`{}`), Output: contextOutput(t, model.Message{Role: model.RoleTool, ToolCallID: "call-b", Content: `{}`}, nil), FinishedAt: new(int64(3))})
	if err != nil {
		t.Fatal(err)
	}
	batch, err := r.ContextBatch(t.Context(), ex.Conversation.ID, ex.Run.ID, 0)
	if err != nil || batch == nil || len(batch.Messages) != 3 {
		t.Fatalf("完整批次读取失败: %+v %v", batch, err)
	}
	now := time.Now().UnixMilli()
	parent, err := r.CreateStep(t.Context(), agent.Step{ID: newID(t), ConversationID: ex.Conversation.ID, RunID: ex.Run.ID, Type: "context_compression", Name: "压缩过程", Status: agent.Running, Input: json.RawMessage(`{}`), StartedAt: &now, CreatedAt: now, UpdatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	value := agent.ContextCheckpoint{Version: 1, ID: parent.ID, ConversationID: ex.Conversation.ID, SourceRunID: ex.Run.ID, Through: batch.Through, Sources: []agent.BatchSource{batch.BatchSource}, Summary: `{"goals":[],"constraints":[],"decisions":[],"facts":["操作已发生"],"open_questions":[]}`, Stats: json.RawMessage(`{}`)}
	value.Summary, err = compaction.Validate(value.Summary)
	if err != nil {
		t.Fatal(err)
	}
	changed := contextOutput(t, model.Message{Role: model.RoleTool, ToolCallID: "call-b", Content: `{"changed":true}`}, nil)
	if err := db.DB.Model(&schema.Step{}).Where("id=?", second.ID).Update("output", string(changed)).Error; err != nil {
		t.Fatal(err)
	}
	if err := r.SaveCheckpoint(t.Context(), value); !errors.Is(err, agent.ErrConflict) {
		t.Fatalf("来源变化仍激活摘要: %v", err)
	}
	if current, err := r.LatestCheckpoint(t.Context(), ex.Conversation.ID, ex.Run.ID); err != nil || current != nil {
		t.Fatal("失败激活留下半成品")
	}
	batch, err = r.ContextBatch(t.Context(), ex.Conversation.ID, ex.Run.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	value.Sources = []agent.BatchSource{batch.BatchSource}
	if err := r.SaveCheckpoint(t.Context(), value); err != nil {
		t.Fatal(err)
	}
	if err := r.SaveCheckpoint(t.Context(), value); !errors.Is(err, agent.ErrConflict) {
		t.Fatalf("旧父检查点仍能覆盖新状态: %v", err)
	}
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	if err := r.SaveCheckpoint(cancelled, value); err == nil {
		t.Fatal("取消后仍保存")
	}
	current, err := r.LatestCheckpoint(t.Context(), ex.Conversation.ID, ex.Run.ID)
	if err != nil || current == nil || current.ID != value.ID {
		t.Fatal("取消或冲突损坏了此前有效摘要")
	}
}
