package workspace

import (
	"context"

	"github.com/xiaoyu98628/callum/internal/stdio/protocol"
	domain "github.com/xiaoyu98628/callum/internal/workspace"
	"github.com/xiaoyu98628/callum/internal/workspace/service"
)

// Handler 转换工作区接口，业务服务及数据库由 app 持有。
type Handler struct{ service *service.Service }

func New(s *service.Service) *Handler { return &Handler{service: s} }

func (h *Handler) Handle(ctx context.Context, req protocol.Request) (any, *protocol.Error) {
	switch req.Method {
	case "workspace.create":
		var p struct {
			Name       string   `json:"name"`
			RootPath   string   `json:"root_path"`
			OtherPaths []string `json:"other_paths"`
		}
		if err := protocol.DecodeParams(req.Params, &p); err != nil {
			return nil, err
		}
		value, err := h.service.Create(ctx, p.Name, p.RootPath, p.OtherPaths...)
		return value, publicError(err)
	case "workspace.list":
		var p domain.Filter
		if err := protocol.DecodeParams(req.Params, &p); err != nil {
			return nil, err
		}
		value, err := h.service.List(ctx, p)
		return value, publicError(err)
	case "workspace.get", "workspace.delete", "workspace.restore":
		var p struct {
			ID string `json:"id"`
		}
		if err := protocol.DecodeParams(req.Params, &p); err != nil {
			return nil, err
		}
		if req.Method == "workspace.get" {
			value, err := h.service.Get(ctx, p.ID)
			return value, publicError(err)
		}
		value, err := h.service.Update(ctx, p.ID, domain.Change{Deleted: new(req.Method == "workspace.delete")})
		return value, publicError(err)
	case "workspace.update":
		var p struct {
			ID string `json:"id"`
			domain.Change
		}
		if err := protocol.DecodeParams(req.Params, &p); err != nil {
			return nil, err
		}
		value, err := h.service.Update(ctx, p.ID, p.Change)
		return value, publicError(err)
	default:
		return nil, protocol.MethodNotFound()
	}
}
