package service

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/xiaoyu98628/callum/internal/agent"
	"github.com/xiaoyu98628/callum/internal/agent/approval"
	"github.com/xiaoyu98628/callum/internal/agent/executor"
	"github.com/xiaoyu98628/callum/internal/conversation"
	toolexecutor "github.com/xiaoyu98628/callum/internal/tool/executor"
	"github.com/xiaoyu98628/callum/internal/workspace"
)

// Engine 编排已提交的普通对话执行，不持有数据库事务或 HTTP 资源所有权。
type Engine struct {
	repository    agent.Repository
	conversations conversation.Repository
	manager       *executor.Manager
	emit          func(agent.Event) error
	logger        *slog.Logger
	options       Options
	inputMu       sync.Mutex
	inputSignals  map[string]chan struct{}
	usageMu       sync.RWMutex
	usage         map[string]liveContextUsage
	usageRevision atomic.Int64
}

// Options 保存运行时依赖；审批设置来自每次执行受理时持久化的快照。
type Options struct {
	Tools       *toolexecutor.Executor
	Approvals   *approval.Service
	Workspace   func(context.Context, string) (workspace.Workspace, error)
	CallTimeout time.Duration
}

func New(repository agent.Repository, conversations conversation.Repository, manager *executor.Manager, emit func(agent.Event) error, logger *slog.Logger, options Options) (*Engine, error) {
	if repository == nil || conversations == nil || manager == nil || emit == nil || logger == nil || options.CallTimeout <= 0 || options.Tools == nil || options.Approvals == nil || options.Workspace == nil {
		return nil, agent.ErrInvalid
	}
	for _, definition := range options.Tools.Definitions() {
		if definition.Name == agent.UserInputTool {
			return nil, agent.ErrInvalid
		}
	}
	return &Engine{repository: repository, conversations: conversations, manager: manager, emit: emit, logger: logger, options: options, inputSignals: make(map[string]chan struct{}), usage: make(map[string]liveContextUsage)}, nil
}

func (e *Engine) event(ex conversation.Execution, name, stepID, messageID string, data any) error {
	event := agent.Event{Type: name, ConversationID: ex.Conversation.ID, RunID: ex.Run.ID, StepID: stepID, MessageID: messageID, Data: data}
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

// ConversationUpdated 在会话变更提交后发布通知，订阅故障不能回滚已提交的修改。
func (e *Engine) ConversationUpdated(value conversation.Conversation) {
	if err := e.emit(agent.Event{Type: "conversation.updated", ConversationID: value.ID, Data: value}); err != nil {
		e.logger.Warn("发布会话变更事件失败", "module", "conversation", "conversation_id", value.ID)
	}
}
