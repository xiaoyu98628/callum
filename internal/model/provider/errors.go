package provider

import "errors"

var (
	ErrNotFound      = errors.New("配置不存在")
	ErrConflict      = errors.New("模型标识重复")
	ErrReferenced    = errors.New("关联配置不存在或仍被引用")
	ErrInvalidConfig = errors.New("配置无效")
	ErrDisabled      = errors.New("模型已禁用")
)
