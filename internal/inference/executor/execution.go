package executor

import (
	"context"
	"errors"

	"github.com/xiaoyu98628/callum/internal/inference"
	"github.com/xiaoyu98628/callum/internal/model"
	scheduler "github.com/xiaoyu98628/callum/internal/run/executor"
)

func (m *Manager) execute(ctx context.Context, id string, client model.Client, req model.Request, stream bool, lease *scheduler.Lease) {
	event := inference.Event{Type: "completed", RunID: id}
	err := m.scheduler.Call(ctx, func() error {
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
				if bytes > inference.MaxTextBytes {
					return inference.ErrOutputLimit
				}
				return m.emit(inference.Event{Type: "text_delta", RunID: id, Text: new(delta.Text), ProviderRequestID: delta.RequestID})
			})
		} else {
			var response model.Response
			response, err = client.Generate(ctx, req)
			event.ProviderRequestID = response.RequestID
			if len(response.Text) > inference.MaxTextBytes {
				err = errors.Join(err, inference.ErrOutputLimit)
			} else {
				event.Text = new(response.Text)
			}
		}
		return err
	})
	// 与取消操作串行确定终态，已进入终态的执行不能再被标记为取消。
	if cancelErr := lease.Finishing(); cancelErr != nil {
		err = errors.Join(cancelErr, err)
	}
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
