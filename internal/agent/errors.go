package agent

import "errors"

var (
	ErrInvalid     = errors.New("执行参数无效")
	ErrNotFound    = errors.New("执行或步骤不存在")
	ErrBusy        = errors.New("执行并发已满或会话正在执行")
	ErrClosed      = errors.New("执行器已关闭")
	ErrOutputLimit = errors.New("模型内容超过字节上限")
	ErrStorage     = errors.New("执行数据持久化失败")
)
