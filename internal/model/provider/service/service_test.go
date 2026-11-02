package service

import (
	"context"
	"errors"
	"testing"

	"github.com/xiaoyu98628/callum/internal/model/provider"
)

const testID = "01900000-0000-7000-8000-000000000001"

type repositoryStub struct {
	provider.Repository
	createProvider func(context.Context, string, provider.ProviderInput) error
	loadRuntime    func(context.Context, string) (provider.RuntimeConfig, error)
	getModel       func(context.Context, string) (provider.Model, error)
	listModels     func(context.Context, bool) ([]provider.Model, error)
}

func (r repositoryStub) CreateProvider(ctx context.Context, id string, in provider.ProviderInput) error {
	return r.createProvider(ctx, id, in)
}
func (r repositoryStub) LoadRuntime(ctx context.Context, id string) (provider.RuntimeConfig, error) {
	return r.loadRuntime(ctx, id)
}
func (r repositoryStub) GetModel(ctx context.Context, id string) (provider.Model, error) {
	return r.getModel(ctx, id)
}
func (r repositoryStub) ListModels(ctx context.Context, only bool) ([]provider.Model, error) {
	return r.listModels(ctx, only)
}

func TestServiceValidatesBeforeStorageAndAssignsID(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	called := false
	storedID := ""
	service, err := New(repositoryStub{createProvider: func(got context.Context, id string, in provider.ProviderInput) error {
		called = true
		storedID = id
		if got != ctx || !provider.ValidID(id) || in.Name != "厂商" {
			t.Fatal("上下文、标识或配置未正确传给存储")
		}
		return nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateProvider(ctx, provider.ProviderInput{}); !errors.Is(err, provider.ErrInvalidConfig) || called {
		t.Fatalf("无效配置触达存储: %v", err)
	}
	id, err := service.CreateProvider(ctx, provider.ProviderInput{Name: "厂商", Protocol: "chatcompletions", BaseURL: "https://example.invalid"})
	if err != nil || id != storedID || !called {
		t.Fatalf("创建失败: %s %v", id, err)
	}
	if _, err := New(nil); err == nil {
		t.Fatal("接受了空存储")
	}
}

func TestServiceEnforcesRuntimeRulesAndPreservesContext(t *testing.T) {
	runtime := provider.RuntimeConfig{Model: provider.Model{ID: testID, ModelInput: provider.ModelInput{ProviderID: testID, DisplayName: "模型", RemoteModelID: "remote"}}, APIKey: "fake-service-key"}
	reads := 0
	service, err := New(repositoryStub{loadRuntime: func(ctx context.Context, id string) (provider.RuntimeConfig, error) {
		reads++
		if err := ctx.Err(); err != nil {
			return provider.RuntimeConfig{}, err
		}
		return runtime, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Resolve(t.Context(), "invalid"); !errors.Is(err, provider.ErrInvalidConfig) || reads != 0 {
		t.Fatalf("无效标识触达存储: %v", err)
	}
	if _, err := service.Resolve(t.Context(), testID); !errors.Is(err, provider.ErrDisabled) {
		t.Fatalf("未阻止禁用模型: %v", err)
	}
	runtime.Model.Enabled = true
	got, err := service.Resolve(t.Context(), testID)
	if err != nil || got.APIKey != runtime.APIKey {
		t.Fatalf("有效运行配置丢失: %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := service.Resolve(ctx, testID); !errors.Is(err, context.Canceled) {
		t.Fatalf("取消信号丢失: %v", err)
	}
	runtime.Model.ModelType = []string{"tools"}
	if _, err := service.Resolve(t.Context(), testID); !errors.Is(err, provider.ErrInvalidConfig) {
		t.Fatalf("无效持久化配置未被检查: %v", err)
	}
}

func TestServiceValidatesReadModelsAndPreservesStorageError(t *testing.T) {
	bad := provider.Model{ID: testID, ProviderID: testID, DisplayName: "m", RemoteModelID: "r", ContextWindow: new(int64(0))}
	storageErr := errors.New("存储失败")
	service, err := New(repositoryStub{
		getModel:       func(context.Context, string) (provider.Model, error) { return bad, nil },
		listModels:     func(context.Context, bool) ([]provider.Model, error) { return []provider.Model{bad}, nil },
		createProvider: func(context.Context, string, provider.ProviderInput) error { return storageErr },
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.GetModel(t.Context(), testID); !errors.Is(err, provider.ErrInvalidConfig) {
		t.Fatalf("查询未校验配置: %v", err)
	}
	if rows, err := service.ListModels(t.Context(), false); !errors.Is(err, provider.ErrInvalidConfig) || rows != nil {
		t.Fatalf("返回了无效列表: %v", err)
	}
	if _, err := service.CreateProvider(t.Context(), provider.ProviderInput{Name: "p", Protocol: "chatcompletions", BaseURL: "https://example.invalid"}); !errors.Is(err, storageErr) {
		t.Fatalf("存储错误链丢失: %v", err)
	}
}
