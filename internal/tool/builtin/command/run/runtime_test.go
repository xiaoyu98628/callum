package run

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/xiaoyu98628/callum/internal/runtimeassets"
	"github.com/xiaoyu98628/callum/internal/tool"
)

func bundledFixture(t *testing.T, parent, helper string) *runtimeassets.Bundle {
	t.Helper()
	directory := filepath.Join(parent, "resources with spaces")
	data, err := os.ReadFile(helper)
	if err != nil {
		t.Fatal(err)
	}
	python, node := "python/bin/python3", "node/bin/node"
	if runtime.GOOS == "windows" {
		python, node = "python/bin/python.exe", "node/bin/node.exe"
	}
	for _, name := range []string{python, node, "git/bin/git", "git/etc/gitconfig", "git/libexec/git-core/helper", "git/share/templates/template"} {
		full := filepath.Join(directory, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, data, 0755); err != nil {
			t.Fatal(err)
		}
	}
	files, err := runtimeassets.Inventory(t.Context(), directory)
	if err != nil {
		t.Fatal(err)
	}
	manifest := runtimeassets.Manifest{FormatVersion: runtimeassets.FormatVersion, Platform: runtime.GOOS, Arch: runtime.GOARCH, Python: runtimeassets.Executable{Version: "controlled", Path: python}, Node: runtimeassets.Executable{Version: "controlled", Path: node}, Git: runtimeassets.GitRuntime{Executable: runtimeassets.Executable{Version: "controlled", Path: "git/bin/git"}, ExecPath: "git/libexec/git-core", TemplatePath: "git/share/templates", ConfigPath: "git/etc/gitconfig"}, Files: files}
	if err := runtimeassets.WriteManifest(directory, manifest); err != nil {
		t.Fatal(err)
	}
	bundle, err := runtimeassets.Load(t.Context(), directory)
	if err != nil {
		t.Fatal(err)
	}
	return bundle
}

func TestBundledCommandsUseFixedExecutablesAndPreserveCancellation(t *testing.T) {
	bundle := bundledFixture(t, t.TempDir(), buildHelper(t))
	instance := NewWithRuntime([]string{"PATH=" + filepath.Join(t.TempDir(), "missing"), "OPENAI_API_KEY=must-not-leak"}, tool.ProtectedData{}, bundle).(*commandTool)
	for _, name := range []string{"python", "python3", "node", "git"} {
		raw, err := json.Marshal(arguments{Command: []string{name, "inspect", "a b", "$HOME"}, Cwd: t.TempDir()})
		if err != nil {
			t.Fatal(err)
		}
		call, err := instance.PrepareWithPermissions(t.Context(), tool.Scope{}, raw, tool.ExecutionPermissions{SandboxMode: tool.SandboxFullAccess})
		if err != nil {
			t.Fatal(err)
		}
		fixed := call.(*invocation).fixed
		if !strings.HasPrefix(fixed.executable.actual, bundle.Directory()+string(filepath.Separator)) || environmentValue(fixed.environment, "PYTHONDONTWRITEBYTECODE") != "1" {
			t.Fatal("未固定内置解释器或未隔离缓存")
		}
		result, executeErr := call.Execute(t.Context())
		if err := errors.Join(executeErr, call.Close()); err != nil {
			t.Fatal(err)
		}
		report := decodeReport(t, result)
		if report.ExitCode == nil || *report.ExitCode != 0 || !report.CleanupComplete || strings.Contains(string(result.Output), "must-not-leak") || !strings.Contains(report.Stdout.Data, "$HOME") {
			t.Fatalf("命令映射改变了参数、环境或生命周期: %s", result.Output)
		}
	}
	raw, _ := json.Marshal(arguments{Command: []string{"python", "sleep"}, Cwd: t.TempDir()})
	call, err := instance.PrepareWithPermissions(t.Context(), tool.Scope{}, raw, tool.ExecutionPermissions{SandboxMode: tool.SandboxFullAccess})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	result, err := call.Execute(ctx)
	if !errors.Is(err, context.DeadlineExceeded) || !decodeReport(t, result).CleanupComplete {
		t.Fatalf("内置命令没有按超时回收: %s %v", result.Output, err)
	}
	if err := call.Close(); err != nil {
		t.Fatal(err)
	}
}

// 显式传入本次准备的工作区资源，普通单元测试和 CI 不依赖用户安装的解释器。
func TestBundledInterpreters(t *testing.T) {
	directory := os.Getenv("CALLUM_TEST_RUNTIME_DIR")
	if directory == "" {
		t.Skip("需要本次准备的独立运行时资源")
	}
	bundle, err := runtimeassets.Load(t.Context(), directory)
	if err != nil {
		t.Fatal(err)
	}
	instance := NewWithRuntime([]string{"PATH=" + t.TempDir(), "OPENAI_API_KEY=must-not-leak"}, tool.ProtectedData{Directory: t.TempDir()}, bundle).(*commandTool)
	python := `import sys, ssl, sqlite3, json, subprocess, os; assert sys.version_info[:3] == (3,14,10); assert os.getenv("OPENAI_API_KEY") is None; assert sqlite3.connect(":memory:").execute("select 1").fetchone() == (1,); ssl.SSLContext(ssl.PROTOCOL_TLS_CLIENT); assert subprocess.check_output(["python3", "-c", "print('child-ok')"], text=True).strip() == "child-ok"; print("bundled-python-ok")`
	node := `const assert=require('node:assert/strict'); require('node:fs'); require('node:crypto').randomBytes(4); assert.equal(process.version,'v26.13.0'); assert.equal(process.env.OPENAI_API_KEY,undefined); assert.equal(require('node:child_process').execFileSync('node',['-p',"'child-ok'"],{encoding:'utf8'}).trim(),'child-ok'); console.log('bundled-node-ok')`
	for _, command := range [][]string{{"python3", "-c", python}, {"node", "-e", node}, {"python3", "-c", "print('x'*100000)"}} {
		raw, _ := json.Marshal(arguments{Command: command, Cwd: t.TempDir()})
		call, err := instance.PrepareWithPermissions(t.Context(), tool.Scope{}, raw, tool.ExecutionPermissions{SandboxMode: tool.SandboxReadOnly})
		if err != nil {
			t.Fatal(err)
		}
		result, executeErr := call.Execute(t.Context())
		if err := errors.Join(executeErr, call.Close()); err != nil {
			t.Fatalf("真实解释器执行失败: %s %v", result.Output, err)
		}
		report := decodeReport(t, result)
		if report.ExitCode == nil || *report.ExitCode != 0 || !report.CleanupComplete {
			t.Fatalf("真实解释器未正常完成: %s", result.Output)
		}
		if strings.Contains(command[2], "100000") && !report.Stdout.Truncated {
			t.Fatal("解释器的大量输出未保持有界")
		}
	}
}

// 使用本次准备的 Git 验证真实仓库操作，所有写入限定在测试临时目录。
func TestBundledGit(t *testing.T) {
	directory := os.Getenv("CALLUM_TEST_RUNTIME_DIR")
	if directory == "" {
		t.Skip("需要本次准备的独立运行时资源")
	}
	bundle, err := runtimeassets.Load(t.Context(), directory)
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	instance := NewWithRuntime([]string{"PATH=" + t.TempDir(), "HOME=" + home, "USERPROFILE=" + home}, tool.ProtectedData{Directory: t.TempDir()}, bundle).(*commandTool)
	repo := t.TempDir()
	runGit := func(mode tool.SandboxMode, args ...string) processReport {
		t.Helper()
		raw, _ := json.Marshal(arguments{Command: append([]string{"git"}, args...), Cwd: repo})
		call, err := instance.PrepareWithPermissions(t.Context(), tool.Scope{}, raw, tool.ExecutionPermissions{SandboxMode: mode})
		if err != nil {
			t.Fatal(err)
		}
		result, executeErr := call.Execute(t.Context())
		if err := errors.Join(executeErr, call.Close()); err != nil {
			t.Fatalf("Git 执行失败: %s %v", result.Output, err)
		}
		report := decodeReport(t, result)
		if !report.CleanupComplete || report.ExitCode == nil {
			t.Fatalf("Git 未正常回收: %s", result.Output)
		}
		return report
	}
	success := func(mode tool.SandboxMode, args ...string) string {
		t.Helper()
		report := runGit(mode, args...)
		if *report.ExitCode != 0 {
			t.Fatalf("git %v: exit=%d stdout=%s stderr=%s", args, *report.ExitCode, report.Stdout.Data, report.Stderr.Data)
		}
		return report.Stdout.Data
	}
	success(tool.SandboxReadOnly, "--version")
	// HTTPS 辅助程序在空 stdin 下以 1 结束，不连接端点；查找或依赖加载失败会有诊断。
	remote := runGit(tool.SandboxReadOnly, "remote-https", "origin", "https://example.invalid/not-accessed")
	if *remote.ExitCode != 1 || remote.Stdout.Data != "" || remote.Stderr.Data != "" {
		t.Fatalf("HTTPS 辅助程序未正常处理 EOF: %+v", remote)
	}
	success(tool.SandboxReadOnly, "lfs", "version")
	if got := strings.TrimSpace(success(tool.SandboxReadOnly, "--exec-path")); !strings.HasPrefix(got, bundle.Directory()+string(filepath.Separator)) {
		t.Fatalf("辅助程序没有固定: %s", got)
	}
	success(tool.SandboxFullAccess, "init", "--initial-branch=main")
	if _, err := os.Stat(filepath.Join(repo, ".git", "hooks", "pre-commit.sample")); err != nil {
		t.Fatal("没有使用随附 Git 模板:", err)
	}
	success(tool.SandboxFullAccess, "config", "user.name", "Callum Test")
	success(tool.SandboxFullAccess, "config", "user.email", "callum-test@example.invalid")
	success(tool.SandboxFullAccess, "config", "commit.gpgsign", "false")
	file := filepath.Join(repo, "tracked.txt")
	if err := os.WriteFile(file, []byte("before\n"), 0600); err != nil {
		t.Fatal(err)
	}
	success(tool.SandboxFullAccess, "add", "tracked.txt")
	success(tool.SandboxFullAccess, "commit", "-m", "controlled commit")
	if !strings.Contains(success(tool.SandboxReadOnly, "log", "-1", "--format=%s"), "controlled commit") {
		t.Fatal("无法读取提交")
	}
	if err := os.WriteFile(file, []byte("after\n"), 0600); err != nil {
		t.Fatal(err)
	}
	index := filepath.Join(repo, ".git", "index")
	before, err := os.ReadFile(index)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(index)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(success(tool.SandboxReadOnly, "status", "--short"), "tracked.txt") {
		t.Fatal("状态没有报告修改")
	}
	after, err := os.ReadFile(index)
	if err != nil {
		t.Fatal(err)
	}
	afterInfo, err := os.Stat(index)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) || !info.ModTime().Equal(afterInfo.ModTime()) {
		t.Fatal("只读 status 修改了索引")
	}
	if !strings.Contains(success(tool.SandboxReadOnly, "diff", "--no-ext-diff"), "+after") {
		t.Fatal("无法读取差异")
	}
	if code := *runGit(tool.SandboxReadOnly, "diff", "--exit-code", "--no-ext-diff").ExitCode; code != 1 {
		t.Fatalf("差异退出码丢失: %d", code)
	}
	if runtime.GOOS == "darwin" || runtime.GOOS == "windows" {
		if *runGit(tool.SandboxReadOnly, "add", "tracked.txt").ExitCode == 0 {
			t.Fatal("只读沙箱允许修改 .git")
		}
	}
	clone := filepath.Join(t.TempDir(), "local clone")
	success(tool.SandboxFullAccess, "clone", "--no-local", "--no-hardlinks", repo, clone)
	if data, err := os.ReadFile(filepath.Join(clone, "tracked.txt")); err != nil || string(data) != "before\n" {
		t.Fatalf("本地克隆内容错误: %q %v", data, err)
	}
	// PATH 内没有系统程序，验证解释器子进程同样找到内置 Git。
	python := `import subprocess; print(subprocess.check_output(["git","--version"],text=True))`
	raw, _ := json.Marshal(arguments{Command: []string{"python3", "-c", python}, Cwd: repo})
	call, err := instance.PrepareWithPermissions(t.Context(), tool.Scope{}, raw, tool.ExecutionPermissions{SandboxMode: tool.SandboxReadOnly})
	if err != nil {
		t.Fatal(err)
	}
	result, executeErr := call.Execute(t.Context())
	if err := errors.Join(executeErr, call.Close()); err != nil {
		t.Fatal(err)
	}
	if report := decodeReport(t, result); report.ExitCode == nil || *report.ExitCode != 0 || !strings.Contains(report.Stdout.Data, "git version 2.56.1") {
		t.Fatalf("子进程没有使用内置 Git: %s", result.Output)
	}
}
