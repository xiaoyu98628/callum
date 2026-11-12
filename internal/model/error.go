package model

import (
	"errors"
	"fmt"
)

var (
	ErrInvalidResponse   = errors.New("模型响应格式无效")
	ErrIncomplete        = errors.New("模型生成未完整完成")
	ErrRefused           = errors.New("模型拒绝生成内容")
	ErrUnsupportedOutput = errors.New("暂不支持此类模型输出")
	ErrImageRead         = errors.New("图片文件不可读取")
	ErrImageFormat       = errors.New("图片格式不支持")
	ErrAttachment        = errors.New("附件不可读取或已超过处理范围")
	ErrImageLimit        = errors.New("图片超过处理上限，请缩小或转换后重试")
	ErrInputLimit        = errors.New("模型输入超过字节上限")
	ErrTokenBudget       = errors.New("缺少有效的上下文 Token 预算")
	ErrTokenLimit        = errors.New("当前必须保留的上下文超过 Token 预算")
)

// CallError 保留调用阶段、HTTP 状态和供应商请求标识。
type CallError struct {
	Operation  string
	StatusCode int
	RequestID  string
	cause      error
}

func NewCallError(
	operation string,
	statusCode int,
	requestID string,
	cause error,
) *CallError {
	return &CallError{
		Operation:  operation,
		StatusCode: statusCode,
		RequestID:  requestID,
		cause:      cause,
	}
}

func (e *CallError) Error() string {
	return fmt.Sprintf(
		"model %s failed (status=%d)",
		e.Operation,
		e.StatusCode,
	)
}

func (e *CallError) Unwrap() error {
	return e.cause
}

// ToolArgumentsError 表示完整响应中工具身份有效，但某项 arguments 不是 JSON 对象。
// Fingerprint 是参数摘要，不包含参数原文；仅在整批尚未执行时可请求模型纠正。
type ToolArgumentsError struct {
	Index       int
	Fingerprint string
}

func (e *ToolArgumentsError) Error() string {
	return fmt.Sprintf("模型第 %d 项工具调用的 arguments 必须是合法 JSON 对象，本轮工具均未执行", e.Index+1)
}
func (*ToolArgumentsError) Unwrap() error { return ErrInvalidResponse }

// ToolCorrectionStalledError 表示连续提交相同坏参数且未发生执行进展，不能继续自动纠正。
type ToolCorrectionStalledError struct{}

func (*ToolCorrectionStalledError) Error() string {
	return "模型重复提交相同无效工具参数，没有执行进展，请检查模型服务的工具调用输出"
}
func (*ToolCorrectionStalledError) Unwrap() error { return ErrInvalidResponse }
