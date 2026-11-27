package catalog

import (
	"context"
	"encoding/json"

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
		var p provider.ListOptions
		if err := protocol.DecodeParams(req.Params, &p); err != nil {
			return nil, err
		}
		page, err := s.providers.ListProviders(ctx, p)
		if err != nil {
			return nil, publicError(err)
		}
		out := make([]providerView, 0, len(page.Items))
		for _, row := range page.Items {
			out = append(out, viewProvider(row))
		}
		return publicPage(out, page.HasMore, func(v providerView) string { return v.ID })
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
			provider.ListOptions
		}
		if err := protocol.DecodeParams(req.Params, &p); err != nil {
			return nil, err
		}
		page, err := s.providers.ListModels(ctx, p.SelectableOnly, p.ListOptions)
		if err != nil {
			return nil, publicError(err)
		}
		out := make([]modelView, 0, len(page.Items))
		for _, row := range page.Items {
			out = append(out, viewModel(row))
		}
		return publicPage(out, page.HasMore, func(v modelView) string { return v.ID })
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

// 实际序列化后再次限制一页的 JSON 字节，给响应包装和请求标识保留余量。
func publicPage[T any](items []T, more bool, itemID func(T) string) (any, *protocol.Error) {
	total := 0
	for i, item := range items {
		raw, err := json.Marshal(item)
		if err != nil {
			return nil, publicError(err)
		}
		if total+len(raw)+1 > 6<<20 {
			if i == 0 {
				return nil, publicError(provider.ErrInvalidConfig)
			}
			items, more = items[:i], true
			break
		}
		total += len(raw) + 1
	}
	page := provider.Page[T]{Items: items, HasMore: more}
	if more {
		page.NextAfterID = itemID(items[len(items)-1])
	}
	return page, nil
}
