package catalog_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	if err != nil || len(list.Items) != 1 || !list.Items[0].HasAPIKey {
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
		Capabilities: map[string]bool{"reasoning": true, "tools": false}, ModelType: []string{"text", "image"},
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
	if err != nil || len(models.Items) != 0 {
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

func TestCatalogPagesKeepStableCursorAndByteBounds(t *testing.T) {
	db, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "data", "callum.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	service, err := newService(db.DB)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := service.CreateProvider(t.Context(), provider.ProviderInput{Name: "p", Protocol: "chatcompletions", BaseURL: "https://example.invalid"})
	if err != nil {
		t.Fatal(err)
	}
	for i := range 501 {
		_, err := service.CreateModel(t.Context(), provider.ModelInput{ProviderID: pid, DisplayName: "模型", RemoteModelID: fmt.Sprint(i), Enabled: i%2 == 0})
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, only := range []bool{false, true} {
		after, count := "", 0
		for {
			page, err := service.ListModels(t.Context(), only, provider.ListOptions{AfterID: after, Limit: 200})
			if err != nil {
				t.Fatal(err)
			}
			for _, row := range page.Items {
				if row.ID <= after || (only && !row.Enabled) {
					t.Fatal("游标或筛选错误")
				}
				after = row.ID
				count++
			}
			if !page.HasMore {
				if page.NextAfterID != "" {
					t.Fatal("末页有游标")
				}
				break
			}
			if page.NextAfterID != after || len(page.Items) == 0 {
				t.Fatal("分页没有推进")
			}
		}
		want := 501
		if only {
			want = 251
		}
		if count != want {
			t.Fatalf("读取 %d，期望 %d", count, want)
		}
	}
	for _, options := range []provider.ListOptions{{Limit: -1}, {Limit: 201}, {AfterID: "invalid"}} {
		if _, err := service.ListModels(t.Context(), false, options); !errors.Is(err, provider.ErrInvalidConfig) {
			t.Fatalf("接受错误分页: %v", err)
		}
	}
	first, err := service.ListModels(t.Context(), false, provider.ListOptions{Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	cursor := first.NextAfterID
	if err := service.DeleteModel(t.Context(), cursor); err != nil {
		t.Fatal(err)
	}
	next, err := service.ListModels(t.Context(), false, provider.ListOptions{AfterID: cursor, Limit: 1})
	if err != nil || len(next.Items) != 1 || next.Items[0].ID <= cursor {
		t.Fatalf("删除游标后分页漂移: %+v %v", next, err)
	}
	// 单条配置合法，但合计超过存储页预算时仍可按游标读完，不默默截断。
	for i := range 12 {
		if _, err := service.CreateProvider(t.Context(), provider.ProviderInput{Name: strings.Repeat("中", 70000), Protocol: "chatcompletions", BaseURL: fmt.Sprintf("https://example.invalid/%d", i)}); err != nil {
			t.Fatal(err)
		}
	}
	page, err := service.ListProviders(t.Context(), provider.ListOptions{Limit: 200})
	if err != nil || !page.HasMore || len(page.Items) >= 13 {
		t.Fatalf("字节预算未生效: %d %v", len(page.Items), err)
	}
	total := len(page.Items)
	for page.HasMore {
		page, err = service.ListProviders(t.Context(), provider.ListOptions{AfterID: page.NextAfterID, Limit: 200})
		if err != nil {
			t.Fatal(err)
		}
		total += len(page.Items)
	}
	if total != 13 {
		t.Fatalf("字节分片丢失记录: %d", total)
	}
}
