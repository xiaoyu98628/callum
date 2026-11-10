package inference

import (
	"context"
	"errors"

	"github.com/xiaoyu98628/callum/internal/agent"
	"github.com/xiaoyu98628/callum/internal/model"
	"github.com/xiaoyu98628/callum/internal/model/provider"
)

// Failure 只包含可公开的错误描述，不保留原始错误链或供应商响应正文。
type Failure struct {
	Kind              string
	Message           string
	ProviderRequestID string
	HTTPStatus        int
}

// Notification 是无历史模型调用的只读事件数据，协议编码由通信边界处理。
type Notification struct {
	Text              *string
	ProviderRequestID string
	Failure           *Failure
}

// DescribeError 统一提取受控错误信息，防止事件队列保存凭据或大型错误正文。
func DescribeError(err error) Failure {
	e := Failure{Kind: "internal", Message: "内部处理失败"}
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		e.Message, e.Kind = "操作超时", "timeout"
	case errors.Is(err, context.Canceled):
		e.Message, e.Kind = "操作已取消", "cancelled"
	case errors.Is(err, provider.ErrInvalidConfig), errors.Is(err, ErrInvalidRequest), errors.Is(err, agent.ErrInvalid):
		e.Message, e.Kind = "参数无效", "invalid_params"
	case errors.Is(err, provider.ErrNotFound):
		e.Message, e.Kind = "配置不存在", "not_found"
	case errors.Is(err, agent.ErrNotFound):
		e.Message, e.Kind = "执行或步骤不存在", "not_found"
	case errors.Is(err, provider.ErrConflict):
		e.Message, e.Kind = "同一厂商下的远端模型标识已存在", "conflict"
	case errors.Is(err, provider.ErrReferenced):
		e.Message, e.Kind = "关联配置不存在或仍被引用", "referenced"
	case errors.Is(err, provider.ErrDisabled):
		e.Message, e.Kind = "模型已禁用", "disabled"
	case errors.Is(err, provider.ErrCredentials):
		e.Message, e.Kind = "系统凭据库不可用或凭据无法解密，请解锁凭据库后重试。", "credentials_unavailable"
	case errors.Is(err, ErrUnsupported), errors.Is(err, model.ErrUnsupportedOutput):
		e.Message, e.Kind = "当前适配器不支持该模型功能或输出", "unsupported"
	case errors.Is(err, ErrBusy):
		e.Message, e.Kind = "模型调用已达到并发上限", "busy"
	case errors.Is(err, ErrClosed):
		e.Message, e.Kind = "服务正在关闭", "closing"
	case errors.Is(err, ErrOutputLimit):
		e.Message, e.Kind = "模型文本超过大小限制", "output_limit"
	case errors.Is(err, model.ErrIncomplete):
		e.Message, e.Kind = "模型输出未完整完成", "incomplete"
	case errors.Is(err, model.ErrRefused):
		e.Message, e.Kind = "模型拒绝生成内容", "refused"
	case errors.Is(err, model.ErrInvalidResponse):
		e.Message, e.Kind = "模型响应格式无效", "invalid_response"
	default:
		e.Message, e.Kind = "模型调用失败", "model_failed"
	}

	if call, ok := errors.AsType[*model.CallError](err); ok {
		e.ProviderRequestID, e.HTTPStatus = call.RequestID, call.StatusCode
	}
	return e
}

// PublicEvent 在发布到共享订阅前移除错误链。
func PublicEvent(event Event) agent.Event {
	data := Notification{Text: event.Text, ProviderRequestID: event.ProviderRequestID}
	if event.Err != nil {
		failure := DescribeError(event.Err)
		data.Failure = &failure
	}
	return agent.Event{Type: "run." + event.Type, RunID: event.RunID, Data: data}
}
