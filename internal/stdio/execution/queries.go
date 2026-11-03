package execution

import (
	"context"
	"unicode/utf8"

	"github.com/xiaoyu98628/callum/internal/model/provider"
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
		if !provider.ValidID(p.ID) {
			return nil, protocol.InvalidParams()
		}
		if req.Method == "run.get" {
			value, err := h.persistent.Get(ctx, p.ID)
			return value, queryError(err)
		}
		value, err := h.persistent.GetStep(ctx, p.ID)
		return summary(value), queryError(err)
	case "run.list":
		var p struct {
			ConversationID string `json:"conversation_id"`
			Limit          int    `json:"limit,omitempty"`
			Offset         int    `json:"offset,omitempty"`
		}
		if err := protocol.DecodeParams(req.Params, &p); err != nil {
			return nil, err
		}
		if !provider.ValidID(p.ConversationID) {
			return nil, protocol.InvalidParams()
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
		if !provider.ValidID(p.RunID) {
			return nil, protocol.InvalidParams()
		}
		if p.Limit == 0 {
			p.Limit = 100
		}
		rows, err := h.persistent.Steps(ctx, p.RunID, p.AfterSequence, p.Limit)
		if err != nil {
			return nil, queryError(err)
		}
		out := make([]StepSummary, 0, len(rows))
		for _, row := range rows {
			out = append(out, summary(row))
		}
		return out, nil
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
		if !provider.ValidID(p.StepID) || p.Offset < 0 {
			return nil, protocol.InvalidParams()
		}
		if p.Limit == 0 {
			p.Limit = 64 << 10
		}
		if p.Limit < 4 || p.Limit > 64<<10 {
			return nil, protocol.InvalidParams()
		}
		row, err := h.persistent.GetStep(ctx, p.StepID)
		if err != nil {
			return nil, queryError(err)
		}
		var raw []byte
		switch p.Field {
		case "output":
			raw = row.Output
		case "usage":
			raw = row.Usage
		case "definition":
			raw = row.Definition
		case "input":
			raw = row.Input
		default:
			return nil, protocol.InvalidParams()
		}
		if p.Offset > len(raw) || (p.Offset < len(raw) && !utf8.RuneStart(raw[p.Offset])) {
			return nil, protocol.InvalidParams()
		}
		end := min(len(raw), p.Offset+p.Limit)
		for end < len(raw) && !utf8.RuneStart(raw[end]) {
			end--
		}
		return map[string]any{"step_id": p.StepID, "field": p.Field, "present": raw != nil, "offset": p.Offset, "next_offset": end, "total_bytes": len(raw), "done": end == len(raw), "fragment": string(raw[p.Offset:end])}, nil
	default:
		return nil, protocol.MethodNotFound()
	}
}
