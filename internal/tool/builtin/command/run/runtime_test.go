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
	for _, name := range []string{python, node} {
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
	manifest := runtimeassets.Manifest{FormatVersion: 1, Platform: runtime.GOOS, Arch: runtime.GOARCH, Python: runtimeassets.Executable{Version: "controlled", Path: python}, Node: runtimeassets.Executable{Version: "controlled", Path: node}, Files: files}
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
	for _, name := range []string{"python", "python3", "node"} {
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
	for _, command := range [][]string{{"python", "-c", python}, {"node", "-e", node}, {"python", "-c", "print('x'*100000)"}} {
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
