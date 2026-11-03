package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/xiaoyu98628/callum/internal/inference"
	"github.com/xiaoyu98628/callum/internal/model"
	"github.com/xiaoyu98628/callum/internal/run"
)

// requestFailure 只保留受控错误信息，不保存可能回显凭据的供应商错误体。
type requestFailure struct {
	HTTPStatus int    `json:"http_status,omitempty"`
	Operation  string `json:"operation"`
	RequestID  string `json:"provider_request_id,omitempty"`
}

func fillFailure(result *model.Response, protocol string, err error) {
	call, ok := errors.AsType[*model.CallError](err)
	if !ok {
		return
	}
	if result.RequestID == "" {
		result.RequestID = call.RequestID
	}
	if result.Output != nil {
		return
	}
	envelope := struct {
		Protocol string         `json:"protocol"`
		Format   string         `json:"format"`
		Error    requestFailure `json:"error"`
	}{protocol, "request_error", requestFailure{call.StatusCode, call.Operation, call.RequestID}}
	// 结构仅含字符串和整数，JSON 编码不可能失败。
	result.Output, _ = json.Marshal(envelope)
}

func failure(err error) (status, kind, message string) {
	status = run.Failed
	switch {
	case errors.Is(err, run.ErrStorage):
		return status, "internal", "保存执行数据失败"
	case errors.Is(err, context.Canceled):
		return run.Cancelled, "cancelled", "执行已取消"
	case errors.Is(err, context.DeadlineExceeded):
		return status, "timeout", "执行超时"
	case errors.Is(err, run.ErrOutputLimit):
		return status, "output_limit", "内容超过字节上限"
	case errors.Is(err, model.ErrUnsupportedOutput), errors.Is(err, inference.ErrUnsupported):
		return status, "unsupported", "当前适配器不支持该输入或输出"
	case errors.Is(err, model.ErrRefused):
		return status, "refused", "模型拒绝生成内容"
	case errors.Is(err, model.ErrIncomplete), errors.Is(err, io.ErrUnexpectedEOF):
		return status, "incomplete", "模型输出未完整完成"
	case errors.Is(err, model.ErrInvalidResponse):
		return status, "invalid_response", "模型响应格式无效"
	case errors.Is(err, model.ErrImageRead):
		return status, "image_read", "图片文件不可读取"
	case errors.Is(err, model.ErrImageFormat):
		return status, "image_format", "图片格式不支持"
	case errors.Is(err, model.ErrInputLimit):
		return status, "input_limit", "模型输入超过字节上限"
	default:
		if call, ok := errors.AsType[*model.CallError](err); ok && call.StatusCode > 0 {
			return status, "model_failed", fmt.Sprintf("模型调用失败（HTTP %d）", call.StatusCode)
		}
		return status, "model_failed", "执行失败"
	}
}
