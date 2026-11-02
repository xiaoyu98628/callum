package executor

import (
	"context"
	"crypto/rand"
	"strings"
	"sync"
	"time"

	"github.com/xiaoyu98628/callum/internal/inference"
	"github.com/xiaoyu98628/callum/internal/model"
)

type execution struct {
	cancel    context.CancelFunc
	finishing bool
}

// Manager 持有执行的取消函数，创建者必须调用 Close 取消并等待全部任务。
// emit 必须支持并发调用并及时返回错误，accepted 必须在返回前将响应排在事件之前。
type Manager struct {
	ctx     context.Context
	resolve inference.Resolver
	emit    func(inference.Event) error
	limit   int
	timeout time.Duration
	mu      sync.Mutex
	closed  bool
	runs    map[string]*execution
	wg      sync.WaitGroup
}

// New 创建管理器；ctx 是服务运行上下文，不能使用短期初始化上下文。
func New(ctx context.Context, resolve inference.Resolver, limit int, timeout time.Duration, emit func(inference.Event) error) (*Manager, error) {
	if resolve == nil || emit == nil || limit < 1 || limit > 64 || timeout <= 0 {
		return nil, inference.ErrInvalidRequest
	}
	return &Manager{ctx: ctx, resolve: resolve, emit: emit, limit: limit, timeout: timeout, runs: make(map[string]*execution)}, nil
}

// Limits 返回创建时确定的最大并发数和单次调用超时，供调用方展示运行限制。
func (m *Manager) Limits() (int, time.Duration) { return m.limit, m.timeout }

// Start 校验并解析配置，accepted 成功后异步执行；接受失败时不调用模型。
// ctx 只控制配置解析，执行使用 Manager 的运行上下文与独立超时。
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
	id := rand.Text()
	runCtx, cancel := context.WithTimeout(m.ctx, m.timeout)
	m.mu.Lock()
	if m.closed || m.ctx.Err() != nil {
		m.mu.Unlock()
		cancel()
		return inference.ErrClosed
	}
	if len(m.runs) >= m.limit {
		m.mu.Unlock()
		cancel()
		return inference.ErrBusy
	}
	m.runs[id] = &execution{cancel: cancel}
	m.wg.Add(1)
	m.mu.Unlock()
	started := false
	defer func() {
		if !started {
			m.release(id, cancel)
		}
	}()
	lookupCtx, lookupCancel := context.WithCancel(ctx)
	stop := context.AfterFunc(runCtx, lookupCancel)
	client, remoteID, err := m.resolve(lookupCtx, in.ModelID)
	lookupErr := lookupCtx.Err()
	stop()
	lookupCancel()
	if runCtx.Err() != nil {
		return runCtx.Err()
	}
	if err != nil {
		return err
	}
	if lookupErr != nil {
		return lookupErr
	}
	if client == nil || strings.TrimSpace(remoteID) == "" {
		return inference.ErrInvalidRequest
	}
	if err := accepted(id); err != nil {
		return err
	}
	started = true
	go func() {
		defer m.release(id, cancel)
		m.execute(runCtx, id, client, model.Request{Model: remoteID, Messages: in.Messages}, in.Stream)
	}()
	return nil
}

func (m *Manager) release(id string, cancel context.CancelFunc) {
	cancel()
	m.mu.Lock()
	delete(m.runs, id)
	m.mu.Unlock()
	m.wg.Done()
}

// Cancel 请求取消指定执行；不存在或已经确定终态时返回 false。
func (m *Manager) Cancel(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	run := m.runs[id]
	if run == nil || run.finishing {
		return false
	}
	run.cancel()
	return true
}

// Close 停止接受任务，取消并等待全部执行，可重复调用。
func (m *Manager) Close() {
	m.mu.Lock()
	m.closed = true
	for _, run := range m.runs {
		run.cancel()
	}
	m.mu.Unlock()
	m.wg.Wait()
}
