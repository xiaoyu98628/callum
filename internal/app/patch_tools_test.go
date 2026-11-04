package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/xiaoyu98628/callum/internal/agent"
	"github.com/xiaoyu98628/callum/internal/config"
	"github.com/xiaoyu98628/callum/internal/tool"
	"github.com/xiaoyu98628/callum/internal/tool/executor"
)

func TestPatchToolsThroughStdio(t *testing.T) {
	for _, test := range []struct {
		name, policy, decision                                    string
		update, outside, stream, changeDuringApproval, fragmented bool
	}{
		{name: "create_workspace"},
		{name: "create_never", policy: "never"},
		{name: "update_stream", update: true, stream: true},
		{name: "fragmented_create", stream: true, fragmented: true},
		{name: "always_approve", policy: "always", decision: "approved"},
		{name: "always_reject", policy: "always", decision: "rejected"},
		{name: "outside_approve", outside: true, decision: "approved", stream: true},
		{name: "outside_never", outside: true, policy: "never"},
		{name: "cancel_waiting", policy: "always", decision: "cancel"},
		{name: "version_changed", update: true, policy: "always", decision: "approved", changeDuringApproval: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			workspace, outside := t.TempDir(), t.TempDir()
			root := workspace
			if test.outside {
				root = outside
			}
			path := filepath.Join(root, "note.txt")
			resolvedRoot, err := filepath.EvalSymlinks(root)
			if err != nil {
				t.Fatal(err)
			}
			resolvedPath := filepath.Join(resolvedRoot, "note.txt")
			before := "head\nold\ntail\n"
			if test.update {
				if err := os.WriteFile(path, []byte(before), 0640); err != nil {
					t.Fatal(err)
				}
			}
			var toolResults atomic.Int32
			var digest atomic.Value
			partialSent, completeArguments := make(chan struct{}), make(chan struct{})
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var req struct {
					Stream   bool              `json:"stream"`
					Tools    []json.RawMessage `json:"tools"`
					Messages []struct {
						Role       string `json:"role"`
						Content    string `json:"content"`
						ToolCallID string `json:"tool_call_id"`
					} `json:"messages"`
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.Messages) == 0 {
					t.Errorf("请求无效: %v", err)
					w.WriteHeader(400)
					return
				}
				if len(req.Tools) != 5 {
					t.Errorf("工具数量不符: %d", len(req.Tools))
				}
				last := req.Messages[len(req.Messages)-1]
				if last.Role == "user" && last.Content == "核对已写文件" {
					found := false
					for _, message := range req.Messages {
						if message.Role == "tool" && message.ToolCallID == "patch-files" && strings.Contains(message.Content, `"outcome":"applied"`) && strings.Contains(message.Content, `"changes"`) {
							found = true
						}
					}
					if !found {
						t.Error("后续上下文没有恢复真实文件操作")
					}
					writeFileToolResponse(t, w, req.Stream, map[string]any{"content": "文件操作事实已恢复"}, "stop")
					return
				}
				if last.Role == "tool" {
					var result struct {
						Operation tool.OutcomeRecord `json:"operation"`
						Result    struct {
							SHA256 string `json:"sha256"`
						} `json:"result"`
						Error *struct {
							Kind string `json:"kind"`
						} `json:"error"`
					}
					if err := json.Unmarshal([]byte(last.Content), &result); err != nil {
						t.Error(err)
						w.WriteHeader(400)
						return
					}
					if last.ToolCallID == "read-before" {
						digest.Store(result.Result.SHA256)
					} else {
						toolResults.Add(1)
						want := tool.OutcomeApplied
						if test.decision == "rejected" || (test.outside && test.policy == "never") || test.changeDuringApproval {
							want = tool.OutcomeNotApplied
						}
						if result.Operation.Outcome != want {
							t.Errorf("操作事实不符: %s", last.Content)
						}
						if test.changeDuringApproval && (result.Error == nil || result.Error.Kind != "file_conflict") {
							t.Errorf("没有返回版本冲突: %s", last.Content)
						}
						writeFileToolResponse(t, w, req.Stream, map[string]any{"content": "已记录文件操作结果"}, "stop")
						return
					}
				}
				callPath := "note.txt"
				if test.outside {
					callPath = path
				}
				name, id := "apply_patch", "patch-files"
				var args any
				if test.update && digest.Load() == nil {
					name, id = "read_file", "read-before"
					args = map[string]string{"path": callPath}
				} else {
					body := "*** Add File: " + callPath + "\n+created\n"
					versions := []map[string]string{}
					if test.update {
						body = "*** Update File: " + callPath + "\n@@\n head\n-old\n+new\n tail\n"
						versions = append(versions, map[string]string{"path": callPath, "sha256": digest.Load().(string)})
					}
					args = map[string]any{"patch": "*** Begin Patch\n" + body + "*** End Patch\n", "base_versions": versions}
				}
				raw, err := json.Marshal(args)
				if err != nil {
					t.Error(err)
					w.WriteHeader(500)
					return
				}
				call := map[string]any{"id": id, "type": "function", "function": map[string]any{"name": name, "arguments": string(raw)}}
				if req.Stream {
					call["index"] = 0
				}
				if test.fragmented && name == "apply_patch" {
					cut := len(raw) / 2
					call["function"] = map[string]any{"name": name, "arguments": string(raw[:cut])}
					chunk, err := json.Marshal(map[string]any{"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"tool_calls": []any{call}}, "finish_reason": nil}}})
					if err != nil {
						t.Error(err)
						return
					}
					w.Header().Set("Content-Type", "text/event-stream")
					if _, err := io.WriteString(w, "data: "+string(chunk)+"\n\n"); err != nil {
						t.Error(err)
						return
					}
					w.(http.Flusher).Flush()
					close(partialSent)
					select {
					case <-completeArguments:
					case <-r.Context().Done():
						return
					}
					writeFileToolResponse(t, w, true, map[string]any{"tool_calls": []any{map[string]any{"index": 0, "function": map[string]any{"arguments": string(raw[cut:])}}}}, "tool_calls")
					return
				}
				writeFileToolResponse(t, w, req.Stream, map[string]any{"tool_calls": []any{call}}, "tool_calls")
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
			workspaceID := resultString(t, p.call(t, "workspace.create", map[string]any{"name": "文件工具", "root_path": workspace}), "id")
			accepted := p.call(t, "conversation.send", map[string]any{"workspace_id": workspaceID, "model_id": mid, "stream": test.stream, "content": messageBlocks("创建或修改文件")})
			runID := resultString(t, accepted, "run_id")
			if test.fragmented {
				select {
				case <-partialSent:
				case <-time.After(3 * time.Second):
					t.Fatal("未收到流式参数片段")
				}
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Fatal("参数尚不完整时创建了文件")
				}
				close(completeArguments)
			}
			var patchApproval agent.Approval
			if test.decision != "" {
				for {
					var event struct {
						Approval agent.Approval `json:"approval"`
					}
					if err := json.Unmarshal(p.event(t, "approval.requested").Params, &event); err != nil {
						t.Fatal(err)
					}
					var operation executor.Operation
					if json.Unmarshal(event.Approval.Request, &operation) != nil {
						t.Fatal("审批快照无效")
					}
					if operation.Tool.Name == "read_file" {
						requireOK(t, p.call(t, "approval.decide", map[string]any{"approval_id": event.Approval.ID, "decision": "approved"}))
						continue
					}
					patchApproval = event.Approval
					if operation.Plan == nil || len(operation.Plan.Targets) != 1 || operation.Plan.Targets[0].Path != resolvedPath {
						t.Fatalf("审批没有固定实际写入目标: %s", event.Approval.Request)
					}
					if test.update {
						sum := sha256.Sum256([]byte(before))
						if operation.Plan.Targets[0].ExpectedSHA256 != hex.EncodeToString(sum[:]) {
							t.Fatal("审批未绑定文件版本")
						}
						if actual, err := os.ReadFile(path); err != nil || string(actual) != before {
							t.Fatal("审批前修改了目标")
						}
					} else if _, err := os.Stat(path); !os.IsNotExist(err) {
						t.Fatal("审批前创建了目标")
					}
					entries, err := os.ReadDir(root)
					if err != nil {
						t.Fatal(err)
					}
					for _, entry := range entries {
						if strings.HasPrefix(entry.Name(), ".callum-patch-") {
							t.Fatal("审批前创建了恢复副本")
						}
					}
					if test.changeDuringApproval {
						if err := os.WriteFile(path, []byte("user changed"), 0640); err != nil {
							t.Fatal(err)
						}
					}
					if test.decision == "cancel" {
						requireOK(t, p.call(t, "run.cancel", map[string]any{"run_id": runID}))
					} else {
						requireOK(t, p.call(t, "approval.decide", map[string]any{"approval_id": event.Approval.ID, "decision": test.decision}))
					}
					break
				}
			}
			terminal := "run.completed"
			if test.decision == "cancel" {
				terminal = "run.cancelled"
			}
			p.event(t, terminal)
			want := !test.changeDuringApproval && test.decision != "rejected" && test.decision != "cancel" && !(test.outside && test.policy == "never")
			actual, readErr := os.ReadFile(path)
			if want {
				text := "created\n"
				if test.update {
					text = "head\nnew\ntail\n"
				}
				if readErr != nil || string(actual) != text {
					t.Fatalf("实际文件内容不符: %q %v", actual, readErr)
				}
			} else if test.update {
				text := before
				if test.changeDuringApproval {
					text = "user changed"
				}
				if readErr != nil || string(actual) != text {
					t.Fatal("覆盖了用户文件")
				}
			} else if !os.IsNotExist(readErr) {
				t.Fatal("被拒绝或取消的操作创建了文件")
			}
			stepsFrame := p.call(t, "step.list", map[string]any{"run_id": runID})
			requireOK(t, stepsFrame)
			var steps []agent.StepSummary
			if err := json.Unmarshal(stepsFrame.Result, &steps); err != nil {
				t.Fatal(err)
			}
			found := false
			for _, step := range steps {
				if step.Name != "apply_patch" {
					continue
				}
				found = true
				frame := p.call(t, "step.data", map[string]any{"step_id": step.ID, "field": "output"})
				requireOK(t, frame)
				var data struct {
					Fragment string `json:"fragment"`
				}
				if json.Unmarshal(frame.Result, &data) != nil || !strings.Contains(data.Fragment, `"operation"`) {
					t.Fatal("步骤缺少操作事实")
				}
			}
			if !found {
				t.Fatal("缺少持久化补丁步骤")
			}
			if want {
				conversationID := resultString(t, accepted, "conversation_id")
				requireOK(t, p.call(t, "conversation.send", map[string]any{"conversation_id": conversationID, "stream": test.stream, "content": messageBlocks("核对已写文件")}))
				p.event(t, "run.completed")
			}
			if test.decision != "cancel" && toolResults.Load() != 1 {
				t.Fatalf("模型未收到结果: %d", toolResults.Load())
			}
			if patchApproval.ID != "" && test.decision == "cancel" {
				frame := p.call(t, "approval.get", map[string]any{"id": patchApproval.ID})
				requireOK(t, frame)
				if !strings.Contains(string(frame.Result), "cancelled") {
					t.Fatal("取消未结束审批")
				}
			}
			p.stop(t)
		})
	}
}
