package conversation

import (
	"errors"

	"github.com/xiaoyu98628/callum/internal/agent"
	domain "github.com/xiaoyu98628/callum/internal/conversation"
	"github.com/xiaoyu98628/callum/internal/stdio/execution"
	"github.com/xiaoyu98628/callum/internal/stdio/protocol"
)

func publicError(err error) *protocol.Error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, domain.ErrInvalid), errors.Is(err, agent.ErrInvalid):
		return protocol.InvalidParams()
	case errors.Is(err, domain.ErrNotFound), errors.Is(err, agent.ErrNotFound):
		return &protocol.Error{Code: 1001, Message: "会话、执行或消息不存在"}
	case errors.Is(err, domain.ErrBusy):
		return &protocol.Error{Code: 1006, Message: "该会话正在执行"}
	case errors.Is(err, domain.ErrInactive):
		return &protocol.Error{Code: 1003, Message: "会话或工作区已归档或删除"}
	case errors.Is(err, domain.ErrConflict):
		return &protocol.Error{Code: 1003, Message: "只能编辑最后一条用户消息并重发"}
	default:
		converted := execution.Error(err)
		if converted.Code == 1009 {
			return &protocol.Error{Code: -32603, Message: "会话操作失败"}
		}
		return converted
	}
}
