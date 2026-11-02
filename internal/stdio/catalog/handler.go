package catalog

import (
	"context"

	"github.com/xiaoyu98628/callum/internal/model/provider"
	providerservice "github.com/xiaoyu98628/callum/internal/model/provider/service"
	"github.com/xiaoyu98628/callum/internal/stdio/protocol"
)

// Handler 将厂商和模型配置接口转换为业务调用，不持有数据库或标准流。
type Handler struct{ providers *providerservice.Service }

// New 使用已初始化的非 nil 配置服务创建接口处理器，依赖由调用方持有。
func New(providers *providerservice.Service) *Handler { return &Handler{providers: providers} }

// Handle 转换配置接口参数并调用配置业务服务。
func (s *Handler) Handle(ctx context.Context, req protocol.Request) (any, *protocol.Error) {
	switch req.Method {
	case "provider.create", "provider.update":
		return s.saveProvider(ctx, req)
	case "provider.get", "provider.delete":
		var p idParams
		if err := protocol.DecodeParams(req.Params, &p); err != nil {
			return nil, err
		}
		if !provider.ValidID(p.ID) {
			return nil, protocol.InvalidParams()
		}
		if req.Method == "provider.delete" {
			if err := s.providers.DeleteProvider(ctx, p.ID); err != nil {
				return nil, publicError(err)
			}
			return idResult{p.ID}, nil
		}
		row, err := s.providers.GetProvider(ctx, p.ID)
		if err != nil {
			return nil, publicError(err)
		}
		return viewProvider(row), nil
	case "provider.list":
		if err := protocol.DecodeParams(req.Params, &struct{}{}); err != nil {
			return nil, err
		}
		rows, err := s.providers.ListProviders(ctx)
		if err != nil {
			return nil, publicError(err)
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
		if err := protocol.DecodeParams(req.Params, &p); err != nil {
			return nil, err
		}
		if !provider.ValidID(p.ID) {
			return nil, protocol.InvalidParams()
		}
		if req.Method == "model.delete" {
			if err := s.providers.DeleteModel(ctx, p.ID); err != nil {
				return nil, publicError(err)
			}
			return idResult{p.ID}, nil
		}
		row, err := s.providers.GetModel(ctx, p.ID)
		if err != nil {
			return nil, publicError(err)
		}
		return viewModel(row), nil
	case "model.list":
		var p struct {
			SelectableOnly bool `json:"selectable_only"`
		}
		if err := protocol.DecodeParams(req.Params, &p); err != nil {
			return nil, err
		}
		rows, err := s.providers.ListModels(ctx, p.SelectableOnly)
		if err != nil {
			return nil, publicError(err)
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
		if err := protocol.DecodeParams(req.Params, &p); err != nil {
			return nil, err
		}
		if !provider.ValidID(p.ID) || p.Enabled == nil {
			return nil, protocol.InvalidParams()
		}
		if err := s.providers.SetModelEnabled(ctx, p.ID, *p.Enabled); err != nil {
			return nil, publicError(err)
		}
		return idResult{p.ID}, nil
	default:
		return nil, &protocol.Error{Code: -32601, Message: "接口不存在"}
	}
}

func (s *Handler) saveProvider(ctx context.Context, req protocol.Request) (any, *protocol.Error) {
	var p struct {
		ID string `json:"id"`
		providerFields
	}
	if err := protocol.DecodeParams(req.Params, &p); err != nil {
		return nil, err
	}
	in, rpcErr := p.input()
	if rpcErr != nil {
		return nil, rpcErr
	}
	if req.Method == "provider.create" {
		if p.ID != "" {
			return nil, protocol.InvalidParams()
		}
		id, err := s.providers.CreateProvider(ctx, in)
		if err != nil {
			return nil, publicError(err)
		}
		return idResult{id}, nil
	}
	if !provider.ValidID(p.ID) {
		return nil, protocol.InvalidParams()
	}
	if err := s.providers.UpdateProvider(ctx, p.ID, in); err != nil {
		return nil, publicError(err)
	}
	return idResult{p.ID}, nil
}

func (s *Handler) saveModel(ctx context.Context, req protocol.Request) (any, *protocol.Error) {
	var p struct {
		ID string `json:"id"`
		modelFields
	}
	if err := protocol.DecodeParams(req.Params, &p); err != nil {
		return nil, err
	}
	in, rpcErr := p.input()
	if rpcErr != nil {
		return nil, rpcErr
	}
	if req.Method == "model.create" {
		if p.ID != "" {
			return nil, protocol.InvalidParams()
		}
		id, err := s.providers.CreateModel(ctx, in)
		if err != nil {
			return nil, publicError(err)
		}
		return idResult{id}, nil
	}
	if !provider.ValidID(p.ID) {
		return nil, protocol.InvalidParams()
	}
	if err := s.providers.UpdateModel(ctx, p.ID, in); err != nil {
		return nil, publicError(err)
	}
	return idResult{p.ID}, nil
}
