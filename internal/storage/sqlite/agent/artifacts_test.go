package agent_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/xiaoyu98628/callum/internal/agent"
	"github.com/xiaoyu98628/callum/internal/attachment"
	"github.com/xiaoyu98628/callum/internal/conversation"
	"github.com/xiaoyu98628/callum/internal/model"
	conversationstore "github.com/xiaoyu98628/callum/internal/storage/sqlite/conversation"
	"github.com/xiaoyu98628/callum/internal/tool"
)

func TestRecoverPublishedArtifactsInCurrentBranch(t *testing.T) {
	db, runs, ex, _ := fixture(t)
	ctx := t.Context()
	conversations, err := conversationstore.NewRepository(db.DB)
	if err != nil {
		t.Fatal(err)
	}
	store, err := attachment.Open(ctx, t.TempDir(), func(context.Context, string, string) (bool, error) {
		return true, nil
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	})
	access, err := attachment.OpenAccess(ctx, t.TempDir(), store, conversations.Attachment)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := access.Close(); err != nil {
			t.Error(err)
		}
	})
	if _, err := access.Begin(ctx, ex.Conversation.ID, ex.Run.ID); err != nil {
		t.Fatal(err)
	}
	const original = "a durable report"
	report, err := access.SaveGenerated(ctx, ex.Conversation.ID, ex.Run.ID, "file", "report.txt", []byte(original), true)
	if err != nil {
		t.Fatal(err)
	}
	failed, err := access.SaveGenerated(ctx, ex.Conversation.ID, ex.Run.ID, "file", "failed.txt", []byte("another file"), true)
	if err != nil {
		t.Fatal(err)
	}
	definition := json.RawMessage(`{"source":"builtin","name":"publish_artifact"}`)
	for _, publication := range []struct {
		block  model.Block
		status string
	}{{report, agent.Completed}, {report, agent.Completed}, {failed, agent.Failed}} {
		step, err := runs.CreateStep(ctx, agent.Step{
			ID: newID(t), ConversationID: ex.Conversation.ID, RunID: ex.Run.ID,
			ParentStepID: &ex.Step.ID, ToolCallID: new("call"), Type: "tool_call", Name: "publish_artifact",
			Status: agent.Pending, Definition: definition, Input: json.RawMessage(`{}`), CreatedAt: 1, UpdatedAt: 1,
		})
		if err != nil {
			t.Fatal(err)
		}
		step.Status, step.FinishedAt = publication.status, new(int64(2))
		step.Output, err = json.Marshal(map[string]any{
			"operation": tool.OutcomeRecord{Version: tool.OperationVersion, Outcome: tool.OutcomeNone},
			"result":    map[string]any{"attachment": publication.block, "published": true},
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := runs.SaveStep(ctx, step); err != nil {
			t.Fatal(err)
		}
	}
	if err := runs.SaveMessage(ctx, ex.Assistant.ID, []model.Block{{Type: "text", Text: "partial answer"}}, agent.Running); err != nil {
		t.Fatal(err)
	}
	// 模拟只持久化了工具结果的一次中断，以及服务再次启动。
	for range 2 {
		if err := runs.Recover(ctx); err != nil {
			t.Fatal(err)
		}
		message, err := conversations.Message(ctx, ex.Conversation.ID, ex.Assistant.ID)
		if err != nil || message.Status != agent.Interrupted || len(message.Content) != 2 ||
			message.Content[0].Text != "partial answer" || message.Content[1] != report {
			t.Fatalf("交付引用未恢复或重复恢复: %+v %v", message, err)
		}
	}
	block, err := conversations.Attachment(ctx, ex.Conversation.ID, report.AttachmentRef)
	if err != nil {
		t.Fatal(err)
	}
	data, _, err := store.Read(ctx, block.AttachmentRef)
	if err != nil || string(data) != original {
		t.Fatalf("恢复后的交付物不可读: %q %v", data, err)
	}
	if _, err := conversations.Attachment(ctx, ex.Conversation.ID, failed.AttachmentRef); !errors.Is(err, conversation.ErrNotFound) {
		t.Fatalf("恢复了失败步骤的交付物: %v", err)
	}
	_, err = conversations.Begin(ctx, conversation.Begin{
		ConversationID: ex.Conversation.ID, RunID: newID(t), StepID: newID(t), AssistantMessageID: newID(t),
		ResendMessageID: ex.User.ID, ResendRunID: ex.Run.ID, Model: ex.Run.Model, Protocol: "chatcompletions",
		Content: []model.Block{{Type: "text", Text: "resend"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := runs.Recover(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := conversations.Attachment(ctx, ex.Conversation.ID, report.AttachmentRef); !errors.Is(err, conversation.ErrNotFound) {
		t.Fatalf("恢复了已替换分支的交付物: %v", err)
	}
}
