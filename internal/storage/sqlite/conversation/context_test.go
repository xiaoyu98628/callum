package conversation_test

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/xiaoyu98628/callum/internal/agent"
	"github.com/xiaoyu98628/callum/internal/conversation"
	"github.com/xiaoyu98628/callum/internal/conversation/compaction"
	"github.com/xiaoyu98628/callum/internal/model"
	"github.com/xiaoyu98628/callum/internal/storage/sqlite"
	agentstore "github.com/xiaoyu98628/callum/internal/storage/sqlite/agent"
	store "github.com/xiaoyu98628/callum/internal/storage/sqlite/conversation"
	"github.com/xiaoyu98628/callum/internal/storage/sqlite/schema"
)

func contextFixture(t *testing.T) (*sqlite.Database, *store.Repository, *agentstore.Repository, conversation.Execution) {
	t.Helper()
	db, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "data", "callum.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	repository, err := store.NewRepository(db.DB)
	if err != nil {
		t.Fatal(err)
	}
	runs, err := agentstore.NewRepository(db.DB)
	if err != nil {
		t.Fatal(err)
	}
	in := input(t)
	in.Content[0].Text = strings.Repeat("原始用户约束", 200)
	var last conversation.Execution
	for range 4 {
		last, err = repository.Begin(t.Context(), in)
		if err != nil {
			t.Fatal(err)
		}
		if err := runs.SaveMessage(t.Context(), last.Assistant.ID, []model.Block{{Type: "text", Text: strings.Repeat("保留原始回答", 100)}}, agent.Completed); err != nil {
			t.Fatal(err)
		}
		last.Run.Status, last.Run.UpdatedAt = agent.Completed, time.Now().UnixMilli()
		last.Run.FinishedAt = new(last.Run.UpdatedAt)
		if err := runs.Finish(t.Context(), last.Run); err != nil {
			t.Fatal(err)
		}
		in.Create, in.RunID, in.UserMessageID, in.AssistantMessageID, in.StepID = false, id(t), id(t), id(t), id(t)
	}
	return db, repository, runs, last
}

func buildingContext(t *testing.T, repository *store.Repository, runs *agentstore.Repository, previous conversation.Execution, retain int) (conversation.Execution, conversation.ContextVersion) {
	t.Helper()
	ex, err := repository.BeginCompaction(t.Context(), conversation.Begin{ConversationID: previous.Conversation.ID, RunID: id(t), StepID: id(t), Model: previous.Run.Model})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	ex.Step.Status, ex.Step.StartedAt = agent.Running, &now
	if err := runs.SaveStep(t.Context(), ex.Step); err != nil {
		t.Fatal(err)
	}
	floor := int64(0)
	if ex.Context != nil {
		floor = ex.Context.ThroughSequence
	}
	traces, err := runs.ContextTranscript(t.Context(), ex.Conversation.ID, floor)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := compaction.Build(ex.History, traces, retain)
	if err != nil || len(plan.Sources) == 0 {
		t.Fatalf("无法准备压缩: %v", err)
	}
	value := conversation.ContextVersion{ID: id(t), ConversationID: ex.Conversation.ID, GeneratedByStepID: ex.Step.ID, ThroughSequence: plan.Through, Sources: plan.Sources}
	if ex.Context != nil {
		value.BaseVersionID = &ex.Context.ID
	}
	value, err = repository.CreateContext(t.Context(), value)
	if err != nil {
		t.Fatal(err)
	}
	value.Summary = `{"goals":["目标"],"constraints":["约束"],"decisions":[],"facts":["事实"],"open_questions":[]}`
	value.Stats = json.RawMessage(`{"before_messages":6,"after_messages":1,"before_bytes":1000,"after_bytes":200}`)
	return ex, value
}

func finishContextRun(t *testing.T, runs *agentstore.Repository, ex conversation.Execution) {
	t.Helper()
	ex.Run.Status, ex.Run.UpdatedAt = agent.Completed, time.Now().UnixMilli()
	ex.Run.FinishedAt = new(ex.Run.UpdatedAt)
	if err := runs.Finish(t.Context(), ex.Run); err != nil {
		t.Fatal(err)
	}
}

func TestContextActivationChecksSourceAndRollsBackAsOneTransaction(t *testing.T) {
	db, repository, runs, last := contextFixture(t)
	first, version := buildingContext(t, repository, runs, last, 2)
	if err := repository.ActivateContext(t.Context(), version); err != nil {
		t.Fatal(err)
	}
	finishContextRun(t, runs, first)
	next, replacement := buildingContext(t, repository, runs, last, 1)
	if err := db.DB.Exec(`CREATE TRIGGER fail_context_activation BEFORE UPDATE OF status ON conversation_context_versions WHEN NEW.status='active' BEGIN SELECT RAISE(ABORT,'controlled activation failure'); END`).Error; err != nil {
		t.Fatal(err)
	}
	if err := repository.ActivateContext(t.Context(), replacement); err == nil {
		t.Fatal("激活故障未返回错误")
	}
	active, err := repository.ActiveContext(t.Context(), last.Conversation.ID)
	if err != nil || active == nil || active.ID != version.ID {
		t.Fatalf("激活失败破坏旧版本: %+v %v", active, err)
	}
	if err := db.DB.Exec("DROP TRIGGER fail_context_activation").Error; err != nil {
		t.Fatal(err)
	}
	changed := []model.Block{{Type: "text", Text: "来源已变化"}}
	raw, err := json.Marshal(changed)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.DB.Model(&schema.Message{}).Where("id=?", replacement.Sources[0].MessageID).Update("content", string(raw)).Error; err != nil {
		t.Fatal(err)
	}
	if err := repository.ActivateContext(t.Context(), replacement); !errors.Is(err, conversation.ErrConflict) {
		t.Fatalf("激活未验证来源: %v", err)
	}
	if err := runs.Recover(t.Context()); err != nil {
		t.Fatal(err)
	}
	saved, err := repository.ContextVersion(t.Context(), last.Conversation.ID, replacement.ID)
	if err != nil || saved.Status != "failed" {
		t.Fatal("恢复留下 building 版本")
	}
	run, err := runs.Get(t.Context(), next.Run.ID)
	if err != nil || run.Status != agent.Interrupted {
		t.Fatal("恢复未中断压缩执行")
	}
	active, err = repository.ActiveContext(t.Context(), last.Conversation.ID)
	if err != nil || active.ID != version.ID {
		t.Fatal("恢复改写有效版本")
	}
	if err := db.DB.Model(&schema.ContextVersion{}).Where("id=?", version.ID).Update("summary", `{"goals":[]}`).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := repository.ActiveContext(t.Context(), last.Conversation.ID); !errors.Is(err, model.ErrInvalidResponse) {
		t.Fatal("损坏的有效摘要被继续使用")
	}
}

func TestResendClearsCoveredSummaryAndItsModelResult(t *testing.T) {
	_, repository, runs, last := contextFixture(t)
	ex, err := repository.BeginCompaction(t.Context(), conversation.Begin{ConversationID: last.Conversation.ID, RunID: id(t), StepID: id(t), Model: last.Run.Model})
	if err != nil {
		t.Fatal(err)
	}
	ex.Step.Status = agent.Running
	if err := runs.SaveStep(t.Context(), ex.Step); err != nil {
		t.Fatal(err)
	}
	// 仓储也保护覆盖到最后问题的历史版本；服务当前会保留最近轮次，通常无需失效该前缀。
	var sources []conversation.ContextSource
	traces, err := runs.ContextTranscript(t.Context(), last.Conversation.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range ex.History {
		var trace []model.Message
		if row.Role == "user" {
			trace = traces[row.RunID]
		}
		hash, err := compaction.Fingerprint(row, trace)
		if err != nil {
			t.Fatal(err)
		}
		sources = append(sources, conversation.ContextSource{MessageID: row.ID, RunID: row.RunID, Sequence: row.Sequence, Fingerprint: hash})
	}
	version, err := repository.CreateContext(t.Context(), conversation.ContextVersion{ID: id(t), ConversationID: ex.Conversation.ID, GeneratedByStepID: ex.Step.ID, ThroughSequence: sources[len(sources)-1].Sequence, Sources: sources})
	if err != nil {
		t.Fatal(err)
	}
	version.Summary = `{"goals":["旧问题"],"constraints":[],"decisions":[],"facts":[],"open_questions":[]}`
	version.Stats = json.RawMessage(`{"before_bytes":1000,"after_bytes":200}`)
	now := time.Now().UnixMilli()
	child, err := runs.CreateStep(t.Context(), agent.Step{ID: id(t), ConversationID: ex.Conversation.ID, RunID: ex.Run.ID, ParentStepID: &ex.Step.ID, Type: "model_call", Name: "生成历史摘要", Status: agent.Completed, Input: json.RawMessage(`{"purpose":"context_summary"}`), Output: json.RawMessage(`{"text":"旧问题的摘要副本"}`), Usage: json.RawMessage(`{"total_tokens":7}`), CreatedAt: now, UpdatedAt: now, FinishedAt: &now})
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.ActivateContext(t.Context(), version); err != nil {
		t.Fatal(err)
	}
	finishContextRun(t, runs, ex)
	in := input(t)
	in.Create, in.ConversationID, in.ResendMessageID = false, last.Conversation.ID, last.User.ID
	accepted, err := repository.Begin(t.Context(), in)
	if err != nil || accepted.Context != nil {
		t.Fatalf("重发复用了失效摘要: %v", err)
	}
	saved, err := repository.ContextVersion(t.Context(), last.Conversation.ID, version.ID)
	if err != nil || saved.Status != "invalid" || saved.Summary != "" || saved.InvalidatedAt == nil {
		t.Fatal("未清理受影响的摘要")
	}
	step, err := runs.GetStep(t.Context(), child.ID)
	if err != nil || len(step.Output) != 0 || len(step.Usage) == 0 {
		t.Fatal("摘要模型副本未清理或实际用量丢失")
	}
}
