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
