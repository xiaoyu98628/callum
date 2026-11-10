package read_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/xiaoyu98628/callum/internal/tool"
	"github.com/xiaoyu98628/callum/internal/tool/builtin/filesystem/read"
)

func TestReadArguments(t *testing.T) {
	instance := read.New(tool.ProtectedData{})
	for _, test := range []struct{ input, want string }{
		{`{"path":"note"}`, `{"path":"note","start_line":1,"end_line":200}`},
		{`{"path":"a/../note","start_line":5}`, `{"path":"note","start_line":5,"end_line":204}`},
		{`{"path":"note","start_line":0,"end_line":0}`, `{"path":"note","start_line":1,"end_line":200}`},
		{`{"path":"note","start_line":1,"end_line":1000}`, `{"path":"note","start_line":1,"end_line":1000}`},
	} {
		got, err := instance.Validate(json.RawMessage(test.input))
		if err != nil || string(got) != test.want {
			t.Fatalf("读取参数规范化改变: %s => %s, %v", test.input, got, err)
		}
	}
	for _, raw := range []string{
		`null`, `{"path":"note","limit":1}`, `{"path":"note","query":"text"}`,
		`{"path":"note","start_line":null}`, `{"path":"note","end_line":-1}`,
		`{"path":"note","start_line":5,"end_line":4}`, `{"path":"note","end_line":1001}`,
		`{"path":"note","start_line":1000001}`, `{"path":"note","start_line":1.5}`,
		`{"path":"note","path":"other"}`,
		`{"path":"note","start_line":1,"start_line":2}`,
		`{"path":"note","end_line":1,"end_line":2}`,
		`{"path":"note","End_line":2}`,
		`{"path":"note"} {}`,
		`{"path":"note","start_line":"1"}`,
	} {
		if _, err := instance.Validate(json.RawMessage(raw)); !errors.Is(err, tool.ErrInvalid) {
			t.Fatalf("接受读取工具的无效参数: %s, %v", raw, err)
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

func TestReadRangeKeepsWholeFileVersion(t *testing.T) {
	dir := t.TempDir()
	data := []byte("first\n第二行\nlast")
	if err := os.WriteFile(filepath.Join(dir, "note"), data, 0600); err != nil {
		t.Fatal(err)
	}
	instance := read.New(tool.ProtectedData{})
	args, err := instance.Validate(json.RawMessage(`{"path":"note","start_line":2,"end_line":2}`))
	if err != nil {
		t.Fatal(err)
	}
	inv, err := instance.Prepare(t.Context(), tool.Scope{ID: "w", RootPath: dir}, args)
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
		t.Fatalf("读取失败: %s, %v", result.Output, err)
	}
	var output struct {
		Text, SHA256 string
		Start        int `json:"start_line"`
		End          int `json:"end_line"`
		Truncated    bool
	}
	if err := json.Unmarshal(result.Output, &output); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	if output.Text != "第二行" || output.Start != 2 || output.End != 2 || !output.Truncated || output.SHA256 != hex.EncodeToString(sum[:]) {
		t.Fatalf("行范围或完整文件版本改变: %s", result.Output)
	}
}
