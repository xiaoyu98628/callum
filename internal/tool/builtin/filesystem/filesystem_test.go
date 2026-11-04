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
	"github.com/xiaoyu98628/callum/internal/tool/policy"
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
	for _, args := range []string{`{"path":"../secret"}`, `{"path":"a.txt","unknown":true}`, `{"path":"a.txt","start_line":-1}`} {
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
	if !p.OutsideWorkspace() || policy.Evaluate(policy.OnRequest, p.Definition().Effects, p.OutsideWorkspace()).Action != policy.Ask {
		t.Fatal("符号链接实际指向工作区外，却未要求审批")
	}
}

func TestFileTargetsWithAndWithoutWorkspace(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	insideFile, outsideFile := filepath.Join(root, "a.txt"), filepath.Join(outside, "b.txt")
	for _, path := range []string{insideFile, outsideFile} {
		if err := os.WriteFile(path, []byte("needle content"), 0600); err != nil {
			t.Fatal(err)
		}
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
	workspace := tool.Scope{ID: "workspace", RootPath: root}
	for _, test := range []struct {
		name, operation, path string
		scope                 tool.Scope
		outside               bool
	}{
		{"relative_inside", "read_file", "a.txt", workspace, false},
		{"absolute_inside", "read_file", insideFile, workspace, false},
		{"absolute_outside", "read_file", outsideFile, workspace, true},
		{"unbound_file", "read_file", outsideFile, tool.Scope{}, true},
		{"unbound_directory", "list_directory", outside, tool.Scope{}, true},
		{"unbound_search_directory", "search_text", outside, tool.Scope{}, true},
		{"unbound_search_file", "search_text", outsideFile, tool.Scope{}, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			args := map[string]any{"path": test.path}
			if test.operation == "search_text" {
				args["query"] = "needle"
			}
			raw, err := json.Marshal(args)
			if err != nil {
				t.Fatal(err)
			}
			p, err := e.Prepare(t.Context(), test.scope, test.operation, raw)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := p.Close(); err != nil {
					t.Error(err)
				}
			})
			if p.OutsideWorkspace() != test.outside {
				t.Fatal("目标范围分类错误")
			}
			var snapshot executor.Operation
			if err := json.Unmarshal(p.Request(), &snapshot); err != nil {
				t.Fatal(err)
			}
			if snapshot.Target == nil || !filepath.IsAbs(snapshot.Target.Path) || snapshot.Target.WithinWorkspace == test.outside || (snapshot.Workspace == nil) != (test.scope.ID == "") {
				t.Fatalf("审批未固定实际目标: %s", p.Request())
			}
			result, err := e.Execute(t.Context(), p, p.Hash())
			if err != nil || !json.Valid(result) {
				t.Fatalf("执行失败: %s %v", result, err)
			}
			if test.operation == "read_file" {
				var value struct {
					Text string `json:"text"`
				}
				if err := json.Unmarshal(result, &value); err != nil || value.Text != "needle content" {
					t.Fatalf("文件内容错误: %s %v", result, err)
				}
			}
			if test.operation == "search_text" {
				var value struct {
					Matches []struct {
						Path string `json:"path"`
					} `json:"matches"`
				}
				resolved, err := filepath.EvalSymlinks(outsideFile)
				if err != nil {
					t.Fatal(err)
				}
				if err := json.Unmarshal(result, &value); err != nil || len(value.Matches) != 1 || value.Matches[0].Path != resolved {
					t.Fatalf("搜索范围或结果错误: %s %v", result, err)
				}
			}
		})
	}
	if p, err := e.Prepare(t.Context(), tool.Scope{}, "read_file", json.RawMessage(`{"path":"a.txt"}`)); !errors.Is(err, tool.ErrPathBase) {
		if p != nil {
			p.Close()
		}
		t.Fatalf("未绑定工作区的相对路径没有明确报错: %v", err)
	}
}

func TestPreparedTargetCannotBeRedirected(t *testing.T) {
	for _, kind := range []string{"file", "directory", "symlink", "workspace"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "target")
			if err := os.WriteFile(path, []byte("original"), 0600); err != nil {
				t.Fatal(err)
			}
			scope := tool.Scope{}
			operation := "read_file"
			if kind == "directory" {
				path = root
				operation = "list_directory"
			}
			if kind == "workspace" {
				scope = tool.Scope{ID: "workspace", RootPath: root}
			}
			if kind == "symlink" {
				link := filepath.Join(root, "link")
				if err := os.Symlink(path, link); err != nil {
					t.Fatal(err)
				}
				path = link
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
			args, err := json.Marshal(map[string]string{"path": path})
			if err != nil {
				t.Fatal(err)
			}
			p, err := e.Prepare(t.Context(), scope, operation, args)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := p.Close(); err != nil {
					t.Error(err)
				}
			}()
			switch kind {
			case "directory", "workspace":
				if err := os.Rename(root, root+"-original"); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { os.RemoveAll(root + "-original") })
				if err := os.Mkdir(root, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(root, "target"), []byte("replacement"), 0600); err != nil {
					t.Fatal(err)
				}
			case "file":
				if err := os.Rename(path, path+"-original"); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte("replacement"), 0600); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				other := filepath.Join(root, "other")
				if err := os.WriteFile(other, []byte("replacement"), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(other, path); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := e.Execute(t.Context(), p, p.Hash()); !errors.Is(err, tool.ErrDenied) {
				t.Fatalf("目标变化后仍继续执行: %v", err)
			}
			if err := p.Close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestDirectorySearchStaysWithinApprovedTarget(t *testing.T) {
	root := t.TempDir()
	directory, sibling := filepath.Join(root, "approved"), filepath.Join(root, "sibling")
	for _, dir := range []string{directory, sibling} {
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("needle"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(sibling, filepath.Join(directory, "outside-link")); err != nil {
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
	args, err := json.Marshal(map[string]string{"path": directory, "query": "needle"})
	if err != nil {
		t.Fatal(err)
	}
	p, err := e.Prepare(t.Context(), tool.Scope{}, "search_text", args)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := p.Close(); err != nil {
			t.Error(err)
		}
	}()
	result, err := e.Execute(t.Context(), p, p.Hash())
	if err != nil {
		t.Fatal(err)
	}
	var output struct {
		Matches []struct {
			Path string `json:"path"`
		} `json:"matches"`
	}
	if err := json.Unmarshal(result, &output); err != nil || len(output.Matches) != 1 {
		t.Fatalf("搜索越出批准目录: %s %v", result, err)
	}
	want, err := filepath.EvalSymlinks(filepath.Join(directory, "a.txt"))
	if err != nil || output.Matches[0].Path != want {
		t.Fatalf("搜索了相邻目录: %s %v", result, err)
	}
}
