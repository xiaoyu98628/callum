package app

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"path/filepath"
	"sync"
	"time"

	"github.com/xiaoyu98628/callum/internal/agent"
	"github.com/xiaoyu98628/callum/internal/agent/approval"
	"github.com/xiaoyu98628/callum/internal/agent/events"
	agentexecutor "github.com/xiaoyu98628/callum/internal/agent/executor"
	agentservice "github.com/xiaoyu98628/callum/internal/agent/service"
	"github.com/xiaoyu98628/callum/internal/config"
	conversationservice "github.com/xiaoyu98628/callum/internal/conversation/service"
	"github.com/xiaoyu98628/callum/internal/inference"
	"github.com/xiaoyu98628/callum/internal/inference/executor"
	providerservice "github.com/xiaoyu98628/callum/internal/model/provider/service"
	"github.com/xiaoyu98628/callum/internal/settings"
	settingsservice "github.com/xiaoyu98628/callum/internal/settings/service"
	"github.com/xiaoyu98628/callum/internal/storage/sqlite"
	agentstore "github.com/xiaoyu98628/callum/internal/storage/sqlite/agent"
	"github.com/xiaoyu98628/callum/internal/storage/sqlite/catalog"
	conversationstore "github.com/xiaoyu98628/callum/internal/storage/sqlite/conversation"
	settingsstore "github.com/xiaoyu98628/callum/internal/storage/sqlite/settings"
	workspacestore "github.com/xiaoyu98628/callum/internal/storage/sqlite/workspace"
	"github.com/xiaoyu98628/callum/internal/tool/builtin/filesystem"
	toolexecutor "github.com/xiaoyu98628/callum/internal/tool/executor"
	"github.com/xiaoyu98628/callum/internal/tool/registry"
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
	manager       *agentexecutor.Manager
	events        *events.Hub
	providers     *providerservice.Service
	workspaces    *workspaceservice.Service
	conversations *conversationservice.Service
	engine        *agentservice.Engine
	approvals     *approval.Service
	settings      *settingsservice.Service
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
	if ctx == nil || logger == nil || !filepath.IsAbs(cfg.DatabasePath) || cfg.MaxConcurrentRuns < 1 || cfg.MaxConcurrentRuns > 64 || cfg.CallTimeout <= 0 || cfg.ToolTimeout <= 0 {
		return nil, errors.New("应用运行配置无效")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
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
	// 实例锁覆盖迁移与恢复，另一个运行时不能在启动失败前改写活动实例的结构。
	r.ownership, err = sqlite.AcquireOwnership(cfg.DatabasePath)
	if err != nil {
		return nil, err
	}
	r.db, err = sqlite.Open(initCtx, cfg.DatabasePath)
	if err != nil {
		return nil, err
	}
	settingsRepository, err := settingsstore.NewRepository(r.db.DB)
	if err != nil {
		return nil, err
	}
	r.settings, err = settingsservice.New(settingsRepository)
	if err != nil {
		return nil, err
	}
	if _, err := r.settings.Get(initCtx, settings.ExecutionKey); err != nil {
		return nil, err
	}
	conversationRepository, err := conversationstore.NewRepository(r.db.DB)
	if err != nil {
		return nil, err
	}
	runRepository, err := agentstore.NewRepository(r.db.DB)
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
	r.manager, err = agentexecutor.New(runtimeCtx, cfg.MaxConcurrentRuns, cfg.CallTimeout)
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
	r.approvals, err = approval.New(runRepository, runRepository.Get, r.events.Publish)
	if err != nil {
		return nil, err
	}
	registry := registry.New()
	for _, instance := range filesystem.Tools() {
		if err := registry.Register(instance); err != nil {
			return nil, err
		}
	}
	tools, err := toolexecutor.New(registry, cfg.ToolTimeout)
	if err != nil {
		return nil, err
	}
	r.engine, err = agentservice.New(runRepository, conversationRepository, r.manager, r.events.Publish, logger, agentservice.Options{Tools: tools, Approvals: r.approvals, Workspace: r.workspaces.Get, CallTimeout: cfg.CallTimeout})
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

// Approvals 返回与通信协议无关的审批服务。
func (r *Runtime) Approvals() *approval.Service { return r.approvals }

// Settings 返回数据库应用设置服务；更新成功后由后续受理执行读取新配置。
func (r *Runtime) Settings() *settingsservice.Service { return r.settings }

// Runs 返回运行时持有的持久化执行查询服务。
func (r *Runtime) Runs() *agentservice.Engine { return r.engine }

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
		return agent.ErrClosed
	}
	r.sessions.Add(1)
	return nil
}
