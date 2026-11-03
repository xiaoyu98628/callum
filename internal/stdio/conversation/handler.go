package conversation

import (
	"context"

	domain "github.com/xiaoyu98628/callum/internal/conversation"
	"github.com/xiaoyu98628/callum/internal/conversation/service"
	"github.com/xiaoyu98628/callum/internal/run"
	"github.com/xiaoyu98628/callum/internal/stdio/protocol"
)

// Handler 负责对话接口和同步变更通知，emit 的发送失败必须取消通信上下文。
type Handler struct {
	service *service.Service
	emit    func(run.Event) error
}

func New(s *service.Service, emit func(run.Event) error) *Handler {
	return &Handler{service: s, emit: emit}
}

func (h *Handler) Handle(ctx context.Context, req protocol.Request, reply func(any, *protocol.Error) error) error {
	if req.Method == "conversation.send" || req.Method == "conversation.resend" {
		return h.send(ctx, req, reply)
	}
	var value any
	var err error
	changed := false
	switch req.Method {
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
			changed = true
			value, err = h.service.Update(ctx, p.ID, domain.Change{Deleted: new(req.Method == "conversation.delete")}, "")
		}
	case "conversation.update":
		var p struct {
			ID      string `json:"id"`
			ModelID string `json:"model_id,omitempty"`
			domain.Change
		}
		if rpcErr := protocol.DecodeParams(req.Params, &p); rpcErr != nil {
			return reply(nil, rpcErr)
		}
		changed = true
		value, err = h.service.Update(ctx, p.ID, p.Change, p.ModelID)
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
	if err := reply(value, publicError(err)); err != nil {
		return err
	}
	if changed && err == nil {
		conv := value.(domain.Conversation)
		return h.emit(run.Event{Type: "conversation.updated", ConversationID: conv.ID, Data: conv})
	}
	return nil
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
