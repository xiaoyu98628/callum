package app

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/xiaoyu98628/callum/internal/agent"
	"github.com/xiaoyu98628/callum/internal/config"
	"github.com/xiaoyu98628/callum/internal/tool"
	"github.com/xiaoyu98628/callum/internal/tool/executor"
)

func commandHelper(t *testing.T) string {
	t.Helper()
	name := "command helper"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	path := filepath.Join(t.TempDir(), name)
	cmd := exec.CommandContext(t.Context(), filepath.Join(runtime.GOROOT(), "bin", "go"), "build", "-mod=readonly", "-buildvcs=false", "-o", path, "../tool/builtin/command/run/testdata/process")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("构建命令测试进程: %s %v", output, err)
	}
	return path
}

func commandStepOutput(t *testing.T, p *appPeer, runID string) (agent.StepSummary, json.RawMessage) {
	t.Helper()
	frame := p.call(t, "step.list", map[string]any{"run_id": runID})
	requireOK(t, frame)
	var rows []agent.StepSummary
	if err := json.Unmarshal(frame.Result, &rows); err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row.Name == "run_command" {
			var raw strings.Builder
			for offset := 0; ; {
				frame := p.call(t, "step.data", map[string]any{"step_id": row.ID, "field": "output", "offset": offset})
				requireOK(t, frame)
				var part struct {
					Fragment string
					Done     bool
					Next     int `json:"next_offset"`
				}
				if err := json.Unmarshal(frame.Result, &part); err != nil {
					t.Fatal(err)
				}
				raw.WriteString(part.Fragment)
				if part.Done {
					return row, json.RawMessage(raw.String())
				}
				offset = part.Next
			}
		}
	}
	t.Fatal("缺少命令步骤")
	return agent.StepSummary{}, nil
}

func TestCommandToolsThroughStdio(t *testing.T) {
	helper := commandHelper(t)
	for _, test := range []struct {
		name, policy, decision, mode string
		bound, stream                bool
	}{
		{"sandbox_automatic", "on-request", "", "write", true, false},
		{"sandbox_never_automatic", "never", "", "write", true, false},
		{"workspace_approve", "on-request", "approved", "write", true, false},
		{"absolute_cwd_stream", "on-request", "approved", "stream", false, true},
		{"always_reject", "always", "rejected", "write", true, true},
		{"never_denies", "never", "", "write", true, false},
		{"cancel_approval", "on-request", "cancel", "write", true, false},
		{"nonzero_exit", "on-request", "approved", "exit", true, false},
		{"cancel_running", "on-request", "approved", "cancel", true, true},
		{"tool_timeout", "on-request", "approved", "timeout", true, false},
		{"shutdown_children", "on-request", "approved", "shutdown", true, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			automatic := strings.HasPrefix(test.name, "sandbox_")
			if automatic && runtime.GOOS != "darwin" {
				t.Skip("该平台尚未接入自动命令沙箱")
			}
			cwd := t.TempDir()
			marker := filepath.Join(cwd, "marker")
			release := filepath.Join(cwd, "release")
			heartbeat := filepath.Join(cwd, "heartbeat")
			command := []string{helper, "write", marker}
			switch test.mode {
			case "stream":
				command = []string{helper, "stream", release}
			case "exit":
				command = []string{helper, "exit", "7"}
			case "cancel", "timeout":
				command = []string{helper, "sleep"}
			case "shutdown":
				command = []string{helper, "spawn", heartbeat}
			}
			// 本组覆盖明确的沙箱外逐次审批；普通沙箱命令在独立用例验证自动执行。
			args := map[string]any{"command": command, "sandbox_permissions": "require_escalated", "justification": "仅在受控临时目录验证本次审批和进程生命周期"}
			if automatic {
				delete(args, "sandbox_permissions")
				delete(args, "justification")
			}
			if !test.bound {
				args["cwd"] = cwd
			}
			rawArgs, err := json.Marshal(args)
			if err != nil {
				t.Fatal(err)
			}
			var results atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var input struct {
					Stream   bool
					Tools    []json.RawMessage
					Messages []struct {
						Role, Content string
						ToolCallID    string `json:"tool_call_id"`
					}
				}
				if err := json.NewDecoder(r.Body).Decode(&input); err != nil || len(input.Messages) == 0 {
					t.Errorf("模型请求无效: %v", err)
					w.WriteHeader(400)
					return
				}
				if len(input.Tools) == 0 {
					writeFileToolResponse(t, w, input.Stream, map[string]any{"content": "命令测试"}, "stop")
					return
				}
				if len(input.Tools) != 6 {
					t.Errorf("工具数量不符: %d", len(input.Tools))
				}
				last := input.Messages[len(input.Messages)-1]
				if last.Role == "tool" {
					results.Add(1)
					var output struct {
						Operation tool.OutcomeRecord `json:"operation"`
					}
					if err := json.Unmarshal([]byte(last.Content), &output); err != nil {
						t.Error(err)
					}
					want := tool.OutcomeApplied
					if test.decision == "rejected" || test.policy == "never" && !automatic {
						want = tool.OutcomeNotApplied
					}
					if test.mode == "timeout" {
						want = tool.OutcomeUnknown
					}
					if output.Operation.Outcome != want {
						t.Errorf("模型收到错误操作事实: %s", last.Content)
					}
					writeFileToolResponse(t, w, input.Stream, map[string]any{"content": "已取得命令结果"}, "stop")
					return
				}
				call := map[string]any{"id": "command-call", "type": "function", "function": map[string]any{"name": "run_command", "arguments": string(rawArgs)}}
				if input.Stream {
					call["index"] = 0
				}
				writeFileToolResponse(t, w, input.Stream, map[string]any{"tool_calls": []any{call}}, "tool_calls")
			}))
			t.Cleanup(upstream.Close)
			cfg, err := config.Load(filepath.Join(t.TempDir(), "data"))
			if err != nil {
				t.Fatal(err)
			}
			if test.mode == "timeout" {
				cfg.ToolTimeout = 200 * time.Millisecond
			}
			p := startApp(t, cfg, io.Discard)
			setApprovalPolicy(t, p, test.policy)
			mid := setupConversationModel(t, p, upstream.URL)
			params := map[string]any{"model_id": mid, "stream": test.stream, "content": messageBlocks("执行受控命令")}
			if test.bound {
				params["workspace_id"] = resultString(t, p.call(t, "workspace.create", map[string]any{"name": "命令", "root_path": cwd}), "id")
			}
			accepted := p.call(t, "conversation.send", params)
			requireOK(t, accepted)
			runID := resultString(t, accepted, "run_id")
			if test.decision != "" {
				var event struct {
					Approval agent.Approval `json:"approval"`
				}
				if err := json.Unmarshal(p.event(t, "approval.requested").Params, &event); err != nil {
					t.Fatal(err)
				}
				var operation executor.Operation
				if err := json.Unmarshal(event.Approval.Request, &operation); err != nil {
					t.Fatal(err)
				}
				var details struct {
					Executable, Cwd string
					Sandboxed       bool
				}
				if err := json.Unmarshal(operation.Details, &details); err != nil {
					t.Fatal(err)
				}
				actual, err := filepath.EvalSymlinks(cwd)
				if err != nil {
					t.Fatal(err)
				}
				if !filepath.IsAbs(details.Executable) || details.Cwd != actual || details.Sandboxed || len(operation.Tool.Effects) != 1 || operation.Tool.Effects[0] != tool.EffectUnknown {
					t.Fatalf("命令审批详情不完整: %s", event.Approval.Request)
				}
				if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("审批前执行了命令")
				}
				if test.decision == "cancel" {
					requireOK(t, p.call(t, "run.cancel", map[string]any{"run_id": runID}))
				} else {
					requireOK(t, p.call(t, "approval.decide", map[string]any{"approval_id": event.Approval.ID, "decision": test.decision}))
				}
			}
			var streamed strings.Builder
			if test.mode == "stream" || test.mode == "cancel" || test.mode == "shutdown" {
				var event struct {
					RunID, StepID string
					Output        agent.ToolOutput `json:"output"`
				}
				frame := p.event(t, "tool.output")
				var fields map[string]json.RawMessage
				if err := json.Unmarshal(frame.Params, &fields); err != nil {
					t.Fatal(err)
				}
				if err := json.Unmarshal(fields["run_id"], &event.RunID); err != nil {
					t.Fatal(err)
				}
				if err := json.Unmarshal(fields["step_id"], &event.StepID); err != nil {
					t.Fatal(err)
				}
				if err := json.Unmarshal(fields["output"], &event.Output); err != nil {
					t.Fatal(err)
				}
				bytes, err := base64.StdEncoding.DecodeString(event.Output.Data)
				if err != nil {
					t.Fatal(err)
				}
				streamed.Write(bytes)
				if event.RunID != runID || event.StepID == "" || event.Output.Sequence != 1 || event.Output.Encoding != "base64" {
					t.Fatalf("分片未关联执行: %s", frame.Params)
				}
				switch test.mode {
				case "stream":
					if err := os.WriteFile(release, nil, 0600); err != nil {
						t.Fatal(err)
					}
				case "cancel":
					requireOK(t, p.call(t, "run.cancel", map[string]any{"run_id": runID}))
				case "shutdown":
					p.stop(t)
					before, err := os.ReadFile(heartbeat)
					if err != nil {
						t.Fatal(err)
					}
					timer := time.NewTimer(120 * time.Millisecond)
					select {
					case <-timer.C:
					case <-t.Context().Done():
						timer.Stop()
						t.Fatal(t.Context().Err())
					}
					after, err := os.ReadFile(heartbeat)
					if err != nil || string(before) != string(after) {
						t.Fatal("应用关闭后子进程继续写入")
					}
					p = startApp(t, cfg, io.Discard)
				}
			}
			if test.mode != "shutdown" {
				terminal := "run.completed"
				if test.decision == "cancel" || test.mode == "cancel" {
					terminal = "run.cancelled"
				}
				p.event(t, terminal)
			}
			step, raw := commandStepOutput(t, p, runID)
			var saved struct {
				Operation tool.OutcomeRecord `json:"operation"`
				Result    struct {
					Boundary tool.CommandBoundary `json:"execution_boundary"`
					Started  bool
					ExitCode *int `json:"exit_code"`
					TimedOut bool `json:"timed_out"`
					Stdout   struct{ Data string }
				} `json:"result"`
			}
			if err := json.Unmarshal(raw, &saved); err != nil {
				t.Fatal(err)
			}
			want := tool.OutcomeApplied
			if test.decision == "rejected" || test.decision == "cancel" || test.policy == "never" && !automatic {
				want = tool.OutcomeNotApplied
			}
			if test.mode == "cancel" || test.mode == "timeout" || test.mode == "shutdown" {
				want = tool.OutcomeUnknown
			}
			if saved.Operation.Outcome != want || saved.Operation.RequestHash == "" {
				t.Fatalf("终态没有保存事实: %s", raw)
			}
			if automatic && (!saved.Result.Boundary.Sandboxed || saved.Result.Boundary.Escalated) {
				t.Fatalf("自动命令没有使用沙箱: %s", raw)
			}
			if want == tool.OutcomeNotApplied {
				if saved.Result.Started {
					t.Fatal("被拒绝或取消后仍启动")
				}
				if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("未批准的命令产生副作用")
				}
			}
			if test.mode == "write" && want == tool.OutcomeApplied {
				if data, err := os.ReadFile(marker); err != nil || string(data) != "executed" {
					t.Fatal("命令未执行")
				}
			}
			if test.mode == "stream" && (!strings.Contains(streamed.String(), "first") || saved.Result.Stdout.Data != "first中文\nsecond\n") {
				t.Fatalf("实时或最终输出丢失: %s", raw)
			}
			if test.mode == "exit" && (saved.Result.ExitCode == nil || *saved.Result.ExitCode != 7 || step.Status != agent.Completed) {
				t.Fatalf("非零退出码被当作启动失败: %s", raw)
			}
			if test.mode == "timeout" && (!saved.Result.TimedOut || step.Status != agent.Failed) {
				t.Fatalf("超时记录错误: %s", raw)
			}
			if test.mode != "cancel" && test.mode != "shutdown" && test.decision != "cancel" && results.Load() != 1 {
				t.Fatalf("模型未收到最终结果: %d", results.Load())
			}
			p.stop(t)
		})
	}
}
