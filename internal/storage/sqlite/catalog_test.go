package sqlite_test

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/xiaoyu98628/callum/internal/config"
	"github.com/xiaoyu98628/callum/internal/model/provider"
	"github.com/xiaoyu98628/callum/internal/storage/sqlite"
)

func TestCatalogPersistenceAndConstraints(t *testing.T) {
	cfg, err := config.Load(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatal(err)
	}
	path := cfg.DatabasePath
	db, err := sqlite.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	initialDB := db
	t.Cleanup(func() {
		if err := initialDB.Close(); err != nil {
			t.Error(err)
		}
	})
	s, err := newService(db.DB)
	if err != nil {
		t.Fatal(err)
	}
	p := provider.ProviderInput{Name: "主账号", Protocol: "chatcompletions", BaseURL: "https://example.invalid/v1", APIKey: new("private-test-key")}
	pid, err := s.CreateProvider(t.Context(), p)
	if err != nil {
		t.Fatal(err)
	}
	list, err := s.ListProviders(t.Context())
	if err != nil || len(list) != 1 || !list[0].HasAPIKey {
		t.Fatalf("providers=%+v err=%v", list, err)
	}
	public, err := json.Marshal(list)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(public), "private-test-key") {
		t.Fatal("列表泄露密钥")
	}
	in := provider.ModelInput{ProviderID: pid, DisplayName: "日常助手", RemoteModelID: "remote-chat", Enabled: true,
		ContextWindow: new(int64(128000)), MaxOutputTokens: new(int64(8000)),
		Capabilities: map[string]bool{"reasoning": true, "tools": false}, ModelType: []string{"reasoning"},
		ReasoningLevels: []provider.ReasoningLevel{{DisplayName: "标准", RemoteValue: "medium"}}}
	id, err := s.CreateModel(t.Context(), in)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateModel(t.Context(), in); !errors.Is(err, provider.ErrConflict) {
		t.Fatalf("重复标识: %v", err)
	}
	if err := s.DeleteProvider(t.Context(), pid); !errors.Is(err, provider.ErrReferenced) {
		t.Fatalf("删除被引用厂商: %v", err)
	}
	bad := in
	bad.ProviderID = "01900000-0000-7000-8000-000000000001"
	if _, err := s.CreateModel(t.Context(), bad); !errors.Is(err, provider.ErrReferenced) {
		t.Fatalf("无效厂商: %v", err)
	}
	bad = in
	bad.ModelType = []string{"tools"}
	if _, err := s.CreateModel(t.Context(), bad); !errors.Is(err, provider.ErrInvalidConfig) {
		t.Fatalf("不支持的功能: %v", err)
	}
	p.APIKey = nil
	p.Name = "修改后的名称"
	if err := s.UpdateProvider(t.Context(), pid, p); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = sqlite.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	s, err = newService(db.DB)
	if err != nil {
		t.Fatal(err)
	}
	run, err := s.Resolve(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	if run.APIKey != "private-test-key" || !reflect.DeepEqual(run.Model.ModelInput, in) {
		t.Fatal("重开数据库后的配置不一致")
	}
	in.DisplayName = "更名"
	if err := s.UpdateModel(t.Context(), id, in); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetModel(t.Context(), id)
	if err != nil || got.DisplayName != "更名" {
		t.Fatalf("更新模型: %+v %v", got, err)
	}
	if err := s.SetModelEnabled(t.Context(), id, false); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Resolve(t.Context(), id); !errors.Is(err, provider.ErrDisabled) {
		t.Fatalf("禁用模型: %v", err)
	}
	models, err := s.ListModels(t.Context(), true)
	if err != nil || len(models) != 0 {
		t.Fatalf("可选模型: %+v %v", models, err)
	}
	p.APIKey = new("")
	if err := s.UpdateProvider(t.Context(), pid, p); err != nil {
		t.Fatal(err)
	}
	gotProvider, err := s.GetProvider(t.Context(), pid)
	if err != nil || gotProvider.HasAPIKey {
		t.Fatalf("清空凭据: %+v %v", gotProvider, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := s.GetModel(ctx, id); !errors.Is(err, context.Canceled) {
		t.Fatalf("取消: %v", err)
	}
	if err := s.DeleteModel(t.Context(), id); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteModel(t.Context(), id); !errors.Is(err, provider.ErrNotFound) {
		t.Fatalf("重复删除: %v", err)
	}
	if err := s.DeleteProvider(t.Context(), pid); err != nil {
		t.Fatal(err)
	}
}

func TestUnknownAndEmptyMetadata(t *testing.T) {
	db, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "data", "callum.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	}()
	s, err := newService(db.DB)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := s.CreateProvider(t.Context(), provider.ProviderInput{Name: "厂商", Protocol: "chatcompletions", BaseURL: "https://example.invalid"})
	if err != nil {
		t.Fatal(err)
	}
	in := provider.ModelInput{ProviderID: pid, DisplayName: "模型", RemoteModelID: "m", Enabled: true}
	id, err := s.CreateModel(t.Context(), in)
	if err != nil {
		t.Fatal(err)
	}
	m, err := s.GetModel(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	if m.Capabilities != nil || m.ReasoningLevels != nil || m.ContextWindow != nil || m.MaxOutputTokens != nil || m.ModelType == nil {
		t.Fatalf("未知元数据未保持为空: %+v", m)
	}
	in.Capabilities = map[string]bool{}
	in.ReasoningLevels = []provider.ReasoningLevel{}
	if err := s.UpdateModel(t.Context(), id, in); err != nil {
		t.Fatal(err)
	}
	m, err = s.GetModel(t.Context(), id)
	if err != nil || m.Capabilities == nil || m.ReasoningLevels == nil {
		t.Fatalf("空元数据未保持: %+v %v", m, err)
	}
}
