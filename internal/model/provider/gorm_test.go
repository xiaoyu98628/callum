package provider_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/xiaoyu98628/callum/internal/database"
	"github.com/xiaoyu98628/callum/internal/model/provider"
)

func TestWaitingForConnectionCanTimeout(t *testing.T) {
	db, err := database.Open(t.Context(), filepath.Join(t.TempDir(), "data", "callum.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	s, err := provider.NewStore(db.DB)
	if err != nil {
		t.Fatal(err)
	}
	pool, err := db.DB.DB()
	if err != nil {
		t.Fatal(err)
	}
	conn, err := pool.Conn(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := conn.Close(); err != nil && !errors.Is(err, sql.ErrConnDone) {
			t.Error(err)
		}
	}()
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	if _, err := s.ListProviders(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("等待连接时没有传递超时: %v", err)
	}
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ListProviders(t.Context()); err != nil {
		t.Fatalf("超时后连接池无法继续使用: %v", err)
	}
}

func TestGORMZeroValues(t *testing.T) {
	db, err := database.Open(t.Context(), filepath.Join(t.TempDir(), "data", "callum.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	}()
	s, err := provider.NewStore(db.DB)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := s.CreateProvider(t.Context(), provider.ProviderInput{Name: "厂商", Protocol: "chatcompletions", BaseURL: "https://example.invalid"})
	if err != nil {
		t.Fatal(err)
	}
	in := provider.ModelInput{ProviderID: pid, DisplayName: "模型", RemoteModelID: "remote-model", Enabled: false}
	id, err := s.CreateModel(t.Context(), in)
	if err != nil {
		t.Fatal(err)
	}
	m, err := s.GetModel(t.Context(), id)
	if err != nil || m.Enabled {
		t.Fatalf("显式 false 被默认值覆盖: %+v %v", m, err)
	}
	in.Enabled = true
	in.ContextWindow = new(int64(1000))
	in.MaxOutputTokens = new(int64(100))
	in.Capabilities = map[string]bool{"reasoning": true}
	in.ModelType = []string{"reasoning"}
	in.ReasoningLevels = []provider.ReasoningLevel{{DisplayName: "标准", RemoteValue: "medium"}}
	if err := s.UpdateModel(t.Context(), id, in); err != nil {
		t.Fatal(err)
	}
	in.Enabled = false
	in.ContextWindow = nil
	in.MaxOutputTokens = nil
	in.Capabilities = nil
	in.ModelType = nil
	in.ReasoningLevels = nil
	if err := s.UpdateModel(t.Context(), id, in); err != nil {
		t.Fatal(err)
	}
	m, err = s.GetModel(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	if m.Enabled || m.ContextWindow != nil || m.MaxOutputTokens != nil || m.Capabilities != nil || len(m.ModelType) != 0 || m.ModelType == nil || m.ReasoningLevels != nil {
		t.Fatalf("零值或 NULL 更新失败: %+v", m)
	}
	if err := s.UpdateModel(t.Context(), "01900000-0000-7000-8000-000000000001", in); !errors.Is(err, provider.ErrNotFound) {
		t.Fatalf("无效更新: %v", err)
	}
	all, err := s.ListModels(t.Context(), false)
	if err != nil || len(all) != 1 {
		t.Fatalf("更新错误地新增了记录: %+v %v", all, err)
	}
}
