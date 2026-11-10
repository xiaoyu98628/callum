package run

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/xiaoyu98628/callum/internal/tool"
	"github.com/xiaoyu98628/callum/internal/tool/executor"
	"github.com/xiaoyu98628/callum/internal/tool/policy"
	"github.com/xiaoyu98628/callum/internal/tool/registry"
)

func buildHelper(t *testing.T) string {
	t.Helper()
	name := "helper space"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	path := filepath.Join(t.TempDir(), name)
	cmd := exec.CommandContext(t.Context(), filepath.Join(runtime.GOROOT(), "bin", "go"), "build", "-mod=readonly", "-buildvcs=false", "-o", path, "./testdata/process")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("构建受控进程: %s %v", output, err)
	}
	return path
}

func testExecutor(t *testing.T, environment []string) *executor.Executor {
	t.Helper()
	r := registry.New()
	if err := r.Register(New(environment, tool.ProtectedData{})); err != nil {
		t.Fatal(err)
	}
	e, err := executor.New(r, 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func prepareTest(t *testing.T, e *executor.Executor, scope tool.Scope, command []string, cwd string) *executor.Prepared {
	t.Helper()
	raw, err := json.Marshal(arguments{Command: command, Cwd: cwd})
	if err != nil {
		t.Fatal(err)
	}
	p, err := e.Prepare(t.Context(), scope, "run_command", raw)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := p.Close(); err != nil {
			t.Error(err)
		}
	})
	return p
}

func decodeReport(t *testing.T, result tool.Result) processReport {
	t.Helper()
	var report processReport
	if err := json.Unmarshal(result.Output, &report); err != nil {
		t.Fatalf("结果无效: %s %v", result.Output, err)
	}
	return report
}

func TestCommandValidation(t *testing.T) {
	instance := New(nil, tool.ProtectedData{})
	for _, raw := range []string{`{}`, `null`, `[]`, `{"command":[]}`, `{"command":[""]}`, `{"command":["go",null]}`, `{"command":["go",1]}`, `{"command":"go"}`, `{"command":["go"],"cwd":null}`, `{"command":["go"],"cwd":""}`, `{"command":["go"],"cwd":"../escape"}`, `{"command":["go"],"env":{}}`, `{"command":["go"],"command":["other"]}`, `{"command":["go"]} {}`, `{"command":["a\u0000b"]}`, `{"Command":["go"]}`, `{"command":["go"],"cwd":".","cwd":"other"}`} {
		if _, err := instance.Validate(json.RawMessage(raw)); !errors.Is(err, tool.ErrInvalid) {
			t.Fatalf("接受无效参数: %s %v", raw, err)
		}
		if invocation, err := instance.Prepare(t.Context(), tool.Scope{}, json.RawMessage(raw)); !errors.Is(err, tool.ErrInvalid) {
			if invocation != nil {
				if closeErr := invocation.Close(); closeErr != nil {
					t.Error(closeErr)
				}
			}
			t.Fatalf("准备阶段接受无效命令参数: %s, %v", raw, err)
		}

	}
	valid, err := instance.Validate(json.RawMessage(`{"cwd":"a/../b","command":["go","","a b","&;>"]}`))
	if err != nil || string(valid) != `{"command":["go","","a b","\u0026;\u003e"],"cwd":"b"}` {
		t.Fatalf("规范化改变参数: %s %v", valid, err)
	}
	if policy.Evaluate(policy.OnRequest, instance.Definition().Effects, false).Action != policy.Ask || policy.Evaluate(policy.Never, instance.Definition().Effects, false).Action != policy.Deny {
		t.Fatal("命令未按未知影响处理")
	}
}

func TestEscalationRequiresExplicitScopeAndReason(t *testing.T) {
	instance := New(nil, tool.ProtectedData{})
	for _, raw := range []string{
		`{"command":["pwd"],"sandbox_permissions":"full-access"}`,
		`{"command":["pwd"],"sandbox_permissions":"require_escalated"}`,
		`{"command":["pwd"],"sandbox_permissions":"require_escalated","justification":"  "}`,
		`{"command":["pwd"],"sandbox_permissions":null}`,
		`{"command":["pwd"],"justification":null}`,
	} {
		if _, err := instance.Validate(json.RawMessage(raw)); !errors.Is(err, tool.ErrInvalid) {
			t.Fatalf("接受了不明确的越权请求: %s %v", raw, err)
		}
	}
	for _, command := range [][]string{{"/bin/rm", "file"}, {"/bin/sh", "-c", "/usr/bin/git reset --hard"}, {"find", ".", "-delete"}} {
		if commandRisk(command) == "" {
			t.Fatalf("已识别的危险命令未保留审批: %v", command)
		}
	}
	if commandRisk([]string{"/bin/pwd"}) != "" {
		t.Fatal("pwd 仍需命令文字审批")
	}
}

func TestCommandSnapshotAndPreparationDoNotExecute(t *testing.T) {
	helper := buildHelper(t)
	cwd := t.TempDir()
	path := filepath.Join(cwd, "marker")
	e := testExecutor(t, append(os.Environ(), "OPENAI_API_KEY=private-test-value"))
	p := prepareTest(t, e, tool.Scope{ID: "w", RootPath: cwd}, []string{helper, "write", path}, "")
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("审批前执行命令")
	}
	var operation executor.Operation
	if err := json.Unmarshal(p.Request(), &operation); err != nil {
		t.Fatal(err)
	}
	var details struct {
		Executable, Cwd string
		EnvironmentKeys []string `json:"environment_keys"`
		Sandboxed       bool
	}
	if err := json.Unmarshal(operation.Details, &details); err != nil {
		t.Fatal(err)
	}
	resolved, err := filepath.EvalSymlinks(cwd)
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(details.Executable) || details.Cwd != resolved || details.Sandboxed || slices.Contains(details.EnvironmentKeys, "OPENAI_API_KEY") || strings.Contains(string(p.Request()), "private-test-value") {
		t.Fatalf("审批快照不正确: %s", p.Request())
	}
	if result, err := e.Execute(t.Context(), p, "wrong"); !errors.Is(err, tool.ErrDenied) || result.Outcome != tool.OutcomeNotApplied {
		t.Fatal("未验证操作指纹")
	}
	result, err := e.Execute(t.Context(), p, p.Hash())
	if err != nil || result.Outcome != tool.OutcomeApplied {
		t.Fatalf("执行失败: %s %v", result.Output, err)
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != "executed" {
		t.Fatalf("未执行固定命令: %s %v", data, err)
	}
}

func TestCommandArgumentsEnvironmentAndExitCodes(t *testing.T) {
	helper := buildHelper(t)
	e := testExecutor(t, append(os.Environ(), "OPENAI_API_KEY=private-test-value"))
	cwd := t.TempDir()
	args := []string{"", "a b", `quote"slash\`, "&;>", "中文"}
	p := prepareTest(t, e, tool.Scope{}, append([]string{helper, "inspect"}, args...), cwd)
	result, err := e.Execute(t.Context(), p, p.Hash())
	if err != nil || result.Outcome != tool.OutcomeApplied {
		t.Fatalf("执行失败: %s %v", result.Output, err)
	}
	report := decodeReport(t, result)
	var observed struct {
		Args            []string
		Cwd, Credential string
		StdinBytes      int `json:"stdin_bytes"`
	}
	if err := json.Unmarshal([]byte(report.Stdout.Data), &observed); err != nil {
		t.Fatal(err)
	}
	actual, err := filepath.EvalSymlinks(cwd)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(args, observed.Args) || observed.Cwd != actual || observed.Credential != "" || observed.StdinBytes != 0 || !report.CleanupComplete {
		t.Fatalf("参数、环境或工作目录改变: %+v %+v", observed, report)
	}
	for _, code := range []string{"0", "7"} {
		p := prepareTest(t, e, tool.Scope{}, []string{helper, "exit", code}, cwd)
		result, err := e.Execute(t.Context(), p, p.Hash())
		report := decodeReport(t, result)
		if err != nil || result.Outcome != tool.OutcomeApplied || report.ExitCode == nil || (*report.ExitCode != 0 && code == "0") || (*report.ExitCode != 7 && code == "7") || report.Stdout.Data != "out" || report.Stderr.Data != "err" {
			t.Fatalf("退出码或双流丢失: %s %v", result.Output, err)
		}
	}
}

func TestCommandStreamingAndCallbackFailure(t *testing.T) {
	helper := buildHelper(t)
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "stream", true: "callback_failure"}[fail], func(t *testing.T) {
			e := testExecutor(t, os.Environ())
			cwd := t.TempDir()
			release := filepath.Join(cwd, "release")
			p := prepareTest(t, e, tool.Scope{}, []string{helper, "stream", release}, cwd)
			var stdout, stderr strings.Builder
			sequence := int64(0)
			callbackErr := errors.New("subscriber stopped")
			result, err := e.ExecuteStream(t.Context(), p, p.Hash(), func(chunk tool.OutputChunk) error {
				sequence++
				if chunk.Sequence != sequence {
					t.Error("分片序号不连续")
				}
				if chunk.Stream == "stdout" {
					stdout.Write(chunk.Data)
				} else {
					stderr.Write(chunk.Data)
				}
				if fail {
					return callbackErr
				}
				if strings.Contains(stdout.String(), "first") {
					return os.WriteFile(release, nil, 0600)
				}
				return nil
			})
			report := decodeReport(t, result)
			if fail {
				if !errors.Is(err, callbackErr) || result.Outcome != tool.OutcomeUnknown || !strings.Contains(report.Stdout.Data, "first") || report.Cancelled {
					t.Fatalf("回调失败丢失事实或输出: %s %v", result.Output, err)
				}
			} else if err != nil || result.Outcome != tool.OutcomeApplied || stdout.String() != "first中文\nsecond\n" || stderr.String() != "diagnostic\n" {
				t.Fatalf("未实时读取双流: %s %v", result.Output, err)
			}
		})
	}
}

func TestCommandLargeOutputRemainsBounded(t *testing.T) {
	helper := buildHelper(t)
	e := testExecutor(t, os.Environ())
	p := prepareTest(t, e, tool.Scope{}, []string{helper, "large"}, t.TempDir())
	count, emitted := 0, 0
	result, err := e.ExecuteStream(t.Context(), p, p.Hash(), func(chunk tool.OutputChunk) error { count++; emitted += len(chunk.Data); return nil })
	report := decodeReport(t, result)
	if err != nil || result.Outcome != tool.OutcomeApplied || len(result.Output) > tool.MaxResultBytes || count > tool.MaxOutputEvents || emitted > tool.MaxOutputBytes || !report.StreamTruncated || !report.Stdout.Truncated || !report.Stderr.Truncated || report.Stdout.Bytes != 800000 || report.Stderr.Bytes != 200000 || report.Stderr.Encoding != "base64" {
		t.Fatalf("输出限制或排空失败: bytes=%d events=%d output=%s err=%v", len(result.Output), count, result.Output, err)
	}
}

func TestCommandCancellationTimeoutAndClose(t *testing.T) {
	helper := buildHelper(t)
	for _, mode := range []string{"cancel", "timeout", "close"} {
		t.Run(mode, func(t *testing.T) {
			raw, err := json.Marshal(arguments{Command: []string{helper, "sleep"}, Cwd: t.TempDir()})
			if err != nil {
				t.Fatal(err)
			}
			inv, err := New(os.Environ(), tool.ProtectedData{}).Prepare(t.Context(), tool.Scope{}, raw)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := inv.Close(); err != nil {
					t.Error(err)
				}
			})
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if mode == "timeout" {
				var end context.CancelFunc
				ctx, end = context.WithTimeout(ctx, 200*time.Millisecond)
				defer end()
			}
			type completed struct {
				result tool.Result
				err    error
			}
			done, ready := make(chan completed, 1), make(chan struct{}, 1)
			go func() {
				result, err := inv.(tool.StreamingInvocation).ExecuteStream(ctx, func(tool.OutputChunk) error {
					select {
					case ready <- struct{}{}:
					default:
					}
					if mode == "cancel" {
						cancel()
					}
					return nil
				})
				done <- completed{result, err}
			}()
			select {
			case <-ready:
			case <-time.After(2 * time.Second):
				t.Fatal("进程未启动")
			}
			if mode == "close" {
				if err := inv.Close(); err != nil {
					t.Fatal(err)
				}
			}
			select {
			case value := <-done:
				report := decodeReport(t, value.result)
				want := context.Canceled
				if mode == "timeout" {
					want = context.DeadlineExceeded
				}
				if !errors.Is(value.err, want) || value.result.Outcome != tool.OutcomeUnknown || !strings.Contains(report.Stdout.Data, "started") || report.ExitCode == nil {
					t.Fatalf("取消状态或输出丢失: %s %v", value.result.Output, value.err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("取消或关闭没有等待进程退出")
			}
			if err := inv.Close(); err != nil {
				t.Fatal(err)
			}
			if result, err := inv.Execute(t.Context()); !errors.Is(err, tool.ErrDenied) || result.Outcome != tool.OutcomeNotApplied {
				t.Fatal("调用对象被重复执行")
			}
		})
	}
}

func TestCommandPathsAreExplicitAndRechecked(t *testing.T) {
	helper := buildHelper(t)
	cwd := t.TempDir()
	instance := New([]string{"PATH=" + filepath.Dir(helper)}, tool.ProtectedData{})
	raw, err := json.Marshal(arguments{Command: []string{filepath.Base(helper), "exit", "0"}})
	if err != nil {
		t.Fatal(err)
	}
	if inv, err := instance.Prepare(t.Context(), tool.Scope{}, raw); !errors.Is(err, tool.ErrPathBase) {
		if inv != nil {
			inv.Close()
		}
		t.Fatalf("使用了进程隐含 cwd: %v", err)
	}
	inv, err := instance.Prepare(t.Context(), tool.Scope{ID: "w", RootPath: cwd}, raw)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := inv.Close(); err != nil {
			t.Error(err)
		}
	}()
	moved := cwd + "-moved"
	if err := os.Rename(cwd, moved); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(moved); err != nil {
			t.Error(err)
		}
	})
	if err := os.Mkdir(cwd, 0700); err != nil {
		t.Fatal(err)
	}
	if result, err := inv.Execute(t.Context()); !errors.Is(err, tool.ErrDenied) || result.Outcome != tool.OutcomeNotApplied {
		t.Fatalf("工作目录替换后仍执行: %v", err)
	}
}

func testProcessTree(t *testing.T) {
	t.Helper()
	helper := buildHelper(t)
	for _, mode := range []string{"spawn", "spawn-exit"} {
		t.Run(mode, func(t *testing.T) {
			e := testExecutor(t, os.Environ())
			heartbeat := filepath.Join(t.TempDir(), "heartbeat")
			p := prepareTest(t, e, tool.Scope{}, []string{helper, mode, heartbeat}, t.TempDir())
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			result, err := e.ExecuteStream(ctx, p, p.Hash(), func(chunk tool.OutputChunk) error {
				if mode == "spawn" && strings.Contains(string(chunk.Data), "child-ready") {
					cancel()
				}
				return nil
			})
			report := decodeReport(t, result)
			if mode == "spawn" {
				if !errors.Is(err, context.Canceled) || result.Outcome != tool.OutcomeUnknown {
					t.Fatalf("取消事实不符: %s %v", result.Output, err)
				}
			} else if err != nil || result.Outcome != tool.OutcomeApplied {
				t.Fatalf("父进程退出失败: %s %v", result.Output, err)
			}
			if !report.CleanupComplete {
				t.Fatalf("进程组/Job 没有清理完毕: %s", result.Output)
			}
			before, err := os.ReadFile(heartbeat)
			if err != nil {
				t.Fatal(err)
			}
			timer := time.NewTimer(120 * time.Millisecond)
			defer timer.Stop()
			select {
			case <-timer.C:
			case <-t.Context().Done():
				t.Fatal(t.Context().Err())
			}
			after, err := os.ReadFile(heartbeat)
			if err != nil || string(before) != string(after) {
				t.Fatalf("子进程仍然写入: %s %s %v", before, after, err)
			}
		})
	}
}

func TestExecutableReplacementDuringApprovalIsDenied(t *testing.T) {
	helper := buildHelper(t)
	data, err := os.ReadFile(helper)
	if err != nil {
		t.Fatal(err)
	}
	copyPath := filepath.Join(t.TempDir(), filepath.Base(helper))
	if err := os.WriteFile(copyPath, data, 0700); err != nil {
		t.Fatal(err)
	}
	e := testExecutor(t, os.Environ())
	p := prepareTest(t, e, tool.Scope{}, []string{copyPath, "exit", "0"}, t.TempDir())
	if err := os.Rename(copyPath, copyPath+".old"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(copyPath, data, 0700); err != nil {
		t.Fatal(err)
	}
	if result, err := e.Execute(t.Context(), p, p.Hash()); !errors.Is(err, tool.ErrDenied) || result.Outcome != tool.OutcomeNotApplied {
		t.Fatalf("可执行文件被替换后仍启动: %s %v", result.Output, err)
	}
}

func TestStartFailureDoesNotClaimExecution(t *testing.T) {
	cwd := t.TempDir()
	name := "bad"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	program := filepath.Join(cwd, name)
	if err := os.WriteFile(program, []byte("invalid executable format"), 0700); err != nil {
		t.Fatal(err)
	}
	e := testExecutor(t, os.Environ())
	p := prepareTest(t, e, tool.Scope{}, []string{program}, cwd)
	result, err := e.Execute(t.Context(), p, p.Hash())
	report := decodeReport(t, result)
	if !errors.Is(err, tool.ErrUnavailable) || result.Outcome != tool.OutcomeNotApplied || report.Started || report.ExitCode != nil {
		t.Fatalf("启动失败被记为已执行: %s %v", result.Output, err)
	}
}

func TestConcurrentCommandsCancelIndependently(t *testing.T) {
	helper := buildHelper(t)
	e := testExecutor(t, os.Environ())
	cwd := t.TempDir()
	first := prepareTest(t, e, tool.Scope{}, []string{helper, "sleep"}, cwd)
	release := filepath.Join(cwd, "release")
	second := prepareTest(t, e, tool.Scope{}, []string{helper, "stream", release}, cwd)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	type finished struct {
		result tool.Result
		err    error
	}
	firstDone, secondDone := make(chan finished, 1), make(chan finished, 1)
	ready := make(chan struct{}, 2)
	go func() {
		result, err := e.ExecuteStream(ctx, first, first.Hash(), func(tool.OutputChunk) error {
			select {
			case ready <- struct{}{}:
			default:
			}
			return nil
		})
		firstDone <- finished{result, err}
	}()
	go func() {
		result, err := e.ExecuteStream(t.Context(), second, second.Hash(), func(tool.OutputChunk) error {
			select {
			case ready <- struct{}{}:
			default:
			}
			return nil
		})
		secondDone <- finished{result, err}
	}()
	for range 2 {
		select {
		case <-ready:
		case <-time.After(2 * time.Second):
			t.Fatal("并发命令未启动")
		}
	}
	cancel()
	select {
	case value := <-firstDone:
		if !errors.Is(value.err, context.Canceled) || value.result.Outcome != tool.OutcomeUnknown {
			t.Fatalf("独立取消失败: %v", value.err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("取消未结束")
	}
	if err := os.WriteFile(release, nil, 0600); err != nil {
		t.Fatal(err)
	}
	select {
	case value := <-secondDone:
		if value.err != nil || value.result.Outcome != tool.OutcomeApplied || !strings.Contains(decodeReport(t, value.result).Stdout.Data, "second") {
			t.Fatalf("取消影响另一命令: %s %v", value.result.Output, value.err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("独立命令未完成")
	}
}
