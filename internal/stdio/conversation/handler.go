package conversation

import (
	"context"

	domain "github.com/xiaoyu98628/callum/internal/conversation"
	"github.com/xiaoyu98628/callum/internal/conversation/service"
	"github.com/xiaoyu98628/callum/internal/stdio/protocol"
)

// Handler 转换对话接口；accepted 登记本连接发起的执行。
type Handler struct {
	service  *service.Service
	accepted func(string) error
}

func New(s *service.Service, accepted func(string) error) *Handler {
	return &Handler{service: s, accepted: accepted}
}

func (h *Handler) Handle(ctx context.Context, req protocol.Request, reply func(any, *protocol.Error) error) error {
	if req.Method == "conversation.compact" {
		return h.compact(ctx, req, reply)
	}
	if req.Method == "conversation.send" || req.Method == "conversation.resend" {
		return h.send(ctx, req, reply)
	}
	var value any
	var err error
	switch req.Method {
	case "conversation.context_usage":
		var p domain.ContextUsageRequest
		if rpcErr := protocol.DecodeParams(req.Params, &p); rpcErr != nil {
			return reply(nil, rpcErr)
		}
		value, err = h.service.ContextUsage(ctx, p)
	case "conversation.context_versions":
		var p struct {
			ConversationID string `json:"conversation_id"`
			Limit          int    `json:"limit,omitempty"`
			Offset         int    `json:"offset,omitempty"`
		}
		if rpcErr := protocol.DecodeParams(req.Params, &p); rpcErr != nil {
			return reply(nil, rpcErr)
		}
		value, err = h.service.ContextVersions(ctx, p.ConversationID, p.Limit, p.Offset)
	case "conversation.context_version":
		var p struct {
			ConversationID string `json:"conversation_id"`
			ID             string `json:"id"`
		}
		if rpcErr := protocol.DecodeParams(req.Params, &p); rpcErr != nil {
			return reply(nil, rpcErr)
		}
		value, err = h.service.ContextVersion(ctx, p.ConversationID, p.ID)
	case "conversation.list":
		var p domain.Filter
		if rpcErr := protocol.DecodeParams(req.Params, &p); rpcErr != nil {
			return reply(nil, rpcErr)
		}
		value, err = h.service.List(ctx, p)
	case "conversation.get", "conversation.delete", "conversation.restore":
		var p struct {
			ID string `json:"id"`
		}
		if rpcErr := protocol.DecodeParams(req.Params, &p); rpcErr != nil {
			return reply(nil, rpcErr)
		}
		if req.Method == "conversation.get" {
			value, err = h.service.Get(ctx, p.ID)
		} else {
			value, err = h.service.Update(ctx, p.ID, domain.Update{Deleted: new(req.Method == "conversation.delete")})
		}
	case "conversation.update":
		var p struct {
			ID string `json:"id"`
			domain.Update
		}
		if rpcErr := protocol.DecodeParams(req.Params, &p); rpcErr != nil {
			return reply(nil, rpcErr)
		}
		value, err = h.service.Update(ctx, p.ID, p.Update)
	case "conversation.messages":
		var p struct {
			ConversationID string `json:"conversation_id"`
			BeforeSequence int64  `json:"before_sequence,omitempty"`
			Limit          int    `json:"limit,omitempty"`
		}
		if rpcErr := protocol.DecodeParams(req.Params, &p); rpcErr != nil {
			return reply(nil, rpcErr)
		}
		value, err = h.service.Messages(ctx, p.ConversationID, p.BeforeSequence, p.Limit)
	default:
		return reply(nil, protocol.MethodNotFound())
	}
	return reply(value, publicError(err))
}

func (h *Handler) send(ctx context.Context, req protocol.Request, reply func(any, *protocol.Error) error) error {
	var p domain.Send
	if err := protocol.DecodeParams(req.Params, &p); err != nil {
		return reply(nil, err)
	}
	if (req.Method == "conversation.resend" && p.MessageID == "") || (req.Method == "conversation.send" && p.MessageID != "") {
		return reply(nil, protocol.InvalidParams())
	}
	replied := false
	err := h.service.Send(ctx, p, func(ex domain.Execution) error {
		if h.accepted != nil {
			if err := h.accepted(ex.Run.ID); err != nil {
				return err
			}
		}
		replied = true
		return reply(map[string]any{"conversation_id": ex.Conversation.ID, "run_id": ex.Run.ID, "user_message_id": ex.User.ID, "message_id": ex.Assistant.ID, "removed_message_ids": ex.RemovedMessageIDs}, nil)
	})
	if err != nil {
		if replied {
			return err
		}
		return reply(nil, publicError(err))
	}
	return nil
}
