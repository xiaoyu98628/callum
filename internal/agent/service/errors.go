package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/xiaoyu98628/callum/internal/agent"
	"github.com/xiaoyu98628/callum/internal/conversation/compaction"
	"github.com/xiaoyu98628/callum/internal/inference"
	"github.com/xiaoyu98628/callum/internal/model"
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
	status = agent.Failed
	switch {
	case errors.Is(err, model.ErrTokenBudget):
		return status, "token_budget_unconfigured", "请配置模型上下文容量或有效的输入 Token 预算"
	case errors.Is(err, model.ErrTokenLimit):
		return status, "token_budget_exceeded", "保留上下文或单个完整轮次超过 Token 预算，请调整预算或保留轮次"
	case errors.Is(err, compaction.ErrNoReduction):
		return status, "compaction_not_reduced", "摘要未能缩小上下文，原有效版本已保留"
	case errors.Is(err, agent.ErrStorage):
		return status, "internal", "保存执行数据失败"
	case errors.Is(err, context.Canceled):
		return agent.Cancelled, "cancelled", "执行已取消"
	case errors.Is(err, context.DeadlineExceeded):
		return status, "timeout", "执行超时"
	case errors.Is(err, agent.ErrOutputLimit):
		return status, "output_limit", "内容超过字节上限"
	case errors.Is(err, model.ErrUnsupportedOutput), errors.Is(err, inference.ErrUnsupported):
		return status, "unsupported", "当前适配器不支持该输入或输出"
	case errors.Is(err, model.ErrRefused):
		return status, "refused", "模型拒绝生成内容"
	case errors.Is(err, model.ErrIncomplete), errors.Is(err, io.ErrUnexpectedEOF):
		return status, "incomplete", "模型输出未完整完成"
	case errors.Is(err, model.ErrInvalidResponse):
		if detail, ok := errors.AsType[*model.ToolArgumentsError](err); ok {
			return status, "invalid_response", detail.Error()
		}
		if detail, ok := errors.AsType[*model.ToolCorrectionStalledError](err); ok {
			return status, "invalid_response", detail.Error()
		}
		return status, "invalid_response", "模型响应格式无效"
	case errors.Is(err, model.ErrImageLimit):
		return status, "image_limit", model.ErrImageLimit.Error()
	case errors.Is(err, model.ErrImageRead):
		return status, "image_read", "图片文件不可读取"
	case errors.Is(err, model.ErrImageFormat):
		return status, "image_format", "图片格式不支持"
	case errors.Is(err, model.ErrInputLimit):
		if limit, ok := errors.AsType[*model.InputLimitError](err); ok {
			return status, "input_limit", limit.Error()
		}
		return status, "input_limit", "模型输入超过字节上限"
	default:
		if call, ok := errors.AsType[*model.CallError](err); ok && call.StatusCode > 0 {
			return status, "model_failed", fmt.Sprintf("模型调用失败（HTTP %d）", call.StatusCode)
		}
		return status, "model_failed", "执行失败"
	}
}

// operationStorageError 区分受理或运行上下文取消、内容边界与真正的持久化故障。
// 终态清理写入失败仍由调用方显式标记 ErrStorage，不能当作普通取消忽略。
func operationStorageError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		return errors.Join(ctx.Err(), err)
	}
	if errors.Is(err, model.ErrInputLimit) || errors.Is(err, agent.ErrOutputLimit) {
		return err
	}
	return errors.Join(agent.ErrStorage, err)
}
