package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/xiaoyu98628/callum/internal/agent"
	"github.com/xiaoyu98628/callum/internal/config"
)

func TestMCPChildProcess(t *testing.T) {
	marker := os.Getenv("CALLUM_MCP_FIXTURE_MARKER")
	if marker == "" {
		return
	}
	server := sdk.NewServer(&sdk.Implementation{Name: "审批测试服务", Version: "1"}, nil)
	server.AddTool(&sdk.Tool{Name: "echo", Description: "受控回显", InputSchema: json.RawMessage(`{"type":"object","properties":{"text":{"type":"string"}},"required":["text"],"additionalProperties":false}`)}, func(_ context.Context, _ *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
		if err := os.WriteFile(marker, []byte("called"), 0600); err != nil {
			return nil, err
		}
		return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: "mcp-returned"}}}, nil
	})
	if err := server.Run(t.Context(), &sdk.StdioTransport{}); err != nil {
		os.Exit(2)
	}
	os.Exit(0)
}

func TestMCPStdioApprovalThroughPublicProtocol(t *testing.T) {
	for _, mode := range []string{"approved", "rejected", "changed", "never"} {
		t.Run(mode, func(t *testing.T) {
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var input struct {
					Messages []struct {
						Role    string
						Content string
					}
					Tools []struct {
						Function struct{ Name, Description string }
					}
				}
				if json.NewDecoder(r.Body).Decode(&input) != nil || len(input.Messages) == 0 {
					t.Error("模型请求不完整")
					w.WriteHeader(400)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				last := input.Messages[len(input.Messages)-1]
				if last.Role == "tool" {
					if strings.Contains(last.Content, "mcp-returned") != (mode == "approved") {
						t.Errorf("工具结果和审批不匹配: %s", last.Content)
					}
					_, _ = io.WriteString(w, `{"choices":[{"index":0,"message":{"role":"assistant","content":"执行结束"},"finish_reason":"stop"}]}`)
					return
				}
				for _, item := range input.Tools {
					if strings.HasPrefix(item.Function.Name, "mcp_") && strings.Contains(item.Function.Description, "受控回显") {
						_, _ = fmt.Fprintf(w, `{"choices":[{"index":0,"message":{"role":"assistant","content":"调用工具","tool_calls":[{"id":"mcp-test","type":"function","function":{"name":%q,"arguments":"{\"text\":\"示例\"}"}}]},"finish_reason":"tool_calls"}]}`, item.Function.Name)
						return
					}
				}
				_, _ = io.WriteString(w, `{"choices":[{"index":0,"message":{"role":"assistant","content":"测试标题"},"finish_reason":"stop"}]}`)
			}))
			t.Cleanup(provider.Close)
			cfg, err := config.Load(filepath.Join(t.TempDir(), "data"))
			if err != nil {
				t.Fatal(err)
			}
			p := startApp(t, cfg, io.Discard)
			if mode == "never" {
				setApprovalPolicy(t, p, "never")
			}
			executable, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			marker := filepath.Join(t.TempDir(), "call")
			server := map[string]any{
				"name": "回显 MCP", "transport": "stdio", "auth": "none", "enabled": true,
				"command": executable, "args": []string{"-test.run=^TestMCPChildProcess$"}, "allowed_tools": []string{"echo"},
				"env": map[string]string{"CALLUM_MCP_FIXTURE_MARKER": marker},
			}
			unconfirmed := p.call(t, "mcp.save", map[string]any{"revision": 1, "server": server})
			if unconfirmed.Error == nil {
				t.Fatal("未经确认启用本地进程")
			}
			id := resultString(t, p.call(t, "mcp.save", map[string]any{"revision": 1, "server": server, "confirmed": true}), "id")
			if p.call(t, "mcp.test", map[string]any{"id": id, "confirmed": true, "revision": 1}).Error == nil {
				t.Fatal("允许用旧配置确认启动进程")
			}
			requireOK(t, p.call(t, "mcp.test", map[string]any{"id": id, "confirmed": true, "revision": 2}))
			mid := setupConversationModel(t, p, provider.URL)
			accepted := p.call(t, "conversation.send", map[string]any{"model_id": mid, "stream": false, "content": messageBlocks("调用回显")})
			runID := resultString(t, accepted, "run_id")
			if mode != "never" {
				var payload struct {
					Approval agent.Approval `json:"approval"`
				}
				if err := json.Unmarshal(p.event(t, "approval.requested").Params, &payload); err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(string(payload.Approval.Request), "回显 MCP") {
					t.Fatal("审批缺少工具来源")
				}
				if _, err := os.Stat(marker); !os.IsNotExist(err) {
					t.Fatal("审批前调用了 MCP 工具")
				}
				decision := mode
				if mode == "changed" {
					server["id"] = id
					server["enabled"] = false
					requireOK(t, p.call(t, "mcp.save", map[string]any{"revision": 2, "server": server}))
					decision = "approved"
				}
				requireOK(t, p.call(t, "approval.decide", map[string]any{"approval_id": payload.Approval.ID, "decision": decision}))
			}
			p.event(t, "run.completed")
			_, err = os.Stat(marker)
			if (err == nil) != (mode == "approved") {
				t.Fatalf("审批后调用结果不符: %v", err)
			}
			result := p.call(t, "step.list", map[string]any{"run_id": runID, "limit": 20})
			requireOK(t, result)
			if !strings.Contains(string(result.Result), "回显 MCP") {
				t.Fatalf("执行记录缺少 MCP 来源: %s", result.Result)
			}
			p.stop(t)
		})
	}
}
