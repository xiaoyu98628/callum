package service

import (
	"context"
	"log/slog"
	"time"

	"github.com/xiaoyu98628/callum/internal/conversation"
	"github.com/xiaoyu98628/callum/internal/run"
	"github.com/xiaoyu98628/callum/internal/run/executor"
)

// Engine 编排已提交的普通对话执行，不持有数据库事务或 HTTP 资源所有权。
type Engine struct {
	repository    run.Repository
	conversations conversation.Repository
	manager       *executor.Manager
	emit          func(run.Event) error
	logger        *slog.Logger
}

func New(repository run.Repository, conversations conversation.Repository, manager *executor.Manager, emit func(run.Event) error, logger *slog.Logger) (*Engine, error) {
	if repository == nil || conversations == nil || manager == nil || emit == nil || logger == nil {
		return nil, run.ErrInvalid
	}
	return &Engine{repository: repository, conversations: conversations, manager: manager, emit: emit, logger: logger}, nil
}

func (e *Engine) event(ex conversation.Execution, name, stepID, messageID string, data any) error {
	event := run.Event{Type: name, ConversationID: ex.Conversation.ID, RunID: ex.Run.ID, StepID: stepID, MessageID: messageID, Data: data}
	if name == "run.started" {
		event.RemovedMessageIDs = ex.RemovedMessageIDs
	}
	return e.emit(event)
}

// cleanupContext 仅用于保存已发生操作的终态，避免取消导致清理写入被立即拒绝。
// 保留调用上下文的值，并以短超时限制关闭等待。
func cleanupContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
}

func (e *Engine) Abandon(ctx context.Context, ex conversation.Execution, cause error) {
	cleanup, cancel := cleanupContext(ctx)
	defer cancel()
	status, kind, message := failure(cause)
	now := time.Now().UnixMilli()
	ex.Run.Status, ex.Run.ErrorKind, ex.Run.ErrorMessage, ex.Run.UpdatedAt, ex.Run.FinishedAt = status, &kind, &message, now, &now
	if err := e.repository.Finish(cleanup, ex.Run); err != nil {
		e.logger.Error("保存未启动执行的终态失败", "module", "run", "run_id", ex.Run.ID)
	}
}
