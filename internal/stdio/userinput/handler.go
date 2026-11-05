package userinput

import (
	"context"
	"errors"
	"github.com/xiaoyu98628/callum/internal/agent"
	"github.com/xiaoyu98628/callum/internal/agent/service"
	"github.com/xiaoyu98628/callum/internal/stdio/protocol"
)

type Handler struct{ engine *service.Engine }

func New(engine *service.Engine) *Handler { return &Handler{engine: engine} }
func (h *Handler) Handle(ctx context.Context, req protocol.Request) (any, *protocol.Error) {
	var value any
	var err error
	switch req.Method {
	case "user_input.get":
		var p struct {
			ID string `json:"id"`
		}
		if failure := protocol.DecodeParams(req.Params, &p); failure != nil {
			return nil, failure
		}
		value, err = h.engine.GetInput(ctx, p.ID)
	case "user_input.list":
		var p struct {
			RunID  string `json:"run_id"`
			Status string `json:"status"`
			Limit  int    `json:"limit"`
			Offset int    `json:"offset"`
		}
		if failure := protocol.DecodeParams(req.Params, &p); failure != nil {
			return nil, failure
		}
		if p.Limit == 0 {
			p.Limit = 100
		}
		if p.Status != "" && p.Status != "pending" {
			return nil, protocol.InvalidParams()
		}
		value, err = h.engine.Inputs(ctx, p.RunID, p.Status == "pending", p.Limit, p.Offset)
	case "user_input.submit":
		var p struct {
			ID     string            `json:"input_id"`
			RunID  string            `json:"run_id"`
			Answer agent.InputAnswer `json:"answer"`
		}
		if failure := protocol.DecodeParams(req.Params, &p); failure != nil {
			return nil, failure
		}
		value, err = h.engine.SubmitInput(ctx, p.ID, p.RunID, p.Answer)
	default:
		return nil, protocol.MethodNotFound()
	}
	if err == nil {
		return value, nil
	}
	switch {
	case errors.Is(err, agent.ErrInvalid):
		return nil, protocol.InvalidParams()
	case errors.Is(err, agent.ErrNotFound):
		return nil, &protocol.Error{Code: 1001, Message: "待回复问题不存在"}
	case errors.Is(err, agent.ErrConflict):
		return nil, &protocol.Error{Code: 1003, Message: "问题状态已变化，请重新核对"}
	default:
		return nil, &protocol.Error{Code: -32603, Message: "用户回复处理失败"}
	}
}
