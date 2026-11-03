package execution

import (
	"context"
	"time"

	"github.com/xiaoyu98628/callum/internal/inference"
	"github.com/xiaoyu98628/callum/internal/inference/executor"
	"github.com/xiaoyu98628/callum/internal/model"
	"github.com/xiaoyu98628/callum/internal/model/provider"
	runservice "github.com/xiaoyu98628/callum/internal/run/service"
	"github.com/xiaoyu98628/callum/internal/stdio/protocol"
)

// Handler 处理模型调用和独立取消，不负责创建或关闭执行器。
type Handler struct {
	runs       *executor.Manager
	persistent *runservice.Engine
}

// New 使用已初始化的非 nil 执行器创建接口处理器，执行器由调用方持有。
func New(runs *executor.Manager) *Handler { return &Handler{runs: runs} }

// NewPersistent 使用共享调度器入口和持久化运行服务。
func NewPersistent(runs *executor.Manager, persistent *runservice.Engine) *Handler {
	return &Handler{runs: runs, persistent: persistent}
}

// Limits 返回执行器的并发上限与单次调用超时。
func (s *Handler) Limits() (int, time.Duration) { return s.runs.Limits() }

// Invoke 接受调用并异步执行；reply 必须在返回前将响应排在事件之前。
func (s *Handler) Invoke(ctx context.Context, req protocol.Request, reply func(any, *protocol.Error) error) error {
	var p invokeParams
	if err := protocol.DecodeParams(req.Params, &p); err != nil {
		return reply(nil, err)
	}
	if !provider.ValidID(p.ModelID) {
		return reply(nil, protocol.InvalidParams())
	}
	in := inference.Request{ModelID: p.ModelID, Stream: true}
	if p.Stream != nil {
		in.Stream = *p.Stream
	}
	for _, message := range p.Messages {
		if message.Content == nil {
			return reply(nil, protocol.InvalidParams())
		}
		in.Messages = append(in.Messages, model.Message{Role: message.Role, Content: *message.Content})
	}
	err := s.runs.Start(ctx, in, func(id string) error { return reply(map[string]any{"run_id": id}, nil) })
	if err != nil {
		return reply(nil, publicError(err))
	}
	return nil
}

// Cancel 校验取消参数，返回请求取消的结果，不等待模型执行结束。
func (s *Handler) Cancel(req protocol.Request) (any, *protocol.Error) {
	var p cancelParams
	if err := protocol.DecodeParams(req.Params, &p); err != nil {
		return nil, err
	}
	if p.RunID == "" {
		return nil, protocol.InvalidParams()
	}
	return map[string]any{"cancel_requested": s.runs.Cancel(p.RunID)}, nil
}
