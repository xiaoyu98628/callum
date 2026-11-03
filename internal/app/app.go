package app

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/xiaoyu98628/callum/internal/config"
	conversationservice "github.com/xiaoyu98628/callum/internal/conversation/service"
	"github.com/xiaoyu98628/callum/internal/inference"
	"github.com/xiaoyu98628/callum/internal/inference/executor"
	providerservice "github.com/xiaoyu98628/callum/internal/model/provider/service"
	"github.com/xiaoyu98628/callum/internal/run"
	"github.com/xiaoyu98628/callum/internal/run/events"
	runexecutor "github.com/xiaoyu98628/callum/internal/run/executor"
	runservice "github.com/xiaoyu98628/callum/internal/run/service"
	"github.com/xiaoyu98628/callum/internal/storage/sqlite"
	"github.com/xiaoyu98628/callum/internal/storage/sqlite/catalog"
	conversationstore "github.com/xiaoyu98628/callum/internal/storage/sqlite/conversation"
	runstore "github.com/xiaoyu98628/callum/internal/storage/sqlite/run"
	workspacestore "github.com/xiaoyu98628/callum/internal/storage/sqlite/workspace"
	workspaceservice "github.com/xiaoyu98628/callum/internal/workspace/service"
)

// Runtime 持有业务服务、共享调度器、事件发布器和存储，不依赖通信连接。
// 创建者必须 Close；借用的业务服务不得在 Close 返回后继续使用。
type Runtime struct {
	ctx           context.Context
	cancel        context.CancelFunc
	logger        *slog.Logger
	db            *sqlite.Database
	ownership     *sqlite.Ownership
	httpTransport *http.Transport
	manager       *runexecutor.Manager
	events        *events.Hub
	providers     *providerservice.Service
	workspaces    *workspaceservice.Service
	conversations *conversationservice.Service
	engine        *runservice.Engine
	calls         *executor.Manager
	mu            sync.Mutex
	closed        bool
	sessions      sync.WaitGroup
	closeOnce     sync.Once
	closeErr      error
}

// New 装配与协议无关的运行时；ctx 是应用运行上下文，初始化使用独立短超时。
// 部分初始化失败时关闭此前取得的资源，且不覆盖原始错误。
func New(ctx context.Context, cfg config.Config, logger *slog.Logger) (_ *Runtime, retErr error) {
	if ctx == nil || logger == nil || cfg.MaxConcurrentRuns < 1 || cfg.MaxConcurrentRuns > 64 || cfg.CallTimeout <= 0 {
		return nil, errors.New("应用运行配置无效")
	}
	runtimeCtx, cancel := context.WithCancel(ctx)
	r := &Runtime{ctx: runtimeCtx, cancel: cancel, logger: logger}
	defer func() {
		if retErr != nil {
			retErr = errors.Join(retErr, r.Close())
		}
	}()
	initCtx, initCancel := context.WithTimeout(runtimeCtx, 10*time.Second)
	defer initCancel()
	var err error
	r.db, err = sqlite.Open(initCtx, cfg.DatabasePath)
	if err != nil {
		return nil, err
	}
	r.ownership, err = sqlite.AcquireOwnership(cfg.DatabasePath)
	if err != nil {
		return nil, err
	}
	conversationRepository, err := conversationstore.NewRepository(r.db.DB)
	if err != nil {
		return nil, err
	}
	runRepository, err := runstore.NewRepository(r.db.DB)
	if err != nil {
		return nil, err
	}
	if err := runRepository.Recover(initCtx); err != nil {
		return nil, err
	}
	workspaceRepository, err := workspacestore.NewRepository(r.db.DB)
	if err != nil {
		return nil, err
	}
	r.workspaces, err = workspaceservice.New(workspaceRepository)
	if err != nil {
		return nil, err
	}
	repository, err := catalog.NewRepository(r.db.DB)
	if err != nil {
		return nil, err
	}
	r.providers, err = providerservice.New(repository)
	if err != nil {
		return nil, err
	}
	client, httpTransport := newHTTPClient(cfg.MaxConcurrentRuns)
	r.httpTransport = httpTransport
	r.manager, err = runexecutor.New(runtimeCtx, cfg.MaxConcurrentRuns, cfg.CallTimeout)
	if err != nil {
		return nil, err
	}
	// 每个订阅最多排队 64 个事件，运行时最多持有 32 个订阅，无消费者时也可执行。
	r.events, err = events.New(64, 32)
	if err != nil {
		return nil, err
	}
	r.calls, err = executor.NewWithScheduler(r.manager, newModelResolver(r.providers, client), func(event inference.Event) error {
		return r.events.Publish(inference.PublicEvent(event))
	})
	if err != nil {
		return nil, err
	}
	r.engine, err = runservice.New(runRepository, conversationRepository, r.manager, r.events.Publish, logger)
	if err != nil {
		return nil, err
	}
	r.conversations, err = conversationservice.New(conversationRepository, newConversationResolver(r.providers, client), r.manager, r.engine)
	if err != nil {
		return nil, err
	}
	return r, nil
}

// Providers 返回运行时持有的厂商与模型配置服务。
func (r *Runtime) Providers() *providerservice.Service { return r.providers }

// Workspaces 返回运行时持有的工作区服务。
func (r *Runtime) Workspaces() *workspaceservice.Service { return r.workspaces }

// Conversations 返回运行时持有的会话服务。
func (r *Runtime) Conversations() *conversationservice.Service { return r.conversations }

// Runs 返回运行时持有的持久化执行查询服务。
func (r *Runtime) Runs() *runservice.Engine { return r.engine }

// Invoke 提供无历史模型调用，与普通对话共享并发和事件发布器。
func (r *Runtime) Invoke(ctx context.Context, in inference.Request, accepted func(string) error) error {
	return r.calls.Start(ctx, in, accepted)
}

// Subscribe 返回只读业务事件订阅，调用者必须关闭，断开订阅不会取消执行。
func (r *Runtime) Subscribe(filter events.Filter) (*events.Subscription, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed || r.ctx.Err() != nil {
		return nil, events.ErrClosed
	}
	return r.events.Subscribe(filter)
}

// Cancel 请求取消指定执行，不等待结束；不存在或已确定终态时返回 false。
func (r *Runtime) Cancel(id string) bool { return r.manager.Cancel(id) }

// Close 停止入口、取消并等待任务及连接，之后释放网络、数据库和实例锁，可重复调用。
func (r *Runtime) Close() error {
	r.closeOnce.Do(func() {
		r.mu.Lock()
		r.closed = true
		r.cancel()
		r.mu.Unlock()
		if r.manager != nil {
			r.manager.Close()
		}
		r.sessions.Wait()
		if r.events != nil {
			r.events.Close()
		}
		if r.httpTransport != nil {
			r.httpTransport.CloseIdleConnections()
		}
		if r.db != nil {
			r.closeErr = errors.Join(r.closeErr, r.db.Close())
		}
		if r.ownership != nil {
			r.closeErr = errors.Join(r.closeErr, r.ownership.Close())
		}
	})
	return r.closeErr
}

func (r *Runtime) beginSession() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed || r.ctx.Err() != nil {
		return run.ErrClosed
	}
	r.sessions.Add(1)
	return nil
}
