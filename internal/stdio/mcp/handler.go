package mcp

import (
	"context"
	"errors"

	"github.com/xiaoyu98628/callum/internal/credentials"
	service "github.com/xiaoyu98628/callum/internal/mcp"
	"github.com/xiaoyu98628/callum/internal/settings"
	"github.com/xiaoyu98628/callum/internal/stdio/protocol"
)

type Handler struct{ service *service.Service }

func New(s *service.Service) *Handler { return &Handler{service: s} }

func (h *Handler) Handle(ctx, owner context.Context, req protocol.Request) (any, *protocol.Error) {
	var result any
	var err error
	switch req.Method {
	case "mcp.list":
		if e := protocol.DecodeParams(req.Params, &struct{}{}); e != nil {
			return nil, e
		}
		result, err = h.service.List(ctx)
	case "mcp.save":
		var in struct {
			Revision  int64               `json:"revision"`
			Server    settings.MCPServer  `json:"server"`
			Secrets   service.SecretInput `json:"secrets"`
			Confirmed bool                `json:"confirmed"`
		}
		if e := protocol.DecodeParams(req.Params, &in); e != nil {
			return nil, e
		}
		var id string
		id, err = h.service.Save(ctx, in.Revision, in.Server, in.Secrets, in.Confirmed)
		result = map[string]string{"id": id}
	case "mcp.delete":
		var in struct {
			ID       string `json:"id"`
			Revision int64  `json:"revision"`
		}
		if e := protocol.DecodeParams(req.Params, &in); e != nil {
			return nil, e
		}
		err = h.service.Delete(ctx, in.Revision, in.ID)
		result = true
	case "mcp.test":
		var in struct {
			Revision  int64  `json:"revision"`
			ID        string `json:"id"`
			Confirmed bool   `json:"confirmed"`
		}
		if e := protocol.DecodeParams(req.Params, &in); e != nil {
			return nil, e
		}
		result, err = h.service.Test(ctx, in.ID, in.Confirmed, in.Revision)
	case "mcp.oauth.start", "mcp.oauth.status", "mcp.oauth.cancel", "mcp.disconnect":
		var in struct {
			ID string `json:"id"`
		}
		if e := protocol.DecodeParams(req.Params, &in); e != nil {
			return nil, e
		}
		switch req.Method {
		case "mcp.oauth.start":
			result, err = h.service.StartOAuth(ctx, owner, in.ID)
		case "mcp.oauth.status", "mcp.oauth.cancel":
			result, err = h.service.OAuthState(in.ID, req.Method == "mcp.oauth.cancel")
		case "mcp.disconnect":
			err = h.service.Disconnect(ctx, in.ID)
			result = true
		}
	default:
		return nil, protocol.MethodNotFound()
	}
	if err != nil {
		return nil, publicError(err)
	}
	return result, nil
}

func publicError(err error) *protocol.Error {
	for _, item := range []struct {
		kind error
		code int
	}{
		{settings.ErrInvalid, -32602}, {settings.ErrConflict, 3010}, {service.ErrNotFound, 3011},
		{service.ErrAuthorize, 3012}, {service.ErrCallback, 3013}, {service.ErrOAuth, 3014},
		{service.ErrChanged, 3015}, {credentials.ErrUnavailable, 3016}, {service.ErrConnection, 3017},
		{service.ErrClientRegistration, 3018},
		{service.ErrOAuthScope, 3019}, {service.ErrOAuthScopeMismatch, 3020},
	} {
		if errors.Is(err, item.kind) {
			return &protocol.Error{Code: item.code, Message: item.kind.Error()}
		}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return &protocol.Error{Code: 3017, Message: "MCP 请求超时"}
	}
	return &protocol.Error{Code: -32603, Message: "MCP 操作失败"}
}
