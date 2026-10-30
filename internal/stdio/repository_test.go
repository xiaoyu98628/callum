package stdio

import (
	"context"
	"slices"

	"github.com/xiaoyu98628/callum/internal/model/provider"
)

// memoryRepository 为协议测试提供存储替身，SQLite 约束与持久化由存储集成测试验证。
// 协议请求串行访问此替身，测试中的模型解析不访问其状态。
type memoryRepository struct {
	providers map[string]provider.ProviderInput
	models    map[string]provider.Model
}

func (r *memoryRepository) CreateProvider(_ context.Context, id string, in provider.ProviderInput) error {
	r.providers[id] = in
	return nil
}
func (r *memoryRepository) UpdateProvider(_ context.Context, id string, in provider.ProviderInput) error {
	old, ok := r.providers[id]
	if !ok {
		return provider.ErrNotFound
	}
	if in.APIKey == nil {
		in.APIKey = old.APIKey
	}
	r.providers[id] = in
	return nil
}
func (r *memoryRepository) GetProvider(_ context.Context, id string) (provider.Provider, error) {
	in, ok := r.providers[id]
	if !ok {
		return provider.Provider{}, provider.ErrNotFound
	}
	return provider.Provider{ID: id, Name: in.Name, Protocol: in.Protocol, BaseURL: in.BaseURL, HasAPIKey: in.APIKey != nil && *in.APIKey != ""}, nil
}
func (r *memoryRepository) ListProviders(ctx context.Context) ([]provider.Provider, error) {
	out := make([]provider.Provider, 0, len(r.providers))
	for id := range r.providers {
		p, err := r.GetProvider(ctx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	slices.SortFunc(out, func(a, b provider.Provider) int {
		if a.ID < b.ID {
			return -1
		}
		if a.ID > b.ID {
			return 1
		}
		return 0
	})
	return out, nil
}
func (r *memoryRepository) DeleteProvider(_ context.Context, id string) error {
	if _, ok := r.providers[id]; !ok {
		return provider.ErrNotFound
	}
	for _, m := range r.models {
		if m.ProviderID == id {
			return provider.ErrReferenced
		}
	}
	delete(r.providers, id)
	return nil
}
func (r *memoryRepository) CreateModel(_ context.Context, id string, in provider.ModelInput) error {
	if _, ok := r.providers[in.ProviderID]; !ok {
		return provider.ErrReferenced
	}
	for _, m := range r.models {
		if m.ProviderID == in.ProviderID && m.RemoteModelID == in.RemoteModelID {
			return provider.ErrConflict
		}
	}
	if in.ModelType == nil {
		in.ModelType = []string{}
	}
	r.models[id] = provider.Model{ID: id, ModelInput: in}
	return nil
}
func (r *memoryRepository) UpdateModel(_ context.Context, id string, in provider.ModelInput) error {
	if _, ok := r.models[id]; !ok {
		return provider.ErrNotFound
	}
	if in.ModelType == nil {
		in.ModelType = []string{}
	}
	r.models[id] = provider.Model{ID: id, ModelInput: in}
	return nil
}
func (r *memoryRepository) GetModel(_ context.Context, id string) (provider.Model, error) {
	m, ok := r.models[id]
	if !ok {
		return m, provider.ErrNotFound
	}
	return m, nil
}
func (r *memoryRepository) ListModels(_ context.Context, enabledOnly bool) ([]provider.Model, error) {
	out := make([]provider.Model, 0, len(r.models))
	for _, m := range r.models {
		if !enabledOnly || m.Enabled {
			out = append(out, m)
		}
	}
	slices.SortFunc(out, func(a, b provider.Model) int {
		if a.ID < b.ID {
			return -1
		}
		if a.ID > b.ID {
			return 1
		}
		return 0
	})
	return out, nil
}
func (r *memoryRepository) SetModelEnabled(_ context.Context, id string, enabled bool) error {
	m, ok := r.models[id]
	if !ok {
		return provider.ErrNotFound
	}
	m.Enabled = enabled
	r.models[id] = m
	return nil
}
func (r *memoryRepository) DeleteModel(_ context.Context, id string) error {
	if _, ok := r.models[id]; !ok {
		return provider.ErrNotFound
	}
	delete(r.models, id)
	return nil
}
func (r *memoryRepository) LoadRuntime(_ context.Context, id string) (provider.RuntimeConfig, error) {
	m, ok := r.models[id]
	if !ok {
		return provider.RuntimeConfig{}, provider.ErrNotFound
	}
	p := r.providers[m.ProviderID]
	key := ""
	if p.APIKey != nil {
		key = *p.APIKey
	}
	return provider.RuntimeConfig{Model: m, Protocol: p.Protocol, BaseURL: p.BaseURL, APIKey: key}, nil
}
