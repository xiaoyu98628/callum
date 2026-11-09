package app

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
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

func TestWorkspacePrimaryDirectoryThroughStdio(t *testing.T) {
	cfg, err := config.Load(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatal(err)
	}
	p := startApp(t, cfg, io.Discard)
	root, other := t.TempDir(), t.TempDir()
	created := readWorkspace(t, p.call(t, "workspace.create", map[string]any{"name": "原项目", "root_path": root, "other_paths": []string{other}}))
	root, other = created.RootPath, created.OtherPaths[0]
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, []byte("保留文件"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"relative", filepath.Join(t.TempDir(), "missing"), file, t.TempDir()} {
		frame := p.call(t, "workspace.update", map[string]any{"id": created.ID, "name": "不得保存", "root_path": path})
		if frame.Error == nil || frame.Error.Code != -32602 {
			t.Fatalf("没有拒绝非法或未绑定的主目录 %q: %+v", path, frame)
		}
	}
	conflict := readWorkspace(t, p.call(t, "workspace.create", map[string]any{"name": "占用主目录的项目", "root_path": other}))
	for _, deleted := range []bool{false, true} {
		if deleted {
			requireOK(t, p.call(t, "workspace.delete", map[string]any{"id": conflict.ID}))
		}
		frame := p.call(t, "workspace.update", map[string]any{"id": created.ID, "name": "冲突名称", "root_path": other})
		if frame.Error == nil || frame.Error.Code != 1003 {
			t.Fatalf("没有拒绝与现有项目的主目录冲突: %+v", frame)
		}
	}
	unchanged := readWorkspace(t, p.call(t, "workspace.get", map[string]any{"id": created.ID}))
	if unchanged.Name != created.Name || unchanged.RootPath != root || !slices.Equal(unchanged.OtherPaths, []string{other}) {
		t.Fatalf("被拒绝的更新改变了项目: %+v", unchanged)
	}
	// 独立目录集合可以成功切换，冲突项目仍保留其目录和删除状态。
	primary, secondary := t.TempDir(), t.TempDir()
	project := readWorkspace(t, p.call(t, "workspace.create", map[string]any{"name": "切换项目", "root_path": primary, "other_paths": []string{secondary}}))
	primary, secondary = project.RootPath, project.OtherPaths[0]
	value := readWorkspace(t, p.call(t, "workspace.update", map[string]any{"id": project.ID, "name": "切换后名称", "root_path": secondary}))
	if value.ID != project.ID || value.Name != "切换后名称" || value.RootPath != secondary || !slices.Equal(value.OtherPaths, []string{primary}) {
		t.Fatalf("没有原子保存名称和主目录: %+v", value)
	}
	p.stop(t)
	p = startApp(t, cfg, io.Discard)
	value = readWorkspace(t, p.call(t, "workspace.get", map[string]any{"id": project.ID}))
	if value.RootPath != secondary || value.Name != "切换后名称" || !slices.Equal(value.OtherPaths, []string{primary}) {
		t.Fatalf("重启后主目录切换丢失: %+v", value)
	}
	value = readWorkspace(t, p.call(t, "workspace.update", map[string]any{"id": project.ID, "root_path": primary}))
	if value.RootPath != primary || !slices.Equal(value.OtherPaths, []string{secondary}) {
		t.Fatalf("无法切回原主目录: %+v", value)
	}
}

func TestWorkspacePrimaryDirectoryUsesNewPathBase(t *testing.T) {
	root, other := t.TempDir(), t.TempDir()
	for path, content := range map[string]string{root: "from-original-root", other: "from-new-root"} {
		if err := os.WriteFile(filepath.Join(path, "note.txt"), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	var readNewRoot atomic.Bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Tools    []json.RawMessage `json:"tools"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil || len(request.Messages) == 0 {
			t.Error(err)
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		reply, reason := map[string]any{"content": "完成"}, "stop"
		if len(request.Tools) > 0 {
			last := request.Messages[len(request.Messages)-1]
			if last.Role == "tool" {
				readNewRoot.Store(strings.Contains(last.Content, "from-new-root") && !strings.Contains(last.Content, "from-original-root"))
			} else {
				reply = map[string]any{"tool_calls": []any{map[string]any{"id": "read-note", "type": "function", "function": map[string]any{"name": "read_file", "arguments": `{"path":"note.txt"}`}}}}
				reason = "tool_calls"
			}
		}
		writeFileToolResponse(t, w, false, reply, reason)
	}))
	t.Cleanup(upstream.Close)
	cfg, err := config.Load(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatal(err)
	}
	p := startApp(t, cfg, io.Discard)
	mid := setupConversationModel(t, p, upstream.URL)
	project := readWorkspace(t, p.call(t, "workspace.create", map[string]any{"name": "相对路径项目", "root_path": root, "other_paths": []string{other}}))
	requireOK(t, p.call(t, "workspace.update", map[string]any{"id": project.ID, "root_path": project.OtherPaths[0]}))
	requireOK(t, p.call(t, "conversation.send", map[string]any{"workspace_id": project.ID, "model_id": mid, "stream": false, "content": messageBlocks("读取 note.txt")}))
	p.event(t, "run.completed")
	if !readNewRoot.Load() {
		t.Fatal("相对路径读取没有使用切换后的主要目录")
	}
}
