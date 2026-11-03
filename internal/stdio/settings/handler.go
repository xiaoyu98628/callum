package settings

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/xiaoyu98628/callum/internal/settings"
	"github.com/xiaoyu98628/callum/internal/settings/service"
	"github.com/xiaoyu98628/callum/internal/stdio/protocol"
)

// Handler 转换设置接口参数，校验和持久化由设置业务服务负责。
type Handler struct{ service *service.Service }

func New(service *service.Service) *Handler { return &Handler{service: service} }

func (h *Handler) Handle(ctx context.Context, req protocol.Request) (any, *protocol.Error) {
	var value any
	var err error
	switch req.Method {
	case "settings.get":
		var p struct {
			Key string `json:"key"`
		}
		if rpcErr := protocol.DecodeParams(req.Params, &p); rpcErr != nil {
			return nil, rpcErr
		}
		value, err = h.service.Get(ctx, p.Key)
	case "settings.update":
		var p struct {
			Key      string          `json:"key"`
			Revision int64           `json:"revision"`
			Value    json.RawMessage `json:"value"`
		}
		if rpcErr := protocol.DecodeParams(req.Params, &p); rpcErr != nil {
			return nil, rpcErr
		}
		value, err = h.service.Update(ctx, p.Key, p.Revision, p.Value)
	default:
		return nil, protocol.MethodNotFound()
	}
	if err == nil {
		return value, nil
	}
	switch {
	case errors.Is(err, settings.ErrStorage):
		return nil, &protocol.Error{Code: -32603, Message: "设置处理失败"}
	case errors.Is(err, settings.ErrInvalid):
		return nil, protocol.InvalidParams()
	case errors.Is(err, settings.ErrConflict):
		return nil, &protocol.Error{Code: 1003, Message: "设置已被修改，请重新读取后保存"}
	case errors.Is(err, settings.ErrNotFound):
		return nil, &protocol.Error{Code: 1001, Message: "设置不存在"}
	default:
		return nil, &protocol.Error{Code: -32603, Message: "设置处理失败"}
	}
}
