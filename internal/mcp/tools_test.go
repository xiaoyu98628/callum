package mcp

import (
	"context"
	"testing"
	"time"

	"github.com/xiaoyu98628/callum/internal/model"
	"github.com/xiaoyu98628/callum/internal/settings"
	"github.com/xiaoyu98628/callum/internal/tool"
)

func TestToolsSnapshotKeepsCachedToolWhenDiscoveryBudgetExpires(t *testing.T) {
	slow := &entry{ctx: t.Context(), lock: make(chan struct{}, 1), cfg: settings.MCPServer{
		ID: "slow", Enabled: true, AllowedTools: []string{"slow"},
	}}
	ready := &entry{ctx: t.Context(), lock: make(chan struct{}, 1), cfg: settings.MCPServer{
		ID: "ready", Enabled: true, AllowedTools: []string{"cached"},
	}}
	slow.lock <- struct{}{}  // 模拟其他请求占用排在前面的服务。
	ready.lock <- struct{}{} // 读取已发布的目录也不应等待工具执行。
	conn := &connection{}
	cached := &remoteTool{name: "cached", def: tool.Definition{ToolDefinition: model.ToolDefinition{Name: "cached"}}}
	publishTools(conn, map[string]*remoteTool{"cached": cached})
	ready.publicConnection.Store(conn)
	s := &Service{revision: 1, cfg: settings.MCP{Servers: []settings.MCPServer{slow.cfg, ready.cfg}}, entries: map[string]*entry{
		"slow": slow, "ready": ready,
	}}

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Millisecond)
	defer cancel()
	got := s.ToolsSnapshot(ctx, true, 1)
	if len(got) != 1 || got[0] != cached {
		t.Fatalf("等待超时不应丢失另一服务已发布的工具: %v", got)
	}
}
