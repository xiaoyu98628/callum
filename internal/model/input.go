package model

import (
	"context"
	"fmt"
)

// InputLimitError 区分资源维度；仍可通过 ErrInputLimit 识别输入资源边界。
type InputLimitError struct {
	Dimension     string
	Actual, Limit int64
}

func (e *InputLimitError) Error() string {
	return fmt.Sprintf("模型输入%s超限（%d > %d）", e.Dimension, e.Actual, e.Limit)
}
func (e *InputLimitError) Unwrap() error { return ErrInputLimit }

// InputValidator 在网络请求前检查实际协议封装，资源由实现自行释放，不发起模型调用。
type InputValidator interface {
	ValidateInput(context.Context, Request) error
}
