package approval

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/xiaoyu98628/callum/internal/agent"
	"github.com/xiaoyu98628/callum/internal/agent/approval"
	"github.com/xiaoyu98628/callum/internal/stdio/protocol"
)

// Handler 只转换协议参数，决定校验和事件发布由审批业务服务负责。
type Handler struct{ service *approval.Service }

func New(service *approval.Service) *Handler { return &Handler{service: service} }
func (h *Handler) Handle(ctx context.Context, req protocol.Request) (any, *protocol.Error) {
	var value any
	var err error
	switch req.Method {
	case "approval.get":
		var p struct {
			ID string `json:"id"`
		}
		if rpcErr := protocol.DecodeParams(req.Params, &p); rpcErr != nil {
			return nil, rpcErr
		}
		value, err = h.service.Get(ctx, p.ID)
	case "approval.list":
		var p agent.ApprovalFilter
		if rpcErr := protocol.DecodeParams(req.Params, &p); rpcErr != nil {
			return nil, rpcErr
		}
		value, err = h.service.List(ctx, p)
	case "approval.decide":
		var p struct {
			ApprovalID string          `json:"approval_id"`
			Decision   string          `json:"decision"`
			Note       *string         `json:"decision_note,omitempty"`
			Scope      json.RawMessage `json:"scope,omitempty"`
		}
		if rpcErr := protocol.DecodeParams(req.Params, &p); rpcErr != nil {
			return nil, rpcErr
		}
		scope := agent.ApprovalOnce
		if len(p.Scope) > 0 {
			var selected *string
			if json.Unmarshal(p.Scope, &selected) != nil || selected == nil || (*selected != agent.ApprovalOnce && *selected != agent.ApprovalConversationDirectory) {
				return nil, protocol.InvalidParams()
			}
			scope = *selected
		}
		value, err = h.service.Decide(ctx, p.ApprovalID, p.Decision, p.Note, scope)
	case "approval.directory.list":
		var p struct {
			ConversationID string `json:"conversation_id"`
			Limit          int    `json:"limit,omitempty"`
			Offset         int    `json:"offset,omitempty"`
		}
		if rpcErr := protocol.DecodeParams(req.Params, &p); rpcErr != nil {
			return nil, rpcErr
		}
		value, err = h.service.ListDirectories(ctx, p.ConversationID, p.Limit, p.Offset)
	case "approval.directory.revoke":
		var p struct {
			ConversationID string `json:"conversation_id"`
			ID             string `json:"id"`
		}
		if rpcErr := protocol.DecodeParams(req.Params, &p); rpcErr != nil {
			return nil, rpcErr
		}
		value, err = h.service.RevokeDirectory(ctx, p.ConversationID, p.ID)
	default:
		return nil, protocol.MethodNotFound()
	}
	if err == nil {
		return value, nil
	}
	switch {
	case errors.Is(err, agent.ErrApprovalInvalid):
		return nil, protocol.InvalidParams()
	case errors.Is(err, agent.ErrNotFound):
		return nil, &protocol.Error{Code: 1001, Message: "审批不存在"}
	case errors.Is(err, agent.ErrConflict):
		return nil, &protocol.Error{Code: 1003, Message: "审批状态已变化，无法提交该决定"}
	default:
		return nil, &protocol.Error{Code: -32603, Message: "审批处理失败"}
	}
}
