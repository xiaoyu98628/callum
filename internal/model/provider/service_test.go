package provider

import (
	"context"
	"errors"
	"testing"
)

const testID = "01900000-0000-7000-8000-000000000001"

type repositoryStub struct {
	Repository
	createProvider func(context.Context, string, ProviderInput) error
	loadRuntime    func(context.Context, string) (RuntimeConfig, error)
	getModel       func(context.Context, string) (Model, error)
	listModels     func(context.Context, bool) ([]Model, error)
}

func (r repositoryStub) CreateProvider(ctx context.Context, id string, in ProviderInput) error {
	return r.createProvider(ctx, id, in)
}
func (r repositoryStub) LoadRuntime(ctx context.Context, id string) (RuntimeConfig, error) {
	return r.loadRuntime(ctx, id)
}
func (r repositoryStub) GetModel(ctx context.Context, id string) (Model, error) {
	return r.getModel(ctx, id)
}
func (r repositoryStub) ListModels(ctx context.Context, only bool) ([]Model, error) {
	return r.listModels(ctx, only)
}

func TestServiceValidatesBeforeStorageAndAssignsID(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	called := false
	storedID := ""
	service, err := NewService(repositoryStub{createProvider: func(got context.Context, id string, in ProviderInput) error {
		called = true
		storedID = id
		if got != ctx || !ValidID(id) || in.Name != "厂商" {
			t.Fatal("上下文、标识或配置未正确传给存储")
		}
		return nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateProvider(ctx, ProviderInput{}); !errors.Is(err, ErrInvalidConfig) || called {
		t.Fatalf("无效配置触达存储: %v", err)
	}
	id, err := service.CreateProvider(ctx, ProviderInput{Name: "厂商", Protocol: "chatcompletions", BaseURL: "https://example.invalid"})
	if err != nil || id != storedID || !called {
		t.Fatalf("创建失败: %s %v", id, err)
	}
	if _, err := NewService(nil); err == nil {
		t.Fatal("接受了空存储")
	}
}

func TestServiceEnforcesRuntimeRulesAndPreservesContext(t *testing.T) {
	runtime := RuntimeConfig{Model: Model{ID: testID, ModelInput: ModelInput{ProviderID: testID, DisplayName: "模型", RemoteModelID: "remote"}}, APIKey: "fake-service-key"}
	reads := 0
	service, err := NewService(repositoryStub{loadRuntime: func(ctx context.Context, id string) (RuntimeConfig, error) {
		reads++
		if err := ctx.Err(); err != nil {
			return RuntimeConfig{}, err
		}
		return runtime, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Resolve(t.Context(), "invalid"); !errors.Is(err, ErrInvalidConfig) || reads != 0 {
		t.Fatalf("无效标识触达存储: %v", err)
	}
	if _, err := service.Resolve(t.Context(), testID); !errors.Is(err, ErrDisabled) {
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
	if _, err := service.Resolve(t.Context(), testID); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("无效持久化配置未被检查: %v", err)
	}
}

func TestServiceValidatesReadModelsAndPreservesStorageError(t *testing.T) {
	bad := Model{ID: testID, ProviderID: testID, DisplayName: "m", RemoteModelID: "r", ContextWindow: new(int64(0))}
	storageErr := errors.New("存储失败")
	service, err := NewService(repositoryStub{
		getModel:       func(context.Context, string) (Model, error) { return bad, nil },
		listModels:     func(context.Context, bool) ([]Model, error) { return []Model{bad}, nil },
		createProvider: func(context.Context, string, ProviderInput) error { return storageErr },
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.GetModel(t.Context(), testID); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("查询未校验配置: %v", err)
	}
	if rows, err := service.ListModels(t.Context(), false); !errors.Is(err, ErrInvalidConfig) || rows != nil {
		t.Fatalf("返回了无效列表: %v", err)
	}
	if _, err := service.CreateProvider(t.Context(), ProviderInput{Name: "p", Protocol: "chatcompletions", BaseURL: "https://example.invalid"}); !errors.Is(err, storageErr) {
		t.Fatalf("存储错误链丢失: %v", err)
	}
}
