//go:build darwin

package run

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/xiaoyu98628/callum/internal/tool"
	"github.com/xiaoyu98628/callum/internal/tool/policy"
)

func prepareSandboxTest(t *testing.T, root string, command []string, mode tool.SandboxMode, elevated bool) (*invocation, string) {
	t.Helper()
	a := arguments{Command: command, Cwd: root}
	if elevated {
		a.SandboxPermissions, a.Justification = "require_escalated", "在临时测试目录验证本次明确授权"
	}
	raw, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	call, err := New(os.Environ()).(*commandTool).PrepareWithPermissions(t.Context(), tool.Scope{ID: "test-workspace", RootPath: root}, raw, tool.ExecutionPermissions{SandboxMode: mode})
	if err != nil {
		t.Fatal(err)
	}
	invocation := call.(*invocation)
	t.Cleanup(func() {
		if err := invocation.Close(); err != nil {
			t.Error(err)
		}
	})
	if _, err := os.Stat(invocation.fixed.temporaryPath); invocation.fixed.boundary.Sandboxed && !errors.Is(err, os.ErrNotExist) {
		t.Fatal("准备期间创建了沙箱目录")
	}
	return invocation, invocation.fixed.temporaryPath
}

func TestSeatbeltEnforcesWritesProcessesAndNetwork(t *testing.T) {
	helper := buildHelper(t)
	root, outside := t.TempDir(), t.TempDir()
	for _, name := range []string{".git", ".codex", ".agents", ".aws"} {
		if err := os.Mkdir(filepath.Join(root, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(outside, filepath.Join(root, "linked")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "hardlink-source"), []byte("unchanged"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name    string
		command []string
		mode    tool.SandboxMode
		success bool
	}{
		{"pwd", []string{"/bin/pwd"}, tool.SandboxWorkspaceWrite, true},
		{"workspace-write", []string{helper, "write", filepath.Join(root, "ordinary")}, tool.SandboxWorkspaceWrite, true},
		{"outside-write", []string{helper, "write", filepath.Join(outside, "blocked")}, tool.SandboxWorkspaceWrite, false},
		{"symlink-write", []string{helper, "write", filepath.Join(root, "linked", "blocked")}, tool.SandboxWorkspaceWrite, false},
		{"new-hardlink", []string{"/bin/ln", filepath.Join(outside, "hardlink-source"), filepath.Join(root, "hardlink")}, tool.SandboxWorkspaceWrite, false},
		{"git-write", []string{helper, "write", filepath.Join(root, ".git", "blocked")}, tool.SandboxWorkspaceWrite, false},
		{"agent-write", []string{helper, "write", filepath.Join(root, ".agents", "blocked")}, tool.SandboxWorkspaceWrite, false},
		{"codex-write", []string{helper, "write", filepath.Join(root, ".codex", "blocked")}, tool.SandboxWorkspaceWrite, false},
		{"aws-write", []string{helper, "write", filepath.Join(root, ".aws", "blocked")}, tool.SandboxWorkspaceWrite, false},
		{"readonly-write", []string{helper, "write", filepath.Join(root, "readonly-blocked")}, tool.SandboxReadOnly, false},
		{"child-outside-write", []string{"/bin/sh", "-c", `/bin/sh -c 'printf blocked > "$1"' sh "$1"`, "sh", filepath.Join(outside, "child-blocked")}, tool.SandboxWorkspaceWrite, false},
		{"private-temporary-write", []string{"/bin/sh", "-c", `printf allowed > "$TMPDIR/test"`}, tool.SandboxWorkspaceWrite, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			call, tmp := prepareSandboxTest(t, root, test.command, test.mode, false)
			if decision := policy.EvaluateCommand(policy.OnRequest, call.CommandBoundary()); decision.Action != policy.Allow {
				t.Fatalf("普通沙箱命令未自动放行: %+v", decision)
			}
			result, err := call.Execute(t.Context())
			if err != nil {
				t.Fatalf("沙箱执行失败: %s %v", result.Output, err)
			}
			report := decodeReport(t, result)
			if report.ExitCode == nil || (*report.ExitCode == 0) != test.success {
				t.Fatalf("边界未生效: %s", result.Output)
			}
			if !report.Boundary.Sandboxed {
				t.Fatal("未记录真实沙箱边界")
			}
			if _, err := os.Stat(tmp); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("临时目录未清理: %v", err)
			}
		})
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	call, _ := prepareSandboxTest(t, root, []string{"/usr/bin/curl", "--max-time", "2", "http://" + listener.Addr().String()}, tool.SandboxWorkspaceWrite, false)
	result, err := call.Execute(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if report := decodeReport(t, result); report.ExitCode == nil || *report.ExitCode == 0 {
		t.Fatalf("沙箱允许联网: %s", result.Output)
	}
	if err := listener.(*net.TCPListener).SetDeadline(time.Now().Add(50 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if connection, err := listener.Accept(); err == nil {
		connection.Close()
		t.Fatal("联网请求到达了本地监听器")
	}
	for _, path := range []string{filepath.Join(outside, "blocked"), filepath.Join(outside, "child-blocked"), filepath.Join(root, ".git", "blocked")} {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("受限写入生效: %s %v", path, err)
		}
	}
}

func TestSeatbeltCancellationCleansProcessGroupAndTemporaryDirectory(t *testing.T) {
	helper, root := buildHelper(t), t.TempDir()
	heartbeat := filepath.Join(root, "heartbeat")
	call, tmp := prepareSandboxTest(t, root, []string{helper, "spawn", heartbeat}, tool.SandboxWorkspaceWrite, false)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	result, err := call.ExecuteStream(ctx, func(chunk tool.OutputChunk) error {
		if strings.Contains(string(chunk.Data), "child-ready") {
			cancel()
		}
		return nil
	})
	if !errors.Is(err, context.Canceled) || !decodeReport(t, result).CleanupComplete {
		t.Fatalf("取消未完整收尾: %s %v", result.Output, err)
	}
	first, err := os.ReadFile(heartbeat)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(80 * time.Millisecond)
	second, err := os.ReadFile(heartbeat)
	if err != nil || string(first) != string(second) {
		t.Fatal("取消后子进程仍在写入")
	}
	if _, err := os.Stat(tmp); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("取消后私有临时目录未清理")
	}
}

func TestSeatbeltEscalationAndStableOperationIdentity(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	first, _ := prepareSandboxTest(t, root, []string{"/bin/pwd"}, tool.SandboxWorkspaceWrite, false)
	second, _ := prepareSandboxTest(t, root, []string{"/bin/pwd"}, tool.SandboxWorkspaceWrite, false)
	if string(first.Snapshot()) != string(second.Snapshot()) {
		t.Fatal("私有临时目录随机名改变了操作身份")
	}
	call, _ := prepareSandboxTest(t, root, []string{"/usr/bin/touch", filepath.Join(outside, "explicitly-approved")}, tool.SandboxWorkspaceWrite, true)
	boundary := call.CommandBoundary()
	if boundary.Sandboxed || !boundary.Escalated || policy.EvaluateCommand(policy.OnRequest, boundary).Action != policy.Ask || policy.EvaluateCommand(policy.Never, boundary).Action != policy.Deny {
		t.Fatal("越界请求未独立审批")
	}
	result, err := call.Execute(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if report := decodeReport(t, result); report.ExitCode == nil || *report.ExitCode != 0 || report.Boundary.Sandboxed {
		t.Fatalf("明确授权后的执行边界错误: %s", result.Output)
	}
	if _, err := os.Stat(filepath.Join(outside, "explicitly-approved")); err != nil {
		t.Fatal(err)
	}
}
