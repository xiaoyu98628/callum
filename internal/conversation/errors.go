package conversation

import "errors"

var (
	ErrInvalid  = errors.New("会话或消息参数无效")
	ErrNotFound = errors.New("会话或消息不存在")
	ErrBusy     = errors.New("会话有未结束执行")
	ErrInactive = errors.New("会话或工作区已归档或删除")
	ErrConflict = errors.New("消息已变化或不允许重发")
)
