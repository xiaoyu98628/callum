package list_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/xiaoyu98628/callum/internal/tool"
	"github.com/xiaoyu98628/callum/internal/tool/builtin/filesystem/list"
)

func TestDirectoryArguments(t *testing.T) {
	instance := list.New(tool.ProtectedData{})
	for _, test := range []struct{ input, want string }{
		{`{"path":"."}`, `{"path":".","limit":100}`},
		{`{"path":"a/../notes","limit":0}`, `{"path":"notes","limit":100}`},
		{`{"path":".","limit":200}`, `{"path":".","limit":200}`},
	} {
		got, err := instance.Validate(json.RawMessage(test.input))
		if err != nil || string(got) != test.want {
			t.Fatalf("参数规范化改变: %s => %s, %v", test.input, got, err)
		}
	}
	for _, raw := range []string{
		`null`, `[]`, `{"path":""}`, `{"path":"../outside"}`,
		`{"path":".","limit":null}`, `{"path":".","limit":-1}`,
		`{"path":".","limit":201}`, `{"path":".","limit":1.5}`,
		`{"path":".","query":"text"}`, `{"path":".","start_line":1}`,
		`{"path":".","Path":"other"}`,
		`{"path":".","path":"other"}`,
		`{"path":".","limit":1,"limit":2}`,
		`{"path":".","Limit":1}`,
		`{"path":"."} {}`,
		`{"path":".","limit":"1"}`,
	} {
		if _, err := instance.Validate(json.RawMessage(raw)); !errors.Is(err, tool.ErrInvalid) {
			t.Fatalf("接受目录工具的无效参数: %s, %v", raw, err)
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

func TestDirectoryLimitAndTargetType(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a", "b"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("text"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	instance := list.New(tool.ProtectedData{})
	scope := tool.Scope{ID: "w", RootPath: dir}
	args, err := instance.Validate(json.RawMessage(`{"path":".","limit":1}`))
	if err != nil {
		t.Fatal(err)
	}
	inv, err := instance.Prepare(t.Context(), scope, args)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := inv.Close(); err != nil {
			t.Error(err)
		}
	})
	result, err := inv.Execute(t.Context())
	if err != nil || result.Outcome != tool.OutcomeNone {
		t.Fatalf("目录读取失败: %s, %v", result.Output, err)
	}
	var output struct {
		Entries   []struct{ Name string } `json:"entries"`
		Truncated bool                    `json:"truncated"`
	}
	if err := json.Unmarshal(result.Output, &output); err != nil {
		t.Fatal(err)
	}
	if len(output.Entries) != 1 || !output.Truncated {
		t.Fatalf("目录上限未保持: %s", result.Output)
	}
	if inv, err := instance.Prepare(t.Context(), scope, json.RawMessage(`{"path":"a","limit":1}`)); !errors.Is(err, tool.ErrUnavailable) {
		if inv != nil {
			t.Cleanup(func() {
				if err := inv.Close(); err != nil {
					t.Error(err)
				}
			})
		}
		t.Fatalf("目录工具接受了文件目标: %v", err)
	}
}
