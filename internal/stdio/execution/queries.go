package execution

import (
	"context"
	"errors"

	"github.com/xiaoyu98628/callum/internal/agent"
	"github.com/xiaoyu98628/callum/internal/stdio/protocol"
)

func queryError(err error) *protocol.Error {
	converted := Error(err)
	if converted != nil && converted.Code == 1009 {
		return &protocol.Error{Code: -32603, Message: "执行数据查询失败"}
	}
	return converted
}

// Query 仅处理持久化执行查询；标识不能使用旧无历史调用的临时 run_id。
func (h *Handler) Query(ctx context.Context, req protocol.Request) (any, *protocol.Error) {
	if h.persistent == nil {
		return nil, protocol.MethodNotFound()
	}
	switch req.Method {
	case "run.get", "step.get":
		var p struct {
			ID string `json:"id"`
		}
		if err := protocol.DecodeParams(req.Params, &p); err != nil {
			return nil, err
		}
		if req.Method == "run.get" {
			value, err := h.persistent.Get(ctx, p.ID)
			return value, queryError(err)
		}
		value, err := h.persistent.GetStepSummary(ctx, p.ID)
		return value, queryError(err)
	case "run.list":
		var p struct {
			ConversationID string `json:"conversation_id"`
			Limit          int    `json:"limit,omitempty"`
			Offset         int    `json:"offset,omitempty"`
		}
		if err := protocol.DecodeParams(req.Params, &p); err != nil {
			return nil, err
		}
		if p.Limit == 0 {
			p.Limit = 100
		}
		value, err := h.persistent.List(ctx, p.ConversationID, p.Limit, p.Offset)
		return value, queryError(err)
	case "step.list":
		var p struct {
			RunID         string `json:"run_id"`
			AfterSequence int64  `json:"after_sequence,omitempty"`
			Limit         int    `json:"limit,omitempty"`
		}
		if err := protocol.DecodeParams(req.Params, &p); err != nil {
			return nil, err
		}
		if p.Limit == 0 {
			p.Limit = 100
		}
		rows, err := h.persistent.StepSummaries(ctx, p.RunID, p.AfterSequence, p.Limit)
		return rows, queryError(err)
	case "step.data":
		var p struct {
			StepID string `json:"step_id"`
			Field  string `json:"field"`
			Offset int    `json:"offset,omitempty"`
			Limit  int    `json:"limit,omitempty"`
		}
		if err := protocol.DecodeParams(req.Params, &p); err != nil {
			return nil, err
		}
		if p.Offset < 0 {
			return nil, protocol.InvalidParams()
		}
		if p.Limit == 0 {
			p.Limit = 64 << 10
		}
		if p.Limit < 4 || p.Limit > 64<<10 {
			return nil, protocol.InvalidParams()
		}
		data, err := h.persistent.StepData(ctx, p.StepID, p.Field, p.Offset, p.Limit)
		if errors.Is(err, agent.ErrStepReasoning) {
			return nil, &protocol.Error{Code: -32603, Message: agent.ErrStepReasoning.Error()}
		}
		if err != nil {
			return nil, queryError(err)
		}
		return map[string]any{"step_id": p.StepID, "field": p.Field, "present": data.Present, "offset": p.Offset,
			"next_offset": data.NextOffset, "total_bytes": data.TotalBytes, "done": data.NextOffset == data.TotalBytes, "fragment": data.Fragment}, nil
	default:
		return nil, protocol.MethodNotFound()
	}
}
