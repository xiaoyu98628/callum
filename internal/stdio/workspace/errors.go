package workspace

import (
	"context"
	"errors"

	"github.com/xiaoyu98628/callum/internal/stdio/protocol"
	domain "github.com/xiaoyu98628/callum/internal/workspace"
)

func publicError(err error) *protocol.Error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, domain.ErrInvalid):
		return protocol.InvalidParams()
	case errors.Is(err, domain.ErrNotFound):
		return &protocol.Error{Code: 1001, Message: "工作区不存在"}
	case errors.Is(err, domain.ErrBusy):
		return &protocol.Error{Code: 1006, Message: "工作区有未结束执行"}
	case errors.Is(err, domain.ErrConflict):
		return &protocol.Error{Code: 1003, Message: "工作区路径已存在"}
	case errors.Is(err, domain.ErrChanged):
		return &protocol.Error{Code: 1003, Message: "工作区文件夹已更新，请重新打开项目编辑"}
	case errors.Is(err, context.Canceled):
		return &protocol.Error{Code: 1007, Message: "操作已取消"}
	case errors.Is(err, context.DeadlineExceeded):
		return &protocol.Error{Code: 1008, Message: "操作超时"}
	default:
		return &protocol.Error{Code: -32603, Message: "工作区操作失败"}
	}
}
