package app

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/xiaoyu98628/callum/internal/agent"
	"github.com/xiaoyu98628/callum/internal/config"
	"github.com/xiaoyu98628/callum/internal/settings"
	"github.com/xiaoyu98628/callum/internal/tool"
	"github.com/xiaoyu98628/callum/internal/tool/policy"
)

func saveApprovalMode(t *testing.T, p *appPeer, execution settings.Execution) {
	t.Helper()
	requireOK(t, p.call(t, "settings.update", map[string]any{"key": settings.ExecutionKey, "revision": readSettings(t, p).Revision, "value": execution}))
}

func TestAssistedApprovalThroughStdio(t *testing.T) {
	for _, tc := range []struct {
		name, reply                  string
		reviewStatus                 string
		cancelled, highRisk, timeout bool
	}{
		{name: "approve", reply: `{"decision":"approved","reason":"用户明确要求创建此文件。"}`, reviewStatus: agent.ApprovalApproved},
		{name: "reject", reply: `{"decision":"rejected","reason":"与请求无关。"}`, reviewStatus: agent.ApprovalRejected},
		{name: "handoff", reply: `{"decision":"handoff","reason":"需要核对目标。"}`, reviewStatus: "handoff"},
		{name: "malformed", reply: `{"decision":"rejected","decision":"approved","reason":"重复键"}`, reviewStatus: "handoff"},
		{name: "provider_error", reviewStatus: "handoff"},
		{name: "cancel", cancelled: true},
		{name: "timeout", timeout: true, reviewStatus: "handoff"},
		{name: "escalated_command", highRisk: true},
		{name: "workspace_script", highRisk: true},
		{name: "workspace_mkdir", reply: `{"decision":"approved","reason":"用户明确要求创建目录。"}`, reviewStatus: agent.ApprovalApproved},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mkdir := tc.name == "workspace_mkdir"
			script := tc.name == "workspace_script"
			if (mkdir || script) && runtime.GOOS != "darwin" {
				t.Skip("验证 Seatbelt 项目写入")
			}
			root := t.TempDir()
			marker := filepath.Join(root, "reviewed.txt")
			sandbox, intent := tool.SandboxReadOnly, "请创建 reviewed.txt 并写入指定文字。"
			toolName := "apply_patch"
			args := map[string]any{"patch": "*** Begin Patch\n*** Add File: reviewed.txt\n+approved only\n*** End Patch\n", "base_versions": []any{}}
			switch {
			case script:
				toolName, sandbox = "run_command", tool.SandboxWorkspaceWrite
				args = map[string]any{"command": []string{"/bin/sh", "-c", "touch reviewed.txt"}, "sandbox_permissions": "workspace_write", "justification": "在沙箱内执行测试脚本"}
			case tc.highRisk:
				toolName = "run_command"
				args = map[string]any{"command": []string{"/usr/bin/touch", marker}, "sandbox_permissions": "require_escalated", "justification": "受控审批验证"}
			case mkdir:
				toolName, sandbox, intent = "run_command", tool.SandboxWorkspaceWrite, "请创建 reviewed-dir 目录。"
				marker = filepath.Join(root, "reviewed-dir")
				args = map[string]any{"command": []string{"/bin/mkdir", "reviewed-dir"}, "sandbox_permissions": "workspace_write", "justification": "在项目内创建测试目录"}
			}
			rawArgs, err := json.Marshal(args)
			if err != nil {
				t.Fatal(err)
			}
			var reviewCalls atomic.Int32
			started := make(chan struct{})
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request struct {
					Tools    []json.RawMessage
					Stream   bool
					Messages []struct{ Role, Content string }
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil || len(request.Messages) == 0 {
					t.Errorf("无效请求: %v", err)
					w.WriteHeader(400)
					return
				}
				if strings.HasPrefix(request.Messages[0].Content, "你独立审核") {
					reviewCalls.Add(1)
					if request.Stream || len(request.Tools) != 0 || len(request.Messages) != 2 || !strings.Contains(request.Messages[1].Content, intent) || !strings.Contains(request.Messages[1].Content, "fixed_operation") {
						t.Errorf("审核上下文没有隔离: %+v", request)
					}
					if tc.cancelled || tc.timeout {
						close(started)
						<-r.Context().Done()
						return
					}
					if tc.reply == "" {
						w.WriteHeader(503)
						return
					}
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"index": 0, "message": map[string]any{"role": "assistant", "content": tc.reply}, "finish_reason": "stop"}}, "usage": map[string]int{"prompt_tokens": 42, "completion_tokens": 5, "total_tokens": 47}})
					return
				}
				if len(request.Tools) == 0 || request.Messages[len(request.Messages)-1].Role == "tool" {
					writeFileToolResponse(t, w, request.Stream, map[string]any{"content": "任务结束"}, "stop")
					return
				}
				call := map[string]any{"id": "review-target", "type": "function", "function": map[string]any{"name": toolName, "arguments": string(rawArgs)}}
				writeFileToolResponse(t, w, request.Stream, map[string]any{"tool_calls": []any{call}}, "tool_calls")
			}))
			defer upstream.Close()
			cfg, err := config.Load(filepath.Join(t.TempDir(), "data"))
			if err != nil {
				t.Fatal(err)
			}
			cfg.MaxConcurrentRuns = 1
			if tc.timeout {
				cfg.CallTimeout = 400 * time.Millisecond
			}
			p := startApp(t, cfg, io.Discard)
			defer func() {
				if !p.stopped {
					p.stop(t)
				}
			}()
			saveApprovalMode(t, p, settings.Execution{ApprovalPolicy: policy.OnRequest, ApprovalsReviewer: policy.AutoReview, SandboxMode: sandbox})
			mid := setupConversationModel(t, p, upstream.URL)
			wid := resultString(t, p.call(t, "workspace.create", map[string]any{"name": "受控审核项目", "root_path": root}), "id")
			accepted := p.call(t, "conversation.send", map[string]any{"model_id": mid, "workspace_id": wid, "content": messageBlocks(intent), "stream": false})
			rid := resultString(t, accepted, "run_id")
			if tc.cancelled {
				select {
				case <-started:
				case <-time.After(5 * time.Second):
					t.Fatal("审核未开始")
				}
				requireOK(t, p.call(t, "run.cancel", map[string]any{"run_id": rid}))
				p.event(t, "run.cancelled")
			} else if tc.highRisk || tc.reviewStatus == "handoff" {
				var event struct{ Approval agent.Approval }
				if err := json.Unmarshal(p.event(t, "approval.requested").Params, &event); err != nil {
					t.Fatal(err)
				}
				if !tc.highRisk && (event.Approval.Review == nil || event.Approval.Review.Decision != "handoff") {
					t.Fatalf("未保留转人工的审核结论: %+v", event.Approval)
				}
				requireOK(t, p.call(t, "approval.decide", map[string]any{"approval_id": event.Approval.ID, "decision": "rejected"}))
				p.event(t, "run.completed")
			} else {
				var event struct{ Approval agent.Approval }
				if err := json.Unmarshal(p.event(t, "approval.updated").Params, &event); err != nil {
					t.Fatal(err)
				}
				a := event.Approval
				if a.DecidedBy == nil || *a.DecidedBy != agent.ApprovalAutoReviewer || a.Status != tc.reviewStatus || a.Review == nil || a.DirectoryGrant != nil || a.DirectoryScope != nil {
					t.Fatalf("自动决定未绑定审批记录: %+v", a)
				}
				p.event(t, "run.completed")
				// 已提交的 AI 决定不会被迟到的人工决定覆盖或认领。
				if got := p.call(t, "approval.decide", map[string]any{"approval_id": a.ID, "decision": a.Status}); got.Error == nil || got.Error.Code != 1003 {
					t.Fatal("人工请求接管了已决定审批")
				}
				steps := p.call(t, "step.list", map[string]any{"run_id": rid})
				requireOK(t, steps)
				var rows []agent.StepSummary
				if err := json.Unmarshal(steps.Result, &rows); err != nil {
					t.Fatal(err)
				}
				audited := false
				for _, row := range rows {
					if row.ID == a.Review.StepID && row.Name == "AI 审批" && row.Model != nil && row.Model.ID == mid && row.UsageBytes > 0 {
						audited = true
					}
				}
				if !audited {
					t.Fatal("审核模型或实际用量未记录")
				}
				if tc.reviewStatus == agent.ApprovalApproved {
					p.stop(t)
					p = startApp(t, cfg, io.Discard)
					stored := p.call(t, "approval.get", map[string]any{"id": a.ID})
					requireOK(t, stored)
					var restored agent.Approval
					if err := json.Unmarshal(stored.Result, &restored); err != nil || restored.DecidedBy == nil || *restored.DecidedBy != agent.ApprovalAutoReviewer || restored.Review == nil || *restored.Review != *a.Review {
						t.Fatalf("重启后审核记录不一致: %+v %v", restored, err)
					}
				}
			}
			wantCalls := int32(1)
			if tc.highRisk {
				wantCalls = 0
			}
			if tc.reply != "" {
				var steps []agent.StepSummary
				f := p.call(t, "step.list", map[string]any{"run_id": rid})
				requireOK(t, f)
				if err := json.Unmarshal(f.Result, &steps); err != nil {
					t.Fatal(err)
				}
				found := false
				for _, step := range steps {
					if step.Name != "AI 审批" {
						continue
					}
					data := p.call(t, "step.data", map[string]any{"step_id": step.ID, "field": "output"})
					requireOK(t, data)
					var fragment struct{ Fragment string }
					if err := json.Unmarshal(data.Result, &fragment); err != nil {
						t.Fatal(err)
					}
					var output struct {
						Response struct {
							Choices []struct{ Message struct{ Content string } }
						}
						Review *agent.ApprovalReview `json:"approval_review"`
					}
					if json.Unmarshal([]byte(fragment.Fragment), &output) != nil || len(output.Response.Choices) != 1 || output.Response.Choices[0].Message.Content != tc.reply || output.Review == nil {
						t.Fatal("审核步骤未保留原始响应及独立结论")
					}
					found = true
				}
				if !found {
					t.Fatal("缺少审核审计步骤")
				}
			}
			if reviewCalls.Load() != wantCalls {
				t.Fatalf("不符合审核范围: %d", reviewCalls.Load())
			}
			if mkdir {
				if info, err := os.Stat(marker); err != nil || !info.IsDir() {
					t.Fatalf("获批的沙箱目录创建未执行: %v", err)
				}
				return
			}
			data, err := os.ReadFile(marker)
			if tc.reviewStatus == agent.ApprovalApproved {
				if err != nil || string(data) != "approved only\n" {
					t.Fatalf("获批操作未执行: %s %v", data, err)
				}
			} else if !os.IsNotExist(err) {
				t.Fatalf("非批准操作已写入: %q %v", data, err)
			}
		})
	}
}

func TestFullAccessThroughStdio(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("测试命令使用 Unix shell")
	}
	root, outside := t.TempDir(), t.TempDir()
	marker := filepath.Join(outside, "full-access.txt")
	args, err := json.Marshal(map[string]any{"command": []string{"/bin/sh", "-c", `printf approved > "$1"`, "sh", marker}})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Tools    []json.RawMessage
			Stream   bool
			Messages []struct{ Role string }
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil || len(request.Messages) == 0 {
			w.WriteHeader(400)
			return
		}
		if len(request.Tools) == 0 || request.Messages[len(request.Messages)-1].Role == "tool" {
			writeFileToolResponse(t, w, request.Stream, map[string]any{"content": "已结束"}, "stop")
			return
		}
		call := map[string]any{"id": "full-target", "type": "function", "function": map[string]any{"name": "run_command", "arguments": string(args)}}
		writeFileToolResponse(t, w, request.Stream, map[string]any{"tool_calls": []any{call}}, "tool_calls")
	}))
	defer server.Close()
	cfg, err := config.Load(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatal(err)
	}
	p := startApp(t, cfg, io.Discard)
	defer p.stop(t)
	saveApprovalMode(t, p, settings.Execution{ApprovalPolicy: policy.Never, ApprovalsReviewer: policy.User, SandboxMode: tool.SandboxFullAccess})
	mid := setupConversationModel(t, p, server.URL)
	wid := resultString(t, p.call(t, "workspace.create", map[string]any{"name": "完全访问测试", "root_path": root}), "id")
	sent := p.call(t, "conversation.send", map[string]any{"model_id": mid, "workspace_id": wid, "stream": false, "content": messageBlocks("写入隔离的外部目录")})
	rid := resultString(t, sent, "run_id")
	p.event(t, "run.completed")
	approvals := p.call(t, "approval.list", map[string]any{"run_id": rid})
	requireOK(t, approvals)
	if string(approvals.Result) != "[]" {
		t.Fatalf("完全访问意外等待审批: %s", approvals.Result)
	}
	data, err := os.ReadFile(marker)
	if err != nil || string(data) != "approved" {
		t.Fatalf("未以用户权限写入隔离目录: %q %v", data, err)
	}
	_, raw := commandStepOutput(t, p, rid)
	if !strings.Contains(string(raw), `"sandbox_mode":"danger-full-access"`) || !strings.Contains(string(raw), `"sandboxed":false`) {
		t.Fatalf("命令边界未正确记录: %s", raw)
	}
}
