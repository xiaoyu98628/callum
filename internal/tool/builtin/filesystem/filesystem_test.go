package filesystem_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/xiaoyu98628/callum/internal/tool"
	"github.com/xiaoyu98628/callum/internal/tool/builtin/filesystem"
	"github.com/xiaoyu98628/callum/internal/tool/executor"
	"github.com/xiaoyu98628/callum/internal/tool/registry"
)

func TestWorkspaceToolsAndPermissionBoundary(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("first\nneedle here\nlast"), 0600); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret"), []byte("outside"), 0600); err != nil {
		t.Fatal(err)
	}
	r := registry.New()
	for _, instance := range filesystem.Tools() {
		if err := r.Register(instance); err != nil {
			t.Fatal(err)
		}
	}
	e, err := executor.New(r, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	scope := tool.Scope{ID: "workspace", RootPath: dir}
	for _, test := range []struct{ Name, Args string }{{"list_directory", `{"path":"."}`}, {"read_file", `{"path":"a.txt","start_line":2,"end_line":2}`}, {"search_text", `{"path":".","query":"needle"}`}} {
		prepared, err := e.Prepare(t.Context(), scope, test.Name, json.RawMessage(test.Args))
		if err != nil {
			t.Fatal(err)
		}
		output, callErr := e.Execute(t.Context(), prepared, prepared.Hash())
		closeErr := prepared.Close()
		if callErr != nil || closeErr != nil || !json.Valid(output) {
			t.Fatalf("%s: %s %v %v", test.Name, output, callErr, closeErr)
		}
	}
	for _, args := range []string{`{"path":"../secret"}`, `{"path":"/etc/passwd"}`, `{"path":"a.txt","unknown":true}`, `{"path":"a.txt","start_line":-1}`} {
		if p, err := e.Prepare(t.Context(), scope, "read_file", json.RawMessage(args)); err == nil {
			p.Close()
			t.Fatalf("接受越界或无效参数: %s", args)
		}
	}
	if err := os.Symlink(filepath.Join(outside, "secret"), filepath.Join(dir, "link")); err != nil {
		t.Skipf("当前系统无法创建符号链接: %v", err)
	}
	p, err := e.Prepare(t.Context(), scope, "read_file", json.RawMessage(`{"path":"link"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	if _, err := e.Execute(t.Context(), p, "tampered"); !errors.Is(err, tool.ErrDenied) {
		t.Fatal("未核对审批操作哈希")
	}
	if _, err := e.Execute(t.Context(), p, p.Hash()); !errors.Is(err, tool.ErrDenied) {
		t.Fatalf("符号链接越界: %v", err)
	}
}
