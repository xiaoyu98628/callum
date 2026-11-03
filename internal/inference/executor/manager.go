package executor

import (
	"context"
	"crypto/rand"
	"errors"
	"strings"
	"time"

	"github.com/xiaoyu98628/callum/internal/inference"
	"github.com/xiaoyu98628/callum/internal/model"
	"github.com/xiaoyu98628/callum/internal/run"
	scheduler "github.com/xiaoyu98628/callum/internal/run/executor"
)

// Manager 为兼容的无历史模型调用提供入口，与普通对话共享执行调度器。
// emit 必须支持并发，accepted 必须在返回前将受理响应排在事件之前。
type Manager struct {
	scheduler *scheduler.Manager
	resolve   inference.Resolver
	emit      func(inference.Event) error
}

func New(ctx context.Context, resolve inference.Resolver, limit int, timeout time.Duration, emit func(inference.Event) error) (*Manager, error) {
	if resolve == nil || emit == nil {
		return nil, inference.ErrInvalidRequest
	}
	manager, err := scheduler.New(ctx, limit, timeout)
	if err != nil {
		return nil, inference.ErrInvalidRequest
	}
	return NewWithScheduler(manager, resolve, emit)
}

// NewWithScheduler 借用共享调度器；Close 会取消并等待该调度器的所有任务。
func NewWithScheduler(manager *scheduler.Manager, resolve inference.Resolver, emit func(inference.Event) error) (*Manager, error) {
	if manager == nil || resolve == nil || emit == nil {
		return nil, inference.ErrInvalidRequest
	}
	return &Manager{scheduler: manager, resolve: resolve, emit: emit}, nil
}

func (m *Manager) Limits() (int, time.Duration) { return m.scheduler.Limits() }

// Start 使用请求上下文解析配置，已接受的任务使用独立运行上下文。
func (m *Manager) Start(ctx context.Context, in inference.Request, accepted func(string) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if accepted == nil || strings.TrimSpace(in.ModelID) == "" || len(in.Messages) == 0 || len(in.Messages) > 256 {
		return inference.ErrInvalidRequest
	}
	for _, msg := range in.Messages {
		if msg.Role != model.RoleSystem && msg.Role != model.RoleUser && msg.Role != model.RoleAssistant {
			return inference.ErrInvalidRequest
		}
	}
	in.Messages = append([]model.Message(nil), in.Messages...)
	for i := range in.Messages {
		in.Messages[i].Blocks = append([]model.Block(nil), in.Messages[i].Blocks...)
	}
	id := rand.Text()
	lease, err := m.scheduler.ReserveContext(ctx, id, "")
	if err != nil {
		switch {
		case errors.Is(err, run.ErrBusy):
			return inference.ErrBusy
		case errors.Is(err, run.ErrClosed):
			return inference.ErrClosed
		default:
			return err
		}
	}
	defer lease.Abort()
	lookupCtx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(lease.Context(), cancel)
	client, remote, err := m.resolve(lookupCtx, in.ModelID)
	lookupErr := lookupCtx.Err()
	stop()
	cancel()
	if err := lease.Context().Err(); err != nil {
		return err
	}
	if err != nil {
		return err
	}
	if lookupErr != nil {
		return lookupErr
	}
	if client == nil || strings.TrimSpace(remote) == "" {
		return inference.ErrInvalidRequest
	}
	if err := accepted(id); err != nil {
		return err
	}
	lease.Start(func(ctx context.Context) {
		m.execute(ctx, id, client, model.Request{Model: remote, Messages: in.Messages}, in.Stream, lease)
	})
	return nil
}

func (m *Manager) Cancel(id string) bool { return m.scheduler.Cancel(id) }
func (m *Manager) Close()                { m.scheduler.Close() }
