package stdio

import (
	"context"

	"github.com/xiaoyu98628/callum/internal/inference"
	"github.com/xiaoyu98628/callum/internal/model"
	"github.com/xiaoyu98628/callum/internal/model/provider"
)

func (s *Server) invoke(ctx context.Context, c *Connection, req request) error {
	var p struct {
		ModelID  string `json:"model_id"`
		Stream   *bool  `json:"stream"`
		Messages []struct {
			Role    model.Role `json:"role"`
			Content *string    `json:"content"`
		} `json:"messages"`
	}
	if err := decodeParams(req.Params, &p); err != nil {
		return c.reply(req, nil, err)
	}
	if !provider.ValidID(p.ModelID) {
		return c.reply(req, nil, invalidParams())
	}
	in := inference.Request{ModelID: p.ModelID, Stream: true}
	if p.Stream != nil {
		in.Stream = *p.Stream
	}
	for _, message := range p.Messages {
		if message.Content == nil {
			return c.reply(req, nil, invalidParams())
		}
		in.Messages = append(in.Messages, model.Message{Role: message.Role, Content: *message.Content})
	}
	err := s.runs.Start(ctx, in, func(id string) error { return c.reply(req, map[string]any{"run_id": id}, nil) })
	if err != nil {
		return c.reply(req, nil, publicError(err, true))
	}
	return nil
}

func (s *Server) catalog(ctx context.Context, req request) (any, *rpcError) {
	switch req.Method {
	case "provider.create", "provider.update":
		return s.saveProvider(ctx, req)
	case "provider.get", "provider.delete":
		var p idParams
		if err := decodeParams(req.Params, &p); err != nil {
			return nil, err
		}
		if !provider.ValidID(p.ID) {
			return nil, invalidParams()
		}
		if req.Method == "provider.delete" {
			if err := s.providers.DeleteProvider(ctx, p.ID); err != nil {
				return nil, publicError(err, false)
			}
			return idResult{p.ID}, nil
		}
		row, err := s.providers.GetProvider(ctx, p.ID)
		if err != nil {
			return nil, publicError(err, false)
		}
		return viewProvider(row), nil
	case "provider.list":
		if err := decodeParams(req.Params, &struct{}{}); err != nil {
			return nil, err
		}
		rows, err := s.providers.ListProviders(ctx)
		if err != nil {
			return nil, publicError(err, false)
		}
		out := make([]providerView, 0, len(rows))
		for _, row := range rows {
			out = append(out, viewProvider(row))
		}
		return out, nil
	case "model.create", "model.update":
		return s.saveModel(ctx, req)
	case "model.get", "model.delete":
		var p idParams
		if err := decodeParams(req.Params, &p); err != nil {
			return nil, err
		}
		if !provider.ValidID(p.ID) {
			return nil, invalidParams()
		}
		if req.Method == "model.delete" {
			if err := s.providers.DeleteModel(ctx, p.ID); err != nil {
				return nil, publicError(err, false)
			}
			return idResult{p.ID}, nil
		}
		row, err := s.providers.GetModel(ctx, p.ID)
		if err != nil {
			return nil, publicError(err, false)
		}
		return viewModel(row), nil
	case "model.list":
		var p struct {
			SelectableOnly bool `json:"selectable_only"`
		}
		if err := decodeParams(req.Params, &p); err != nil {
			return nil, err
		}
		rows, err := s.providers.ListModels(ctx, p.SelectableOnly)
		if err != nil {
			return nil, publicError(err, false)
		}
		out := make([]modelView, 0, len(rows))
		for _, row := range rows {
			out = append(out, viewModel(row))
		}
		return out, nil
	case "model.set_enabled":
		var p struct {
			ID      string `json:"id"`
			Enabled *bool  `json:"enabled"`
		}
		if err := decodeParams(req.Params, &p); err != nil {
			return nil, err
		}
		if !provider.ValidID(p.ID) || p.Enabled == nil {
			return nil, invalidParams()
		}
		if err := s.providers.SetModelEnabled(ctx, p.ID, *p.Enabled); err != nil {
			return nil, publicError(err, false)
		}
		return idResult{p.ID}, nil
	default:
		return nil, &rpcError{Code: -32601, Message: "接口不存在"}
	}
}

func (s *Server) saveProvider(ctx context.Context, req request) (any, *rpcError) {
	var p struct {
		ID string `json:"id"`
		providerFields
	}
	if err := decodeParams(req.Params, &p); err != nil {
		return nil, err
	}
	in, rpcErr := p.input()
	if rpcErr != nil {
		return nil, rpcErr
	}
	if req.Method == "provider.create" {
		if p.ID != "" {
			return nil, invalidParams()
		}
		id, err := s.providers.CreateProvider(ctx, in)
		if err != nil {
			return nil, publicError(err, false)
		}
		return idResult{id}, nil
	}
	if !provider.ValidID(p.ID) {
		return nil, invalidParams()
	}
	if err := s.providers.UpdateProvider(ctx, p.ID, in); err != nil {
		return nil, publicError(err, false)
	}
	return idResult{p.ID}, nil
}

func (s *Server) saveModel(ctx context.Context, req request) (any, *rpcError) {
	var p struct {
		ID string `json:"id"`
		modelFields
	}
	if err := decodeParams(req.Params, &p); err != nil {
		return nil, err
	}
	in, rpcErr := p.input()
	if rpcErr != nil {
		return nil, rpcErr
	}
	if req.Method == "model.create" {
		if p.ID != "" {
			return nil, invalidParams()
		}
		id, err := s.providers.CreateModel(ctx, in)
		if err != nil {
			return nil, publicError(err, false)
		}
		return idResult{id}, nil
	}
	if !provider.ValidID(p.ID) {
		return nil, invalidParams()
	}
	if err := s.providers.UpdateModel(ctx, p.ID, in); err != nil {
		return nil, publicError(err, false)
	}
	return idResult{p.ID}, nil
}
