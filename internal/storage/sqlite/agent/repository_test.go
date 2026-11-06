package agent_test

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"strings"
	"testing"

	"gorm.io/gorm"

	"github.com/xiaoyu98628/callum/internal/agent"
	"github.com/xiaoyu98628/callum/internal/model"
	store "github.com/xiaoyu98628/callum/internal/storage/sqlite/agent"
	"github.com/xiaoyu98628/callum/internal/storage/sqlite/schema"
)

func TestMessageSavePreservesCancellationAndSubsequentWrites(t *testing.T) {
	db, repository, execution, _ := fixture(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	const callback = "controlled_cancel_message_save"
	if err := db.DB.Callback().Update().Before("gorm:update").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Context != ctx {
			return
		}
		cancel()
		// 将隐式事务固定在已回滚状态，复现 database/sql 的取消回滚竞态。
		// 这与调度器先完成自动回滚的结果一致，不依赖随机等待。
		if transaction, ok := tx.Statement.ConnPool.(*sql.Tx); ok {
			if err := transaction.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
				t.Error(err)
			}
		}
		tx.AddError(ctx.Err())
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.DB.Callback().Update().Remove(callback); err != nil {
			t.Error(err)
		}
	})
	var before, after schema.Message
	if err := db.DB.Where("id=?", execution.Assistant.ID).First(&before).Error; err != nil {
		t.Fatal(err)
	}
	err := repository.SaveMessage(ctx, execution.Assistant.ID, []model.Block{{Type: "text", Text: "must not be written"}}, agent.Running)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("取消错误类型被回滚覆盖: %v", err)
	}
	if err := db.DB.Where("id=?", execution.Assistant.ID).First(&after).Error; err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(before.Content, after.Content) || before.Status != after.Status {
		t.Fatal("取消的写入修改了原消息")
	}
	if err := repository.SaveMessage(t.Context(), execution.Assistant.ID, []model.Block{{Type: "text", Text: "subsequent write"}}, agent.Running); err != nil {
		t.Fatalf("取消污染了后续写入: %v", err)
	}
}

func TestMessageSaveKeepsRealDatabaseErrors(t *testing.T) {
	db, repository, execution, _ := fixture(t)
	if err := db.DB.Exec(`CREATE TRIGGER reject_message_save BEFORE UPDATE ON conversation_messages BEGIN SELECT RAISE(ABORT,'controlled message write failure'); END`).Error; err != nil {
		t.Fatal(err)
	}
	err := repository.SaveMessage(t.Context(), execution.Assistant.ID, []model.Block{{Type: "text", Text: "rejected"}}, agent.Running)
	if err == nil || errors.Is(err, context.Canceled) || !strings.Contains(err.Error(), "controlled message write failure") {
		t.Fatalf("真实数据库错误被丢失: %v", err)
	}
	if err := db.DB.Exec("DROP TRIGGER reject_message_save").Error; err != nil {
		t.Fatal(err)
	}
	if err := repository.SaveMessage(t.Context(), execution.Assistant.ID, []model.Block{{Type: "text", Text: "after failed write"}}, agent.Running); err != nil {
		t.Fatal(err)
	}
}

func TestMessageSaveKeepsCallerTransaction(t *testing.T) {
	db, _, execution, _ := fixture(t)
	var before, after schema.Message
	if err := db.DB.Where("id=?", execution.Assistant.ID).First(&before).Error; err != nil {
		t.Fatal(err)
	}
	rollback := errors.New("controlled caller rollback")
	err := db.DB.WithContext(t.Context()).Transaction(func(tx *gorm.DB) error {
		repository, err := store.NewRepository(tx)
		if err != nil {
			return err
		}
		if err := repository.SaveMessage(t.Context(), execution.Assistant.ID, []model.Block{{Type: "text", Text: "inside caller transaction"}}, agent.Running); err != nil {
			return err
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatalf("调用方事务错误丢失: %v", err)
	}
	if err := db.DB.Where("id=?", execution.Assistant.ID).First(&after).Error; err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(before.Content, after.Content) || before.Status != after.Status {
		t.Fatal("消息写入绕过了调用方事务")
	}
}
