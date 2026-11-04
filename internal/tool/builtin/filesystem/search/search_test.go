package search_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/xiaoyu98628/callum/internal/tool"
	"github.com/xiaoyu98628/callum/internal/tool/builtin/filesystem/search"
	"github.com/xiaoyu98628/callum/internal/tool/executor"
	"github.com/xiaoyu98628/callum/internal/tool/registry"
)

func TestSearchArguments(t *testing.T) {
	instance := search.New()
	for _, test := range []struct{ input, want string }{
		{`{"path":".","query":"needle"}`, `{"path":".","limit":50,"query":"needle"}`},
		{`{"path":"a/../notes","query":"needle","limit":0}`, `{"path":"notes","limit":50,"query":"needle"}`},
		{`{"path":".","query":"needle","limit":100}`, `{"path":".","limit":100,"query":"needle"}`},
	} {
		got, err := instance.Validate(json.RawMessage(test.input))
		if err != nil || string(got) != test.want {
			t.Fatalf("搜索参数规范化改变: %s => %s, %v", test.input, got, err)
		}
	}
	for _, raw := range []string{
		`null`, `{"path":"."}`, `{"path":".","query":""}`,
		`{"path":".","query":null}`, `{"path":".","query":"x","limit":null}`,
		`{"path":".","query":"x","limit":-1}`, `{"path":".","query":"x","limit":101}`,
		`{"path":".","query":"x","end_line":1}`, `{"path":"../outside","query":"x"}`,
		`{"path":".","path":"other","query":"x"}`,
		`{"path":".","query":"x","query":"y"}`,
		`{"path":".","query":"x","limit":1,"limit":2}`,
		`{"path":".","query":2}`,
		`{"path":".","query":"x"} {}`,
		`{"path":".","Query":"x"}`,
	} {
		if _, err := instance.Validate(json.RawMessage(raw)); !errors.Is(err, tool.ErrInvalid) {
			t.Fatalf("接受搜索工具的无效参数: %s, %v", raw, err)
		}
		if invocation, err := instance.Prepare(t.Context(), tool.Scope{}, json.RawMessage(raw)); !errors.Is(err, tool.ErrInvalid) {
			if invocation != nil {
				if closeErr := invocation.Close(); closeErr != nil {
					t.Error(closeErr)
				}
			}
			t.Fatalf("准备阶段接受无效参数: %s, %v", raw, err)
		}
	}
}

func TestSearchHandlesEmptyDirectories(t *testing.T) {
	for _, nested := range []bool{false, true} {
		t.Run(map[bool]string{false: "empty_root", true: "empty_child"}[nested], func(t *testing.T) {
			dir := t.TempDir()
			if nested {
				if err := os.Mkdir(filepath.Join(dir, "empty"), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("needle"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			r := registry.New()
			if err := r.Register(search.New()); err != nil {
				t.Fatal(err)
			}
			e, err := executor.New(r, time.Second)
			if err != nil {
				t.Fatal(err)
			}
			prepared, err := e.Prepare(t.Context(), tool.Scope{ID: "workspace", RootPath: dir}, "search_text", json.RawMessage(`{"path":".","query":"needle"}`))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := prepared.Close(); err != nil {
					t.Error(err)
				}
			})
			output, err := e.Execute(t.Context(), prepared, prepared.Hash())
			if err != nil {
				t.Fatalf("搜索空目录失败: %v", err)
			}
			var result struct {
				Matches   []struct{ Path, Text string } `json:"matches"`
				Truncated bool                          `json:"truncated"`
			}
			if err := json.Unmarshal(output.Output, &result); err != nil {
				t.Fatal(err)
			}
			want := 0
			if nested {
				want = 1
			}
			if len(result.Matches) != want || result.Truncated {
				t.Fatalf("空目录影响搜索结果: %s", output.Output)
			}
			resolved, err := filepath.EvalSymlinks(dir)
			if err != nil {
				t.Fatal(err)
			}
			if nested && (result.Matches[0].Path != filepath.Join(resolved, "a.txt") || result.Matches[0].Text != "needle") {
				t.Fatalf("其他文件的匹配结果丢失: %s", output.Output)
			}
		})
	}
}
