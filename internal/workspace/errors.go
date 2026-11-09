package workspace

import "errors"

var (
	ErrInvalid  = errors.New("工作区参数无效")
	ErrNotFound = errors.New("工作区不存在")
	ErrBusy     = errors.New("工作区存在未结束执行")
	ErrConflict = errors.New("工作区状态冲突")
	ErrChanged  = errors.New("工作区文件夹已被修改")
)
