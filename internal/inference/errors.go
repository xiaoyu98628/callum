package inference

import (
	"errors"

	"github.com/xiaoyu98628/callum/internal/agent"
)

var (
	ErrInvalidRequest = errors.New("模型调用参数无效")
	ErrBusy           = agent.ErrBusy
	ErrClosed         = agent.ErrClosed
	ErrUnsupported    = errors.New("当前适配器不支持选中的模型功能")
	ErrOutputLimit    = agent.ErrOutputLimit
)
