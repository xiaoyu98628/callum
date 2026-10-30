package inference

import (
	"context"
	"crypto/rand"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/xiaoyu98628/callum/internal/model"
)

var (
	ErrInvalidRequest = errors.New("模型调用参数无效")
	ErrBusy           = errors.New("模型调用已达到并发上限")
	ErrClosed         = errors.New("执行管理器已关闭")
	ErrUnsupported    = errors.New("当前适配器不支持选中的模型功能")
	ErrOutputLimit    = errors.New("模型输出超过大小限制")
)

// MaxTextBytes 限制单次调用累计的文本字节数，不代表模型的 Token 容量。
const MaxTextBytes = 1 << 20

// Resolver 为一次调用取得配置快照和统一客户端，不得在返回后持有数据库事务。
type Resolver func(context.Context, string) (model.Client, string, error)

// Request 由调用方提供完整文本上下文，不负责保存会话历史。
type Request struct {
	ModelID  string
	Stream   bool
	Messages []model.Message
}

// Event 是执行事件，Err 仅供通信边界转换，不能直接序列化错误链。
type Event struct {
	Type              string
	RunID             string
	Text              *string
	ProviderRequestID string
	Err               error
}

type execution struct {
	cancel    context.CancelFunc
	finishing bool
}

// Manager 持有执行的取消函数，创建者必须调用 Close 取消并等待全部任务。
// emit 必须支持并发调用并及时返回错误，accepted 必须在返回前将响应排在事件之前。
type Manager struct {
	ctx     context.Context
	resolve Resolver
	emit    func(Event) error
	limit   int
	timeout time.Duration
	mu      sync.Mutex
	closed  bool
	runs    map[string]*execution
	wg      sync.WaitGroup
}

// New 创建管理器；ctx 是服务运行上下文，不能使用短期初始化上下文。
func New(ctx context.Context, resolve Resolver, limit int, timeout time.Duration, emit func(Event) error) (*Manager, error) {
	if resolve == nil || emit == nil || limit < 1 || limit > 64 || timeout <= 0 {
		return nil, ErrInvalidRequest
	}
	return &Manager{ctx: ctx, resolve: resolve, emit: emit, limit: limit, timeout: timeout, runs: make(map[string]*execution)}, nil
}

// Limits 返回创建时确定的最大并发数和单次调用超时，供调用方展示运行限制。
func (m *Manager) Limits() (int, time.Duration) { return m.limit, m.timeout }

// Start 校验并解析配置，accepted 成功后异步执行；接受失败时不调用模型。
// ctx 只控制配置解析，执行使用 Manager 的运行上下文与独立超时。
func (m *Manager) Start(ctx context.Context, in Request, accepted func(string) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if accepted == nil || strings.TrimSpace(in.ModelID) == "" || len(in.Messages) == 0 || len(in.Messages) > 256 {
		return ErrInvalidRequest
	}
	for _, msg := range in.Messages {
		if msg.Role != model.RoleSystem && msg.Role != model.RoleUser && msg.Role != model.RoleAssistant {
			return ErrInvalidRequest
		}
	}
	in.Messages = append([]model.Message(nil), in.Messages...)
	id := rand.Text()
	runCtx, cancel := context.WithTimeout(m.ctx, m.timeout)
	m.mu.Lock()
	if m.closed || m.ctx.Err() != nil {
		m.mu.Unlock()
		cancel()
		return ErrClosed
	}
	if len(m.runs) >= m.limit {
		m.mu.Unlock()
		cancel()
		return ErrBusy
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
		return ErrInvalidRequest
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

func (m *Manager) execute(ctx context.Context, id string, client model.Client, req model.Request, stream bool) {
	event := Event{Type: "completed", RunID: id}
	var err error
	if stream {
		bytes := 0
		err = client.Stream(ctx, req, func(delta model.Event) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			event.ProviderRequestID = delta.RequestID
			if delta.Type != model.EventTextDelta {
				return nil
			}
			bytes += len(delta.Text)
			if bytes > MaxTextBytes {
				return ErrOutputLimit
			}
			return m.emit(Event{Type: "text_delta", RunID: id, Text: new(delta.Text), ProviderRequestID: delta.RequestID})
		})
	} else {
		var response model.Response
		response, err = client.Generate(ctx, req)
		event.ProviderRequestID = response.RequestID
		if len(response.Text) > MaxTextBytes {
			err = errors.Join(err, ErrOutputLimit)
		} else {
			event.Text = new(response.Text)
		}
	}
	m.mu.Lock()
	// 与取消操作串行确定终态，已进入终态的执行不能再被标记为取消。
	if ctx.Err() != nil {
		err = ctx.Err()
	}
	m.runs[id].finishing = true
	m.mu.Unlock()
	if err != nil {
		event.Type, event.Err = "failed", err
		if errors.Is(err, context.Canceled) {
			event.Type = "cancelled"
		}
		if call, ok := errors.AsType[*model.CallError](err); ok {
			event.ProviderRequestID = call.RequestID
		}
	}
	// 只有客户端方法返回成功才发出 completed；适配器内部的完成回调不直接成为执行终态。
	// 输出失败意味着通信已中断，由传入的 emit 关闭连接，不能再次向同一连接报告错误。
	if emitErr := m.emit(event); emitErr != nil {
		return
	}
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
