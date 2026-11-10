package catalog

import (
	"context"
	"errors"

	"github.com/xiaoyu98628/callum/internal/model/provider"
	"github.com/xiaoyu98628/callum/internal/stdio/protocol"
)

// publicError 只输出受控描述，禁止将存储、网络和供应商错误链直接发给客户端。
func publicError(err error) *protocol.Error {
	e := &protocol.Error{Code: -32603, Message: "内部处理失败"}
	kind := "internal"
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		e.Code, e.Message, kind = 1008, "操作超时", "timeout"
	case errors.Is(err, context.Canceled):
		e.Code, e.Message, kind = 1007, "操作已取消", "cancelled"
	case errors.Is(err, provider.ErrInvalidConfig):
		return protocol.InvalidParams()
	case errors.Is(err, provider.ErrNotFound):
		e.Code, e.Message, kind = 1001, "配置不存在", "not_found"
	case errors.Is(err, provider.ErrConflict):
		e.Code, e.Message, kind = 1002, "同一厂商下的远端模型标识已存在", "conflict"
	case errors.Is(err, provider.ErrReferenced):
		e.Code, e.Message, kind = 1003, "关联配置不存在或仍被引用", "referenced"
	case errors.Is(err, provider.ErrDisabled):
		e.Code, e.Message, kind = 1004, "模型已禁用", "disabled"
	case errors.Is(err, provider.ErrCredentials):
		e.Code, e.Message, kind = 1012, "系统凭据库不可用或凭据无法解密，请解锁凭据库后重试。", "credentials_unavailable"
	}
	e.Data = map[string]any{"kind": kind}
	return e
}
