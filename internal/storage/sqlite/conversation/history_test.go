package conversation_test

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/xiaoyu98628/callum/internal/agent"
	"github.com/xiaoyu98628/callum/internal/conversation"
	"github.com/xiaoyu98628/callum/internal/model"
	"github.com/xiaoyu98628/callum/internal/storage/sqlite"
	agentstore "github.com/xiaoyu98628/callum/internal/storage/sqlite/agent"
	store "github.com/xiaoyu98628/callum/internal/storage/sqlite/conversation"
	"github.com/xiaoyu98628/callum/internal/storage/sqlite/schema"
)

func TestWithdrawFollowUpAndAutoSendUseTheSameClaim(t *testing.T) {
	for _, withdrawFirst := range []bool{true, false} {
		name := "send wins"
		if withdrawFirst {
			name = "withdraw wins"
		}
		t.Run(name, func(t *testing.T) {
			ctx := t.Context()
			db, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "data", "callum.db"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := db.Close(); err != nil {
					t.Error(err)
				}
			})
			repo, _ := store.NewRepository(db.DB)
			runs, _ := agentstore.NewRepository(db.DB)
			ex, err := repo.Begin(ctx, input(t))
			if err != nil {
				t.Fatal(err)
			}
			item, err := repo.CreateFollowUp(ctx, conversation.FollowUp{
				ID: id(t), ConversationID: ex.Conversation.ID, ExpectedRunID: ex.Run.ID,
				ModelID: ex.Run.Model.ID, Status: conversation.FollowUpQueued,
				Content: []model.Block{{Type: "text", Text: "原队列正文"}},
			})
			if err != nil {
				t.Fatal(err)
			}
			ex.Run.Status, ex.Run.FinishedAt = agent.Completed, new(time.Now().UnixMilli())
			if err := runs.Finish(ctx, ex.Run); err != nil {
				t.Fatal(err)
			}
			withdraw := conversation.FollowUpChange{ConversationID: item.ConversationID, ID: item.ID, Revision: item.Revision, Action: "withdraw"}
			next := input(t)
			next.Create, next.ConversationID, next.Model = false, item.ConversationID, ex.Run.Model
			next.FollowUp = &conversation.FollowUpStart{ConversationID: item.ConversationID, ID: item.ID, Revision: item.Revision, AfterRunID: ex.Run.ID}
			next.Content = item.Content
			if !withdrawFirst {
				if _, err := repo.Begin(ctx, next); err != nil {
					t.Fatal(err)
				}
				if _, err := repo.ChangeFollowUp(ctx, withdraw, nil); !errors.Is(err, conversation.ErrFollowUpSent) {
					t.Fatalf("已发消息错误地进入草稿: %v", err)
				}
				return
			}
			draft, err := repo.ChangeFollowUp(ctx, withdraw, nil)
			if err != nil || draft.Status != conversation.FollowUpDraft || draft.Content[0].Text != "原队列正文" {
				t.Fatalf("取出的草稿不完整: %+v, %v", draft, err)
			}
			if _, err := repo.Begin(ctx, next); !errors.Is(err, conversation.ErrFollowUpChanged) {
				t.Fatalf("已取出的消息被自动发送: %v", err)
			}
			next.FollowUp.Revision, next.FollowUp.AfterRunID, next.FollowUp.Text = draft.Revision, "", new("手动提交的编辑")
			next.Content = []model.Block{{Type: "text", Text: *next.FollowUp.Text}}
			accepted, err := repo.Begin(ctx, next)
			if err != nil || accepted.User.Content[0].Text != "手动提交的编辑" {
				t.Fatalf("草稿无法手动发送: %+v, %v", accepted.User, err)
			}
		})
	}
}

func TestSteeringHistoryKeepsReceiptPositionAcrossNewSteps(t *testing.T) {
	ctx := t.Context()
	db, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "data", "callum.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	repo, _ := store.NewRepository(db.DB)
	runs, _ := agentstore.NewRepository(db.DB)
	ex, err := repo.Begin(ctx, input(t))
	if err != nil {
		t.Fatal(err)
	}
	add := func(status string) conversation.FollowUp {
		t.Helper()
		followUp, err := repo.CreateFollowUp(ctx, conversation.FollowUp{
			ID: id(t), ConversationID: ex.Conversation.ID, ExpectedRunID: ex.Run.ID,
			ModelID: ex.Run.Model.ID, Status: status, Content: []model.Block{{Type: "text", Text: "接着核对目录"}},
		})
		if err != nil {
			t.Fatal(err)
		}
		return followUp
	}
	check := func(message *conversation.Message, after int64) {
		t.Helper()
		if message == nil || message.SteeringAfterStep == nil || *message.SteeringAfterStep != after {
			t.Fatalf("引导应在步骤 %d 之后，实际为 %+v", after, message)
		}
	}
	history := func() map[string]conversation.Message {
		t.Helper()
		page, err := repo.History(ctx, ex.Conversation.ID, "", 1)
		if err != nil {
			t.Fatal(err)
		}
		result := make(map[string]conversation.Message, len(page.Messages))
		for _, message := range page.Messages {
			result[message.ID] = message
		}
		return result
	}
	addStep := func(kind, name, data string) agent.Step {
		t.Helper()
		now := time.Now().UnixMilli()
		step, err := runs.CreateStep(ctx, agent.Step{ID: id(t), ConversationID: ex.Conversation.ID, RunID: ex.Run.ID,
			Type: kind, Name: name, Status: agent.Pending, Input: json.RawMessage(data), CreatedAt: now, UpdatedAt: now})
		if err != nil {
			t.Fatal(err)
		}
		return step
	}
	save := func(step agent.Step, status string) {
		t.Helper()
		now := time.Now().UnixMilli()
		step.Status, step.UpdatedAt = status, now
		if status != agent.Running {
			step.FinishedAt = &now
		}
		if err := runs.SaveStep(ctx, step); err != nil {
			t.Fatal(err)
		}
	}
	early := add(conversation.FollowUpSteering)
	check(early.Receipt, 0)
	save(ex.Step, agent.Running)
	command := addStep("tool_call", "run_command", `{}`)
	save(command, agent.Running)
	queued := add(conversation.FollowUpQueued)
	guide, err := repo.ChangeFollowUp(ctx, conversation.FollowUpChange{
		ConversationID: ex.Conversation.ID, ID: queued.ID, Revision: queued.Revision,
		ExpectedRunID: ex.Run.ID, Action: "steer",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	check(guide.Receipt, command.Sequence)
	save(ex.Step, agent.Completed) // 隔离中的模型步骤仍持有受理前的快照。
	skipped := addStep("tool_call", "read_file", `{}`)
	save(skipped, agent.Failed)
	pending := history()
	check(new(pending[early.ID]), 0)
	check(new(pending[guide.ID]), command.Sequence)

	request := addStep("model_call", "继续生成回答", `{"steering_ids":["`+early.ID+`","`+guide.ID+`"]}`)
	save(request, agent.Completed)
	updated, err := repo.ApplySteering(ctx, ex.Run.ID, request.ID, []string{early.ID, guide.ID})
	if err != nil || len(updated) != 2 {
		t.Fatalf("应用引导失败: %v, %v", updated, err)
	}
	check(&updated[0], 0)
	check(&updated[1], command.Sequence)
	finished := history()
	check(new(finished[early.ID]), 0)
	check(new(finished[guide.ID]), command.Sequence)
}

func TestHistoryPagesKeepOldRunsAndStandaloneCompactions(t *testing.T) {
	db, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "data", "callum.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo, _ := store.NewRepository(db.DB)
	runs, _ := agentstore.NewRepository(db.DB)
	var conversationID string
	var oldRun string
	for i := range 205 {
		in := input(t)
		if i > 0 {
			in.Create = false
			in.ConversationID = conversationID
		}
		ex, err := repo.Begin(t.Context(), in)
		if err != nil {
			t.Fatal(err)
		}
		conversationID = ex.Conversation.ID
		if i == 0 {
			oldRun = ex.Run.ID
		}
		ex.Run.Status = agent.Completed
		ex.Run.FinishedAt = new(time.Now().UnixMilli())
		if err := runs.Finish(t.Context(), ex.Run); err != nil {
			t.Fatal(err)
		}
	}
	// 密集的独立压缩不应隐藏消息，也不能被挂在更早的消息页末尾。
	for range 30 {
		run := schema.Run{ID: id(t), ConversationID: conversationID, Status: agent.Completed, FinishedAt: new(int64(1))}
		if err := db.DB.Create(&run).Error; err != nil {
			t.Fatal(err)
		}
		step := schema.Step{ID: id(t), RunID: run.ID, ConversationID: conversationID, Sequence: 1, Type: "context_compression", Name: "压缩历史上下文", Status: agent.Completed, Input: []byte(`{"purpose":"context_compaction","trigger":{"type":"manual"}}`), FinishedAt: new(int64(1))}
		if err := db.DB.Create(&step).Error; err != nil {
			t.Fatal(err)
		}
	}
	seen := map[string]bool{}
	cursor := ""
	totalMessages := 0
	for {
		page, err := repo.History(t.Context(), conversationID, cursor, 25)
		if err != nil {
			t.Fatal(err)
		}
		if cursor == "" && len(page.Messages) != 0 {
			t.Fatal("独立压缩分页混入了更早消息")
		}
		ids := map[string]bool{}
		for _, run := range page.Runs {
			if seen[run.ID] || (cursor != "" && run.ID >= cursor) {
				t.Fatal("执行游标未推进")
			}
			seen[run.ID], ids[run.ID] = true, true
		}
		for _, message := range page.Messages {
			if !ids[message.RunID] {
				t.Fatal("消息缺失执行")
			}
		}
		totalMessages += len(page.Messages)
		if !page.HasMore {
			break
		}
		cursor = page.BeforeRunID
	}
	if len(seen) != 235 || totalMessages != 410 || !seen[oldRun] {
		t.Fatal("长历史读取不完整", len(seen), totalMessages)
	}
}
