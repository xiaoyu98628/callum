package conversation

import (
	"context"

	domain "github.com/xiaoyu98628/callum/internal/conversation"
	"github.com/xiaoyu98628/callum/internal/stdio/protocol"
)

func (h *Handler) compact(ctx context.Context, req protocol.Request, reply func(any, *protocol.Error) error) error {
	var input domain.Compact
	if err := protocol.DecodeParams(req.Params, &input); err != nil {
		return reply(nil, err)
	}
	replied := false
	err := h.service.Compact(ctx, input, func(ex domain.Execution) error {
		if h.accepted != nil {
			if err := h.accepted(ex.Run.ID); err != nil {
				return err
			}
		}
		replied = true
		return reply(map[string]string{"conversation_id": ex.Conversation.ID, "run_id": ex.Run.ID}, nil)
	})
	if err != nil {
		if replied {
			return err
		}
		return reply(nil, publicError(err))
	}
	return nil
}
