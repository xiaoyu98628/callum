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

	"golang.org/x/sys/windows"

	"github.com/xiaoyu98628/callum/internal/tool"
	"github.com/xiaoyu98628/callum/internal/tool/policy"
)

func prepareWindowsSandbox(t *testing.T, root, data string, command []string, write bool) *invocation {
	t.Helper()
	a := arguments{Command: command, Cwd: root}
	if write {
		a.SandboxPermissions, a.Justification = sandboxProjectWrite, "验证临时项目的原生隔离"
	}
	raw, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	instance := New(os.Environ(), tool.ProtectedData{Directory: data}).(*commandTool)
	call, err := instance.PrepareWithPermissions(t.Context(), tool.Scope{ID: "windows-fixture", RootPath: root}, raw, tool.ExecutionPermissions{SandboxMode: tool.SandboxWorkspaceWrite})
	if err != nil {
		t.Fatal(err)
	}
	p := call.(*invocation)
	t.Cleanup(func() {
		if err := p.Close(); err != nil {
			t.Error(err)
		}
	})
	if _, err := os.Stat(p.fixed.temporaryPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("审批准备创建了运行资源")
	}
	return p
}

func TestAppContainerFileNetworkAndChildBoundaries(t *testing.T) {
	helper := buildHelper(t)
	root, outside, data := t.TempDir(), t.TempDir(), t.TempDir()
	for _, name := range []string{".git", ".codex", ".agents", ".aws"} {
		if err := os.Mkdir(filepath.Join(root, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	secret := filepath.Join(data, "secret")
	if err := os.WriteFile(secret, []byte("private"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, action, path string
		write, success     bool
	}{
		{"read-only-write", "write", filepath.Join(root, "blocked"), false, false},
		{"project-write", "write", filepath.Join(root, "allowed"), true, true},
		{"outside-write", "write", filepath.Join(outside, "blocked"), true, false},
		{"child-outside-write", "spawn-write", filepath.Join(outside, "child-blocked"), true, false},
		{"git-write", "write", filepath.Join(root, ".git", "blocked"), true, false},
		{"codex-write", "write", filepath.Join(root, ".codex", "blocked"), true, false},
		{"agents-write", "write", filepath.Join(root, ".agents", "blocked"), true, false},
		{"aws-write", "write", filepath.Join(root, ".aws", "blocked"), true, false},
		{"app-data-read", "read", secret, false, false},
		{"app-data-write", "write", secret, true, false},
		{"outside-acl", "relax-acl", outside, true, false},
		{"git-acl", "relax-acl", filepath.Join(root, ".git"), true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := prepareWindowsSandbox(t, root, data, []string{helper, tc.action, tc.path}, tc.write)
			want := policy.Allow
			if tc.write {
				want = policy.Ask
			}
			if decision := policy.EvaluateCommand(policy.OnRequest, p.CommandBoundary()); decision.Action != want {
				t.Fatalf("审批判定错误: %+v", decision)
			}
			result, err := p.Execute(t.Context())
			if err != nil {
				t.Fatalf("原生沙箱执行失败: %v %s", err, result.Output)
			}
			report := decodeReport(t, result)
			if report.ExitCode == nil || (*report.ExitCode == 0) != tc.success {
				t.Fatalf("沙箱边界错误: %s", result.Output)
			}
			if !report.Boundary.Sandboxed || !report.CleanupComplete {
				t.Fatalf("沙箱或清理状态错误: %s", result.Output)
			}
		})
	}
	if contents, err := os.ReadFile(secret); err != nil || string(contents) != "private" {
		t.Fatalf("应用数据被修改: %q %v", contents, err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := listener.Close(); err != nil {
			t.Error(err)
		}
	})
	p := prepareWindowsSandbox(t, root, data, []string{helper, "connect", listener.Addr().String()}, false)
	result, err := p.Execute(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if report := decodeReport(t, result); report.ExitCode == nil || *report.ExitCode == 0 {
		t.Fatalf("沙箱允许连接回环网络: %s", result.Output)
	}
	if err := listener.(*net.TCPListener).SetDeadline(time.Now().Add(50 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if connection, err := listener.Accept(); err == nil {
		if err := connection.Close(); err != nil {
			t.Error(err)
		}
		t.Fatal("受限连接到达本机监听器")
	}
}

func TestAppContainerCannotWriteOutsideNullDACL(t *testing.T) {
	helper := buildHelper(t)
	root, outside, data := t.TempDir(), t.TempDir(), t.TempDir()
	path := filepath.Join(outside, "world-writable")
	if err := os.WriteFile(path, []byte("unchanged"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	p := prepareWindowsSandbox(t, root, data, []string{helper, "write", path}, false)
	result, err := p.Execute(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if report := decodeReport(t, result); report.ExitCode == nil || *report.ExitCode == 0 {
		t.Fatalf("NULL DACL 绕过了写入边界: %s", result.Output)
	}
	contents, err := os.ReadFile(path)
	if err != nil || string(contents) != "unchanged" {
		t.Fatalf("项目外文件被修改: %q %v", contents, err)
	}
}

func TestAppContainerPrivateTemporaryAndCancellation(t *testing.T) {
	helper := buildHelper(t)
	root, data := t.TempDir(), t.TempDir()
	p := prepareWindowsSandbox(t, root, data, []string{helper, "write-temp"}, false)
	result, err := p.Execute(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if report := decodeReport(t, result); report.ExitCode == nil || *report.ExitCode != 0 {
		t.Fatalf("私有临时目录不可写: %s", result.Output)
	}
	if _, err := os.Stat(p.fixed.temporaryPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("临时目录未回收: %v", err)
	}
	p = prepareWindowsSandbox(t, root, data, []string{helper, "sleep"}, false)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	result, err = p.ExecuteStream(ctx, func(chunk tool.OutputChunk) error {
		if strings.Contains(string(chunk.Data), "started") {
			cancel()
		}
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("取消错误丢失: %s %v", result.Output, err)
	}
	if report := decodeReport(t, result); !report.CleanupComplete {
		t.Fatalf("取消未收回进程资源: %s", result.Output)
	}
}

func TestSandboxACLRecoveryPreservesExistingPermissions(t *testing.T) {
	helper := buildHelper(t)
	root, data := t.TempDir(), t.TempDir()
	before, err := windows.GetNamedSecurityInfo(root, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	p := prepareWindowsSandbox(t, root, data, []string{helper, "inspect"}, false)
	_, err = p.fixed.setupExecution(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	state := &p.fixed.sandboxState
	if err := state.token.Close(); err != nil {
		t.Fatal(err)
	}
	state.token, state.active = 0, false
	if err := RecoverSandbox(t.Context(), data); err != nil {
		t.Fatal(err)
	}
	after, err := windows.GetNamedSecurityInfo(root, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	if before.String() != after.String() {
		t.Fatalf("恢复改写了原有 ACL: %s -> %s", before.String(), after.String())
	}
	if err := RecoverSandbox(t.Context(), data); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(data, "command-sandboxes"))
	if err != nil || len(entries) != 0 {
		t.Fatalf("恢复记录未清理: %v %v", entries, err)
	}
}

func TestSandboxConcurrentACLAndReplacedFileCleanup(t *testing.T) {
	helper := buildHelper(t)
	root, data := t.TempDir(), t.TempDir()
	path := filepath.Join(root, "replaced.txt")
	if err := os.WriteFile(path, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	first := prepareWindowsSandbox(t, root, data, []string{helper, "inspect"}, true)
	second := prepareWindowsSandbox(t, root, data, []string{helper, "inspect"}, false)
	for _, p := range []*invocation{first, second} {
		if _, err := p.fixed.setupExecution(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	// 模拟编辑器以新文件替换原目标，并产生继承了执行 SID 的新对象。
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("replacement"), 0600); err != nil {
		t.Fatal(err)
	}
	newData := filepath.Join(data, "created-while-active")
	if err := os.WriteFile(newData, []byte("private"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := first.fixed.sandboxState.cleanup(); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{root, path, newData} {
		sd, err := windows.GetNamedSecurityInfo(target, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(sd.String(), first.fixed.sandboxState.sid.String()) {
			t.Fatalf("已结束执行的 SID 未移除: %s", target)
		}
		if !strings.Contains(sd.String(), second.fixed.sandboxState.sid.String()) {
			t.Fatalf("其他执行的 SID 被清除: %s", target)
		}
	}
	if err := second.fixed.sandboxState.cleanup(); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{root, path, newData} {
		sd, err := windows.GetNamedSecurityInfo(target, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(sd.String(), second.fixed.sandboxState.sid.String()) {
			t.Fatalf("剩余执行的 SID 未移除: %s", target)
		}
	}
}

func TestAppContainerCannotBorrowConcurrentWriteGrant(t *testing.T) {
	helper := buildHelper(t)
	root, data := t.TempDir(), t.TempDir()
	path := filepath.Join(root, "concurrent-write")
	readonly := prepareWindowsSandbox(t, root, data, []string{helper, "write", path}, false)
	writable := prepareWindowsSandbox(t, root, data, []string{helper, "write", path}, true)
	for _, p := range []*invocation{readonly, writable} {
		if _, err := p.fixed.setupExecution(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	report, err := runProcess(t.Context(), readonly.fixed, nil)
	if err != nil || report.ExitCode == nil || *report.ExitCode == 0 || !report.CleanupComplete {
		t.Fatalf("只读执行借用了并发写入授权: %+v %v", report, err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("只读执行创建了文件: %v", err)
	}
	report, err = runProcess(t.Context(), writable.fixed, nil)
	if err != nil || report.ExitCode == nil || *report.ExitCode != 0 || !report.CleanupComplete {
		t.Fatalf("独立写入授权不可用: %+v %v", report, err)
	}
}
