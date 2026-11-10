package conversation_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/xiaoyu98628/callum/internal/agent"
	"github.com/xiaoyu98628/callum/internal/storage/sqlite"
	agentstore "github.com/xiaoyu98628/callum/internal/storage/sqlite/agent"
	store "github.com/xiaoyu98628/callum/internal/storage/sqlite/conversation"
	"github.com/xiaoyu98628/callum/internal/storage/sqlite/schema"
)

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
