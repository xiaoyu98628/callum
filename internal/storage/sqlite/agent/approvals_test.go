package agent_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/xiaoyu98628/callum/internal/agent"
	"github.com/xiaoyu98628/callum/internal/conversation"
	"github.com/xiaoyu98628/callum/internal/model"
	"github.com/xiaoyu98628/callum/internal/storage/sqlite"
	store "github.com/xiaoyu98628/callum/internal/storage/sqlite/agent"
	conversationstore "github.com/xiaoyu98628/callum/internal/storage/sqlite/conversation"
	"github.com/xiaoyu98628/callum/internal/storage/sqlite/schema"
)

func newID(t *testing.T) string {
	t.Helper()
	value, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	return value.String()
}
func fixture(t *testing.T) (*sqlite.Database, *store.Repository, conversation.Execution, agent.Step) {
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
	conversations, err := conversationstore.NewRepository(db.DB)
	if err != nil {
		t.Fatal(err)
	}
	repository, err := store.NewRepository(db.DB)
	if err != nil {
		t.Fatal(err)
	}
	in := conversation.Begin{Create: true, ConversationID: newID(t), RunID: newID(t), UserMessageID: newID(t), AssistantMessageID: newID(t), StepID: newID(t), Protocol: "chatcompletions", Model: model.Snapshot{ID: newID(t), RemoteModelID: "m", Provider: model.ProviderSnapshot{ID: newID(t), Name: "p"}}, Content: []model.Block{{Type: "text", Text: "question"}}}
	execution, err := conversations.Begin(t.Context(), in)
	if err != nil {
		t.Fatal(err)
	}
	step, err := repository.CreateStep(t.Context(), agent.Step{ID: newID(t), RunID: in.RunID, ConversationID: in.ConversationID, ParentStepID: new(in.StepID), Type: "tool_call", Name: "read_file", Status: agent.Pending, Input: json.RawMessage(`{}`), CreatedAt: 1, UpdatedAt: 1})
	if err != nil {
		t.Fatal(err)
	}
	return db, repository, execution, step
}
func newApproval(t *testing.T, repository *store.Repository, step agent.Step) agent.Approval {
	t.Helper()
	request := json.RawMessage(`{"tool":{"name":"read_file"},"arguments":{"path":"a"}}`)
	sum := sha256.Sum256(request)
	value, err := repository.CreateApproval(t.Context(), agent.Approval{ID: newID(t), RunID: step.RunID, StepID: step.ID, Request: request, RequestHash: hex.EncodeToString(sum[:]), Status: agent.ApprovalPending, CreatedAt: 1, UpdatedAt: 1})
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func TestApprovalBindingAndAtomicClaim(t *testing.T) {
	db, repository, execution, step := fixture(t)
	value := newApproval(t, repository, step)
	row := schema.Approval{ID: newID(t), RunID: execution.Run.ID, StepID: execution.Step.ID, Request: value.Request, RequestHash: value.RequestHash, Status: agent.ApprovalPending, CreatedAt: 1, UpdatedAt: 1}
	// 同一审批步骤的唯一性由数据库保证。
	row.StepID = step.ID
	if err := db.DB.Omit("Run", "Step").Create(&row).Error; err == nil {
		t.Fatal("允许一个步骤多条审批")
	}
	// 已有步骤无法被另一个执行引用。
	foreign := schema.Run{ID: newID(t), ConversationID: execution.Conversation.ID, Model: execution.Run.Model, Status: agent.Completed, CreatedAt: 1, UpdatedAt: 1, FinishedAt: new(int64(1))}
	if err := db.DB.Omit("Conversation", "Replaces").Create(&foreign).Error; err != nil {
		t.Fatal(err)
	}
	row.RunID, row.StepID = foreign.ID, execution.Step.ID
	if err := db.DB.Omit("Run", "Step").Create(&row).Error; err == nil {
		t.Fatal("审批允许跨执行关联步骤")
	}
	if _, err := repository.DecideApproval(t.Context(), value.ID, agent.ApprovalApproved, new("user"), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.StartTool(t.Context(), step.RunID, step.ID, value.ID, "changed"); !errors.Is(err, agent.ErrConflict) {
		t.Fatalf("未核对批准参数: %v", err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for range 2 {
		wg.Go(func() {
			_, err := repository.StartTool(t.Context(), step.RunID, step.ID, value.ID, value.RequestHash)
			results <- err
		})
	}
	wg.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("工具被认领 %d 次", successes)
	}
}

func TestApprovalCancelDecisionRaceAndRecovery(t *testing.T) {
	for _, action := range []string{"finish", "recover"} {
		t.Run(action, func(t *testing.T) {
			_, repository, execution, step := fixture(t)
			value := newApproval(t, repository, step)
			var wg sync.WaitGroup
			errs := make(chan error, 2)
			wg.Go(func() {
				_, err := repository.DecideApproval(t.Context(), value.ID, agent.ApprovalApproved, new("user"), nil)
				if errors.Is(err, agent.ErrConflict) {
					err = nil
				}
				errs <- err
			})
			wg.Go(func() {
				if action == "recover" {
					errs <- repository.Recover(t.Context())
				} else {
					now := time.Now().UnixMilli()
					run := execution.Run
					run.Status, run.UpdatedAt, run.FinishedAt = agent.Cancelled, now, &now
					errs <- repository.Finish(t.Context(), run)
				}
			})
			wg.Wait()
			close(errs)
			for err := range errs {
				if err != nil {
					t.Fatal(err)
				}
			}
			saved, err := repository.GetApproval(t.Context(), value.ID)
			if err != nil {
				t.Fatal(err)
			}
			if saved.Status == agent.ApprovalPending || saved.DecidedAt == nil {
				t.Fatalf("终止后仍有待办: %+v", saved)
			}
			if _, err := repository.StartTool(t.Context(), step.RunID, step.ID, value.ID, value.RequestHash); err == nil {
				t.Fatal("终止后启动了工具")
			}
			run, err := repository.Get(t.Context(), execution.Run.ID)
			if err != nil {
				t.Fatal(err)
			}
			want := agent.Cancelled
			if action == "recover" {
				want = agent.Interrupted
			}
			if run.Status != want {
				t.Fatal(run.Status)
			}
		})
	}
}

func TestApprovalRejectsRemovedExpirationState(t *testing.T) {
	db, _, execution, step := fixture(t)
	row := schema.Approval{ID: newID(t), RunID: execution.Run.ID, StepID: step.ID, Request: json.RawMessage(`{}`), RequestHash: "0000000000000000000000000000000000000000000000000000000000000000", Status: "expired", CreatedAt: 1, UpdatedAt: 1, DecidedAt: new(int64(1))}
	if err := db.DB.Omit("Run", "Step").Create(&row).Error; err == nil {
		t.Fatal("审批表仍允许已删除的过期状态")
	}
}
