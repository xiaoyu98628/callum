package agent_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/xiaoyu98628/callum/internal/agent"
	"github.com/xiaoyu98628/callum/internal/conversation/compaction"
	"github.com/xiaoyu98628/callum/internal/model"
	"github.com/xiaoyu98628/callum/internal/storage/sqlite/schema"
	"github.com/xiaoyu98628/callum/internal/tool"
)

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
