package app

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/xiaoyu98628/callum/internal/agent"
	"github.com/xiaoyu98628/callum/internal/config"
	"github.com/xiaoyu98628/callum/internal/tool/executor"
)

func TestFileToolsIndependentOfWorkspaceThroughStdio(t *testing.T) {
	for _, test := range []struct {
		name, operation, policy, decision, errorKind      string
		bound, inside, relative, symlink, replace, stream bool
		capabilities                                      map[string]bool
		modelTypes                                        []string
	}{
		{name: "unbound_approve_stream", decision: "approved", stream: true},
		{name: "tools_false_does_not_disable_tools", decision: "approved", capabilities: map[string]bool{"tools": false}, modelTypes: []string{"text"}},
		{name: "content_types_do_not_disable_tools", decision: "approved", capabilities: map[string]bool{"reasoning": true}, modelTypes: []string{"text", "image", "audio", "video"}},
		{name: "unbound_reject", decision: "rejected", errorKind: "permission_denied"},
		{name: "unbound_cancel", decision: "cancel"},
		{name: "unbound_never", policy: "never", errorKind: "permission_denied"},
		{name: "unbound_relative", relative: true, errorKind: "path_base_required"},
		{name: "workspace_absolute_inside", bound: true, inside: true},
		{name: "workspace_absolute_inside_never", bound: true, inside: true, policy: "never"},
		{name: "workspace_absolute_inside_always", bound: true, inside: true, policy: "always", decision: "approved"},
		{name: "workspace_absolute_outside", bound: true, decision: "approved"},
		{name: "workspace_outside_never", bound: true, policy: "never", errorKind: "permission_denied"},
		{name: "workspace_symlink_outside", bound: true, symlink: true, relative: true, decision: "approved"},
		{name: "unbound_directory", operation: "list_directory", decision: "approved"},
		{name: "unbound_search", operation: "search_text", decision: "approved"},
		{name: "unbound_target_replaced", decision: "approved", replace: true, errorKind: "permission_denied"},
	} {
		t.Run(test.name, func(t *testing.T) {
			workspaceRoot, outside := t.TempDir(), t.TempDir()
			root := outside
			if test.inside {
				root = workspaceRoot
			}
			file := filepath.Join(root, "note.txt")
			if err := os.WriteFile(file, []byte("actual-file-content"), 0600); err != nil {
				t.Fatal(err)
			}
			operation := test.operation
			if operation == "" {
				operation = "read_file"
			}
			path := file
			if operation != "read_file" {
				path = root
			}
			if test.symlink {
				if err := os.Symlink(file, filepath.Join(workspaceRoot, "note.txt")); err != nil {
					t.Fatal(err)
				}
			}
			if test.relative {
				path = "note.txt"
			}
			args := map[string]any{"path": path}
			if operation == "search_text" {
				args["query"] = "actual-file-content"
			}
			arguments, err := json.Marshal(args)
			if err != nil {
				t.Fatal(err)
			}
			var results atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request struct {
					Stream   bool                             `json:"stream"`
					Tools    []json.RawMessage                `json:"tools"`
					Messages []struct{ Role, Content string } `json:"messages"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil || len(request.Messages) == 0 {
					t.Errorf("请求无效: %v", err)
					w.WriteHeader(400)
					return
				}
				if len(request.Tools) != 5 {
					t.Errorf("未绑定工作区时丢失工具定义: %d", len(request.Tools))
				}
				last := request.Messages[len(request.Messages)-1]
				if last.Role == "tool" {
					results.Add(1)
					var result struct {
						Error *struct {
							Kind string `json:"kind"`
						} `json:"error"`
					}
					if err := json.Unmarshal([]byte(last.Content), &result); err != nil {
						t.Error(err)
					}
					if test.errorKind != "" {
						if result.Error == nil || result.Error.Kind != test.errorKind {
							t.Errorf("错误结果不符: %s", last.Content)
						}
					} else if result.Error != nil {
						t.Errorf("文件操作失败: %s", last.Content)
					} else if operation == "read_file" || operation == "search_text" {
						if !bytes.Contains([]byte(last.Content), []byte("actual-file-content")) {
							t.Errorf("没有返回实际内容: %s", last.Content)
						}
					} else if !bytes.Contains([]byte(last.Content), []byte("note.txt")) {
						t.Errorf("没有返回目录条目: %s", last.Content)
					}
					writeFileToolResponse(t, w, request.Stream, map[string]any{"content": "读取完成"}, "stop")
					return
				}
				call := map[string]any{"id": "call-file", "type": "function", "function": map[string]any{"name": operation, "arguments": string(arguments)}}
				if request.Stream {
					call["index"] = 0
				}
				writeFileToolResponse(t, w, request.Stream, map[string]any{"tool_calls": []any{call}}, "tool_calls")
			}))
			t.Cleanup(upstream.Close)
			cfg, err := config.Load(filepath.Join(t.TempDir(), "data"))
			if err != nil {
				t.Fatal(err)
			}
			cfg.MaxConcurrentRuns = 1
			p := startApp(t, cfg, io.Discard)
			if test.policy != "" {
				setApprovalPolicy(t, p, test.policy)
			}
			mid := setupConversationModel(t, p, upstream.URL)
			if test.capabilities != nil || test.modelTypes != nil {
				frame := p.call(t, "model.get", map[string]any{"id": mid})
				requireOK(t, frame)
				var existing struct {
					ProviderID string `json:"provider_id"`
				}
				if err := json.Unmarshal(frame.Result, &existing); err != nil {
					t.Fatal(err)
				}
				requireOK(t, p.call(t, "model.update", map[string]any{"id": mid, "provider_id": existing.ProviderID, "display_name": "类型元数据", "remote_model_id": "remote-model", "enabled": true, "capabilities": test.capabilities, "model_type": test.modelTypes}))
			}
			params := map[string]any{"model_id": mid, "stream": test.stream, "content": messageBlocks("读取指定目标")}
			if test.bound {
				params["workspace_id"] = resultString(t, p.call(t, "workspace.create", map[string]any{"name": "项目", "root_path": workspaceRoot}), "id")
			}
			accepted := p.call(t, "conversation.send", params)
			runID := resultString(t, accepted, "run_id")
			var approvalID string
			if test.decision != "" {
				var event struct {
					Approval agent.Approval `json:"approval"`
				}
				if err := json.Unmarshal(p.event(t, "approval.requested").Params, &event); err != nil {
					t.Fatal(err)
				}
				approvalID = event.Approval.ID
				var snapshot executor.Operation
				if err := json.Unmarshal(event.Approval.Request, &snapshot); err != nil {
					t.Fatal(err)
				}
				resolved, err := filepath.EvalSymlinks(file)
				if operation != "read_file" {
					resolved, err = filepath.EvalSymlinks(root)
				}
				if err != nil {
					t.Fatal(err)
				}
				if snapshot.Target == nil || snapshot.Target.Path != resolved || snapshot.Target.WithinWorkspace != test.inside || (snapshot.Workspace != nil) != test.bound {
					t.Fatalf("审批目标不准确: %s", event.Approval.Request)
				}
				if results.Load() != 0 {
					t.Fatal("审批前已返回工具结果")
				}
				if test.replace {
					if err := os.Rename(file, file+"-original"); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(file, []byte("unapproved replacement"), 0600); err != nil {
						t.Fatal(err)
					}
				}
				if test.decision == "cancel" {
					requireOK(t, p.call(t, "run.cancel", map[string]any{"run_id": runID}))
				} else {
					requireOK(t, p.call(t, "approval.decide", map[string]any{"approval_id": approvalID, "decision": test.decision}))
				}
			}
			terminal := "run.completed"
			if test.decision == "cancel" {
				terminal = "run.cancelled"
			}
			p.event(t, terminal)
			wantResults := int32(1)
			if test.decision == "cancel" {
				wantResults = 0
			}
			if results.Load() != wantResults {
				t.Fatalf("工具结果次数错误: %d", results.Load())
			}
			frame := p.call(t, "approval.list", map[string]any{"run_id": runID})
			requireOK(t, frame)
			var rows []agent.Approval
			if err := json.Unmarshal(frame.Result, &rows); err != nil {
				t.Fatal(err)
			}
			want := 0
			if approvalID != "" {
				want = 1
			}
			if len(rows) != want {
				t.Fatalf("审批数量错误: %s", frame.Result)
			}
			if test.decision == "cancel" && rows[0].Status != agent.ApprovalCancelled {
				t.Fatal("取消没有结束待审批记录")
			}
			p.stop(t)
		})
	}
}

// writeFileToolResponse 用同一响应内容覆盖 JSON 与完整 SSE 工具参数的真实适配调用链。
func writeFileToolResponse(t *testing.T, w http.ResponseWriter, stream bool, content map[string]any, reason string) {
	t.Helper()
	choice := map[string]any{"index": 0, "finish_reason": reason}
	if stream {
		choice["delta"] = content
		w.Header().Set("Content-Type", "text/event-stream")
	} else {
		content["role"] = "assistant"
		choice["message"] = content
		w.Header().Set("Content-Type", "application/json")
	}
	data, err := json.Marshal(map[string]any{"choices": []any{choice}, "usage": map[string]int{"total_tokens": 1}})
	if err != nil {
		t.Error(err)
		return
	}
	if stream {
		_, err = io.WriteString(w, "data: "+string(data)+"\n\ndata: [DONE]\n\n")
	} else {
		_, err = w.Write(data)
	}
	if err != nil {
		t.Error(err)
	}
}
