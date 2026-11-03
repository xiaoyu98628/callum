package conversation_test

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/xiaoyu98628/callum/internal/conversation"
	"github.com/xiaoyu98628/callum/internal/model"
	"github.com/xiaoyu98628/callum/internal/run"
	"github.com/xiaoyu98628/callum/internal/storage/sqlite"
	store "github.com/xiaoyu98628/callum/internal/storage/sqlite/conversation"
	runstore "github.com/xiaoyu98628/callum/internal/storage/sqlite/run"
	"github.com/xiaoyu98628/callum/internal/storage/sqlite/schema"
)

func id(t *testing.T) string {
	t.Helper()
	value, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	return value.String()
}
func input(t *testing.T) conversation.Begin {
	t.Helper()
	return conversation.Begin{Create: true, ConversationID: id(t), UserMessageID: id(t), AssistantMessageID: id(t), RunID: id(t), StepID: id(t), Protocol: "chatcompletions", Model: model.Snapshot{ID: id(t), DisplayName: "模型", RemoteModelID: "remote", Provider: model.ProviderSnapshot{ID: id(t), Name: "厂商"}}, Content: []model.Block{{Type: "text", Text: "原问题"}}, Stream: true}
}

func TestConversationConstraintsAndResendRollback(t *testing.T) {
	db, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "data", "callum.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	}()
	repository, err := store.NewRepository(db.DB)
	if err != nil {
		t.Fatal(err)
	}
	runs, err := runstore.NewRepository(db.DB)
	if err != nil {
		t.Fatal(err)
	}
	firstIn, secondIn := input(t), input(t)
	first, err := repository.Begin(t.Context(), firstIn)
	if err != nil {
		t.Fatal(err)
	}
	second, err := repository.Begin(t.Context(), secondIn)
	if err != nil {
		t.Fatal(err)
	}
	finishedAt := time.Now().UnixMilli()
	foreign := schema.Run{ID: id(t), ConversationID: second.Conversation.ID, Model: second.Run.Model, Status: run.Completed, CreatedAt: finishedAt, UpdatedAt: finishedAt, FinishedAt: &finishedAt}
	if err := db.DB.Omit("Conversation", "Replaces").Create(&foreign).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.DB.Model(&schema.Message{}).Where("id=?", first.Assistant.ID).Update("run_id", foreign.ID).Error; err == nil {
		t.Fatal("允许消息引用其他会话执行")
	}
	if err := db.DB.Model(&schema.Message{}).Where("id=?", first.Assistant.ID).Update("sequence", first.User.Sequence).Error; err == nil {
		t.Fatal("允许同会话消息序号重复")
	}
	duplicate := schema.Run{ID: id(t), ConversationID: first.Conversation.ID, Model: first.Run.Model, Status: run.Running, CreatedAt: 1, UpdatedAt: 1}
	if err := db.DB.Omit("Conversation", "Replaces").Create(&duplicate).Error; err == nil {
		t.Fatal("允许同会话两个活动执行")
	}
	if err := db.DB.Model(&schema.Step{}).Where("id=?", first.Step.ID).Update("usage", `"not-an-object"`).Error; err == nil {
		t.Fatal("usage 未限制为 JSON 对象")
	}
	child := schema.Step{ID: id(t), ConversationID: first.Conversation.ID, RunID: first.Run.ID, ParentStepID: &second.Step.ID, Sequence: 2, Type: "model_call", Name: "错误父级", Status: run.Pending, Input: json.RawMessage(`{}`), CreatedAt: 1, UpdatedAt: 1}
	if err := db.DB.Omit("Run", "Parent").Create(&child).Error; err == nil {
		t.Fatal("允许父步骤跨执行")
	}
	now := time.Now().UnixMilli()
	step := first.Step
	step.Status, step.FinishedAt, step.UpdatedAt = run.Completed, &now, now
	step.Output = json.RawMessage(`{"response":{"text":"旧回答"}}`)
	step.Usage = json.RawMessage(`{"total_tokens":2}`)
	if err := runs.SaveStep(t.Context(), step); err != nil {
		t.Fatal(err)
	}
	if err := runs.SaveMessage(t.Context(), first.Assistant.ID, []model.Block{{Type: "text", Text: "旧回答"}}, run.Completed); err != nil {
		t.Fatal(err)
	}
	finished := first.Run
	finished.Status, finished.FinishedAt, finished.UpdatedAt = run.Completed, &now, now
	if err := runs.Finish(t.Context(), finished); err != nil {
		t.Fatal(err)
	}
	resend := input(t)
	resend.Create = false
	resend.ConversationID = first.Conversation.ID
	resend.ResendMessageID = first.User.ID
	resend.Content = []model.Block{{Type: "text", Text: "修改后"}}
	resend.StepID = first.Step.ID
	if _, err := repository.Begin(t.Context(), resend); err == nil {
		t.Fatal("步骤主键冲突没有使重发失败")
	}
	rows, err := repository.Messages(t.Context(), first.Conversation.ID, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].RunID != first.Run.ID || rows[0].Content[0].Text != "原问题" || rows[1].ID != first.Assistant.ID || rows[1].Content[0].Text != "旧回答" {
		t.Fatalf("失败重发没有完整回滚: %+v", rows)
	}
	kept, err := runs.GetStep(t.Context(), first.Step.ID)
	if err != nil {
		t.Fatal(err)
	}
	if string(kept.Output) != string(step.Output) {
		t.Fatalf("回滚没有恢复旧模型结果: %s", kept.Output)
	}
	if _, err := runs.Get(t.Context(), resend.RunID); !errors.Is(err, run.ErrNotFound) {
		t.Fatalf("遗留部分新执行: %v", err)
	}
}
