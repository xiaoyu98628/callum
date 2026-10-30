package stdio

import (
	"context"
	"errors"

	"github.com/xiaoyu98628/callum/internal/inference"
	"github.com/xiaoyu98628/callum/internal/model"
	"github.com/xiaoyu98628/callum/internal/model/provider"
)

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func invalidParams() *rpcError {
	return &rpcError{Code: -32602, Message: "参数无效，请检查字段、类型和必填值"}
}

// publicError 只输出受控描述，禁止将存储、网络和供应商错误链直接发给客户端。
func publicError(err error, invocation bool) *rpcError {
	e := &rpcError{Code: -32603, Message: "内部处理失败"}
	kind := "internal"
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		e.Code, e.Message, kind = 1008, "操作超时", "timeout"
	case errors.Is(err, context.Canceled):
		e.Code, e.Message, kind = 1007, "操作已取消", "cancelled"
	case errors.Is(err, provider.ErrInvalidConfig), errors.Is(err, inference.ErrInvalidRequest):
		return invalidParams()
	case errors.Is(err, provider.ErrNotFound):
		e.Code, e.Message, kind = 1001, "配置不存在", "not_found"
	case errors.Is(err, provider.ErrConflict):
		e.Code, e.Message, kind = 1002, "同一厂商下的远端模型标识已存在", "conflict"
	case errors.Is(err, provider.ErrReferenced):
		e.Code, e.Message, kind = 1003, "关联配置不存在或仍被引用", "referenced"
	case errors.Is(err, provider.ErrDisabled):
		e.Code, e.Message, kind = 1004, "模型已禁用", "disabled"
	case errors.Is(err, inference.ErrUnsupported), errors.Is(err, model.ErrUnsupportedOutput):
		e.Code, e.Message, kind = 1005, "当前适配器不支持该模型功能或输出", "unsupported"
	case errors.Is(err, inference.ErrBusy):
		e.Code, e.Message, kind = 1006, "模型调用已达到并发上限", "busy"
	case errors.Is(err, inference.ErrClosed):
		e.Code, e.Message, kind = 1011, "服务正在关闭", "closing"
	case errors.Is(err, inference.ErrOutputLimit):
		e.Code, e.Message, kind = 1010, "模型文本超过大小限制", "output_limit"
	case errors.Is(err, model.ErrIncomplete):
		e.Code, e.Message, kind = 1009, "模型输出未完整完成", "incomplete"
	case errors.Is(err, model.ErrRefused):
		e.Code, e.Message, kind = 1009, "模型拒绝生成内容", "refused"
	case errors.Is(err, model.ErrInvalidResponse):
		e.Code, e.Message, kind = 1009, "模型响应格式无效", "invalid_response"
	default:
		if invocation {
			e.Code, e.Message, kind = 1009, "模型调用失败", "model_failed"
		}
	}
	data := map[string]any{"kind": kind}
	if call, ok := errors.AsType[*model.CallError](err); ok {
		if call.RequestID != "" {
			data["provider_request_id"] = call.RequestID
		}
		if call.StatusCode != 0 {
			data["http_status"] = call.StatusCode
		}
	}
	e.Data = data
	return e
}
