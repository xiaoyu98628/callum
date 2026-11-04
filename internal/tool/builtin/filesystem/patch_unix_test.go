//go:build darwin || linux

package filesystem

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"

	"github.com/xiaoyu98628/callum/internal/tool"
)

func TestPatchRejectsSpecialFilesAndHardLinkAliases(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pipe")
	if err := syscall.Mkfifo(path, 0600); err != nil {
		t.Fatal(err)
	}
	e := patchTestExecutor(t)
	scope := tool.Scope{ID: "w", RootPath: dir}
	if _, err := e.Prepare(t.Context(), scope, "apply_patch", patchTestArgs(t, "*** Update File: pipe\n@@\n-a\n+b", patchVersion{"pipe", textSHA256([]byte("a\n"))})); err == nil {
		t.Fatal("打开了特殊文件")
	}
	file := filepath.Join(dir, "file")
	patchTestWrite(t, file, "a\n")
	if err := os.Link(file, filepath.Join(t.TempDir(), "alias")); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Prepare(t.Context(), scope, "apply_patch", patchTestArgs(t, "*** Update File: file\n@@\n-a\n+b", patchVersion{"file", textSHA256([]byte("a\n"))})); !errors.Is(err, tool.ErrDenied) {
		t.Fatalf("硬链接可能修改未批准路径: %v", err)
	}
}

func TestPatchPreservesUnixPermissionsAndACL(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note")
	before := "header\nold\ntail\n"
	patchTestWrite(t, path, before)
	if err := os.Chmod(path, 0750); err != nil {
		t.Fatal(err)
	}
	var aclBefore string
	if runtime.GOOS == "darwin" {
		// 仅对临时文件设置 ACL。禁止执行这一项不影响读写和测试目录清理。
		if output, err := exec.CommandContext(t.Context(), "/bin/chmod", "+a", "everyone deny execute", path).CombinedOutput(); err != nil {
			t.Fatalf("设置临时 ACL: %s %v", output, err)
		}
		output, err := exec.CommandContext(t.Context(), "/bin/ls", "-le", path).Output()
		if err != nil {
			t.Fatal(err)
		}
		aclBefore = strings.Join(strings.Split(string(output), "\n")[1:], "\n")
	}
	e := patchTestExecutor(t)
	p := patchTestPrepare(t, e, tool.Scope{ID: "w", RootPath: dir}, patchTestArgs(t, "*** Update File: note\n@@\n header\n-old\n+new\n tail", patchVersion{"note", textSHA256([]byte(before))}))
	if result, err := e.Execute(t.Context(), p, p.Hash()); err != nil || result.Outcome != tool.OutcomeApplied {
		t.Fatalf("修改失败: %s %v", result.Output, err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0750 {
		t.Fatalf("权限改变: %v %v", info, err)
	}
	if runtime.GOOS == "darwin" {
		output, err := exec.CommandContext(t.Context(), "/bin/ls", "-le", path).Output()
		if err != nil {
			t.Fatal(err)
		}
		aclAfter := strings.Join(strings.Split(string(output), "\n")[1:], "\n")
		if aclBefore != aclAfter {
			t.Fatalf("改变了原 ACL: %q -> %q", aclBefore, aclAfter)
		}
	}
}

type denyPatchCleanup struct {
	context.Context
	path, dir string
	applied   bool
}

func (c *denyPatchCleanup) Err() error {
	if !c.applied {
		data, _ := os.ReadFile(c.path)
		if string(data) == "head\nnew\ntail\n" {
			c.applied = true
			_ = os.Chmod(c.dir, 0500)
		}
	}
	return c.Context.Err()
}
func TestPatchCleanupErrorKeepsAppliedOutputAndCopies(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("特权用户不受目录权限拒绝约束")
	}
	dir := t.TempDir()
	t.Cleanup(func() {
		if err := os.Chmod(dir, 0700); err != nil {
			t.Error(err)
		}
	})
	path := filepath.Join(dir, "note")
	before := "head\nold\ntail\n"
	patchTestWrite(t, path, before)
	instance := newPatchTool()
	args, err := instance.Validate(patchTestArgs(t, "*** Update File: note\n@@\n head\n-old\n+new\n tail", patchVersion{"note", textSHA256([]byte(before))}))
	if err != nil {
		t.Fatal(err)
	}
	inv, err := instance.Prepare(t.Context(), tool.Scope{ID: "w", RootPath: dir}, args)
	if err != nil {
		t.Fatal(err)
	}
	defer inv.Close()
	ctx := &denyPatchCleanup{Context: t.Context(), path: path, dir: dir}
	result, err := inv.Execute(ctx)
	if err == nil || result.Outcome != tool.OutcomeApplied {
		t.Fatalf("清理错误掩盖已生效结果: %s %v", result.Output, err)
	}
	var report patchReport
	if json.Unmarshal(result.Output, &report) != nil || len(report.RecoveryFiles) != 2 || report.Changes[0].ObservedSHA256 != textSHA256([]byte("head\nnew\ntail\n")) {
		t.Fatalf("清理失败丢失审计: %s", result.Output)
	}
	if patchTestRead(t, path) != "head\nnew\ntail\n" {
		t.Fatal("实际修改被回滚")
	}
}

func TestPatchPrivateCopiesDoNotInheritDarwinReadACL(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Darwin ACL 继承边界")
	}
	dir := t.TempDir()
	if output, err := exec.CommandContext(t.Context(), "/bin/chmod", "+a", "everyone allow read,file_inherit,directory_inherit", dir).CombinedOutput(); err != nil {
		t.Fatalf("设置临时继承 ACL: %s %v", output, err)
	}
	path := filepath.Join(dir, "private-copy")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	secureErr := protectPatchFile(file)
	closeErr := file.Close()
	if secureErr != nil || closeErr != nil {
		t.Fatalf("收敛新文件权限: %v %v", secureErr, closeErr)
	}
	output, err := exec.CommandContext(t.Context(), "/bin/ls", "-le", path).Output()
	if err != nil {
		t.Fatal(err)
	}
	if len(strings.Split(strings.TrimSpace(string(output)), "\n")) != 1 {
		t.Fatalf("私有副本仍有继承 ACL: %s", output)
	}
	newDir := filepath.Join(dir, "child")
	if err := os.Mkdir(newDir, 0700); err != nil {
		t.Fatal(err)
	}
	directory, err := os.Open(newDir)
	if err != nil {
		t.Fatal(err)
	}
	secureErr = protectPatchDirectory(directory)
	closeErr = directory.Close()
	if secureErr != nil || closeErr != nil {
		t.Fatalf("收敛新目录权限: %v %v", secureErr, closeErr)
	}
	output, err = exec.CommandContext(t.Context(), "/bin/ls", "-led", newDir).Output()
	if err != nil {
		t.Fatal(err)
	}
	if len(strings.Split(strings.TrimSpace(string(output)), "\n")) != 1 {
		t.Fatalf("私有目录仍有继承 ACL: %s", output)
	}
}
