package inference

import "errors"

var (
	ErrInvalidRequest = errors.New("模型调用参数无效")
	ErrBusy           = errors.New("模型调用已达到并发上限")
	ErrClosed         = errors.New("执行管理器已关闭")
	ErrUnsupported    = errors.New("当前适配器不支持选中的模型功能")
	ErrOutputLimit    = errors.New("模型输出超过大小限制")
)
