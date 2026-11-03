package executor

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/xiaoyu98628/callum/internal/run"
)

// Manager 统一限制普通对话和无历史调用；创建者必须 Close 并等待资源释放。
type Manager struct {
	ctx     context.Context
	cancel  context.CancelFunc
	limit   int
	timeout time.Duration
	mu      sync.Mutex
	closed  bool
	runs    map[string]*Lease
	keys    map[string]string
	calls   chan struct{}
	changed chan struct{}
	wg      sync.WaitGroup
}

// Lease 预留执行容量，调用方必须 Start 或 Abort，不能遗留未启动的预留。
type Lease struct {
	manager            *Manager
	id, key            string
	ctx                context.Context
	cancel             context.CancelFunc
	started, finishing bool
	aborted            bool
	finalErr           error
	once               sync.Once
}

func New(ctx context.Context, limit int, timeout time.Duration) (*Manager, error) {
	if ctx == nil || limit < 1 || limit > 64 || timeout <= 0 {
		return nil, run.ErrInvalid
	}
	runtime, cancel := context.WithCancel(ctx)
	return &Manager{ctx: runtime, cancel: cancel, limit: limit, timeout: timeout, runs: make(map[string]*Lease), keys: make(map[string]string), calls: make(chan struct{}, limit), changed: make(chan struct{})}, nil
}

// ReserveContext 只等待已确定终态的任务退出，防止客户端收到终态后立即发送仍遇到忙碌。
// 正在执行的任务占满容量时仍立即拒绝，不引入后台等待队列。
func (m *Manager) ReserveContext(ctx context.Context, id, key string) (*Lease, error) {
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		lease, err := m.Reserve(id, key)
		if !errors.Is(err, run.ErrBusy) {
			return lease, err
		}
		m.mu.Lock()
		if m.closed {
			m.mu.Unlock()
			return nil, run.ErrClosed
		}
		changed := m.changed
		wait, retry := false, false
		blocking := m.runs[id]
		if existing := m.runs[m.keys[key]]; key != "" && existing != nil {
			blocking = existing
		}
		if blocking != nil {
			wait = blocking.finishing
		} else if len(m.runs) < m.limit {
			retry = true
		} else {
			for _, existing := range m.runs {
				wait = wait || existing.finishing
			}
		}
		m.mu.Unlock()
		if retry {
			continue
		}
		if !wait {
			return nil, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-m.ctx.Done():
			return nil, run.ErrClosed
		case <-changed:
		}
	}
}

func (m *Manager) Limits() (int, time.Duration) { return m.limit, m.timeout }

// Reserve 同时预留全局执行容量和可选会话互斥键，不持有数据库资源。
func (m *Manager) Reserve(id, key string) (*Lease, error) {
	if id == "" {
		return nil, run.ErrInvalid
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed || m.ctx.Err() != nil {
		return nil, run.ErrClosed
	}
	if len(m.runs) >= m.limit || m.runs[id] != nil || (key != "" && m.keys[key] != "") {
		return nil, run.ErrBusy
	}
	ctx, cancel := context.WithTimeout(m.ctx, m.timeout)
	lease := &Lease{manager: m, id: id, key: key, ctx: ctx, cancel: cancel}
	m.runs[id] = lease
	if key != "" {
		m.keys[key] = id
	}
	m.wg.Add(1)
	return lease, nil
}

func (l *Lease) Context() context.Context { return l.ctx }

// Start 在受理响应成功排队后调用；即使取消已发生，也启动任务完成持久化清理。
func (l *Lease) Start(task func(context.Context)) {
	l.manager.mu.Lock()
	if l.started || l.aborted {
		l.manager.mu.Unlock()
		return
	}
	l.started = true
	l.manager.mu.Unlock()
	go func() { defer l.release(); task(l.ctx) }()
}

func (l *Lease) Abort() {
	l.manager.mu.Lock()
	if l.started || l.aborted {
		l.manager.mu.Unlock()
		return
	}
	l.aborted = true
	l.manager.mu.Unlock()
	l.release()
}

func (l *Lease) release() {
	l.once.Do(func() {
		l.cancel()
		m := l.manager
		m.mu.Lock()
		delete(m.runs, l.id)
		if l.key != "" {
			delete(m.keys, l.key)
		}
		close(m.changed)
		m.changed = make(chan struct{})
		m.mu.Unlock()
		m.wg.Done()
	})
}

// Finishing 与 Cancel 串行确定终态，返回已经发生的取消或超时。
func (l *Lease) Finishing() error {
	l.manager.mu.Lock()
	defer l.manager.mu.Unlock()
	if !l.finishing {
		l.finishing = true
		l.finalErr = l.ctx.Err()
	}
	return l.finalErr
}

func (m *Manager) Cancel(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	lease := m.runs[id]
	if lease == nil || lease.finishing {
		return false
	}
	lease.cancel()
	return true
}

// Call 限制实际模型请求数；标题请求也使用同一容量，不能绕过限制。
func (m *Manager) Call(ctx context.Context, task func() error) error {
	select {
	case <-m.ctx.Done():
		return run.ErrClosed
	case <-ctx.Done():
		return ctx.Err()
	case m.calls <- struct{}{}:
	}
	defer func() { <-m.calls }()
	if err := ctx.Err(); err != nil {
		return err
	}
	if m.ctx.Err() != nil {
		return run.ErrClosed
	}
	return task()
}

// TryCall 不排队等待辅助调用，主回答优先，返回值表示是否取得容量。
func (m *Manager) TryCall(ctx context.Context, task func() error) (bool, error) {
	release, ok, err := m.TryAcquireCall(ctx)
	if !ok || err != nil {
		return ok, err
	}
	defer release()
	return true, task()
}

// TryAcquireCall 为辅助任务立即预留模型请求容量，取得后必须调用返回的释放函数。
// 预留发生在启动 goroutine 之前，避免主回答完成后辅助任务才争抢到空闲容量。
func (m *Manager) TryAcquireCall(ctx context.Context) (func(), bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	select {
	case <-m.ctx.Done():
		return nil, false, run.ErrClosed
	case m.calls <- struct{}{}:
		if err := ctx.Err(); err != nil {
			<-m.calls
			return nil, false, err
		}
		if m.ctx.Err() != nil {
			<-m.calls
			return nil, false, run.ErrClosed
		}
		var once sync.Once
		return func() { once.Do(func() { <-m.calls }) }, true, nil
	default:
		return nil, false, nil
	}
}

func (m *Manager) Close() {
	m.mu.Lock()
	m.closed = true
	m.cancel()
	for _, lease := range m.runs {
		lease.cancel()
	}
	close(m.changed)
	m.changed = make(chan struct{})
	m.mu.Unlock()
	m.wg.Wait()
}
