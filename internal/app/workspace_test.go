package app

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/xiaoyu98628/callum/internal/config"
	"github.com/xiaoyu98628/callum/internal/workspace"
)

func TestWorkspaceFoldersPersistAcrossRestart(t *testing.T) {
	cfg, err := config.Load(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatal(err)
	}
	p := startApp(t, cfg, io.Discard)
	one := p.call(t, "workspace.create", map[string]any{"name": "单目录", "root_path": t.TempDir()})
	requireOK(t, one)
	if value := readWorkspace(t, one); value.OtherPaths == nil || len(value.OtherPaths) != 0 {
		t.Fatalf("单目录项目应返回空数组: %s", one.Result)
	}

	root, other := t.TempDir(), t.TempDir()
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(other, alias); err != nil {
		t.Fatal(err)
	}
	params := map[string]any{"name": "多目录", "root_path": root, "other_paths": []string{other, alias}}
	if frame := p.call(t, "workspace.create", params); frame.Error == nil || frame.Error.Code != -32602 {
		t.Fatalf("没有拒绝指向同一目录的路径: %+v", frame)
	}
	params["other_paths"] = []string{alias}
	created := readWorkspace(t, p.call(t, "workspace.create", params))
	actualOther, err := filepath.EvalSymlinks(other)
	if err != nil || !slices.Equal(created.OtherPaths, []string{actualOther}) {
		t.Fatalf("创建结果没有规范化其他目录: %+v %v", created, err)
	}
	p.stop(t)

	p = startApp(t, cfg, io.Discard)
	frame := p.call(t, "workspace.list", map[string]any{})
	requireOK(t, frame)
	var values []workspace.Workspace
	if err := json.Unmarshal(frame.Result, &values); err != nil || len(values) != 2 {
		t.Fatalf("重启后未读回项目: %s %v", frame.Result, err)
	}
	id := map[string]any{"id": created.ID}
	value := readWorkspace(t, p.call(t, "workspace.get", id))
	if value.RootPath != created.RootPath || !slices.Equal(value.OtherPaths, created.OtherPaths) {
		t.Fatalf("重启后目录发生变化: %+v", value)
	}
	requireOK(t, p.call(t, "workspace.delete", id))
	value = readWorkspace(t, p.call(t, "workspace.create", map[string]any{"name": "恢复", "root_path": created.RootPath}))
	if value.ID != created.ID || !slices.Equal(value.OtherPaths, created.OtherPaths) {
		t.Fatalf("省略其他目录恢复时应保留已有目录: %+v", value)
	}
	requireOK(t, p.call(t, "workspace.delete", id))
	value = readWorkspace(t, p.call(t, "workspace.create", map[string]any{"name": "恢复为单目录", "root_path": created.RootPath, "other_paths": []string{}}))
	if value.ID != created.ID || value.OtherPaths == nil || len(value.OtherPaths) != 0 {
		t.Fatalf("明确指定空数组应清空其他目录: %+v", value)
	}
}

func readWorkspace(t *testing.T, frame wireFrame) workspace.Workspace {
	t.Helper()
	requireOK(t, frame)
	var value workspace.Workspace
	if err := json.Unmarshal(frame.Result, &value); err != nil {
		t.Fatal(err)
	}
	return value
}
