package mcp

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/xiaoyu98628/callum/internal/settings"
)

// snapshotRepository 故意持有输入引用，用于检查服务是否把自己的可变配置交给外部持有者。
type snapshotRepository struct {
	Repository
	loaded settings.MCP
	saved  settings.MCP
	secret *Credential
}

func (r *snapshotRepository) Load(context.Context) (settings.MCP, int64, error) {
	return r.loaded, 1, nil
}

func (r *snapshotRepository) Credential(context.Context, string) (Credential, error) {
	return Credential{}, nil
}

func (r *snapshotRepository) Save(_ context.Context, cfg settings.MCP, _ int64, _ string, secret *Credential) error {
	r.saved, r.secret = cfg, secret
	return nil
}

func TestConfigurationDoesNotShareMutableFields(t *testing.T) {
	command := filepath.Join(t.TempDir(), "command")
	cfg := settings.MCPServer{ID: "01901234-1234-7234-8234-123456789abc", Name: "配置隔离", Transport: settings.MCPStdio,
		Command: command, Auth: settings.MCPNone, Args: []string{"original"}, Env: map[string]string{"PUBLIC": "original"}, AllowedTools: []string{"safe"}}
	repo := &snapshotRepository{loaded: settings.MCP{Servers: []settings.MCPServer{cfg}}}
	s, err := New(t.Context(), t.Context(), repo, http.DefaultClient, http.DefaultClient, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	mutate := func(cfg settings.MCPServer) {
		cfg.Args[0], cfg.Env["PUBLIC"], cfg.AllowedTools[0] = "changed", "changed", "changed"
	}
	check := func(revision int64, index int) {
		t.Helper()
		view, err := s.List(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		got := view.Servers[index]
		if view.Revision != revision || got.Args[0] != "original" || got.Env["PUBLIC"] != "original" || got.AllowedTools[0] != "safe" {
			t.Fatalf("配置被外部引用改写: %+v", view)
		}
		mutate(got.MCPServer)
	}
	mutate(cfg) // 仓储加载值不能继续修改服务已持有的快照。
	check(1, 0)
	check(1, 0) // 查询返回值也必须独立。

	input := settings.MCPServer{Name: "保存隔离", Transport: settings.MCPStdio, Command: command, Auth: settings.MCPNone,
		Args: []string{"original"}, Env: map[string]string{"PUBLIC": "original"}, AllowedTools: []string{"safe"}}
	secret := map[string]string{"PRIVATE": "secret"}
	if _, err := s.Save(t.Context(), 1, input, SecretInput{Env: &secret}, SaveOptions{}); err != nil {
		t.Fatal(err)
	}
	mutate(input)
	secret["PRIVATE"] = "changed"
	if repo.secret == nil || repo.secret.Env["PRIVATE"] != "secret" {
		t.Fatal("保存凭据仍借用调用方的环境映射")
	}
	mutate(repo.saved.Servers[1])
	check(2, 1)
	check(2, 1)
}
