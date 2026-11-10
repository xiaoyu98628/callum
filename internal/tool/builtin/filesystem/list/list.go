// Package list 提供固定目标范围的目录查看工具。
package list

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"

	"github.com/xiaoyu98628/callum/internal/tool"
	"github.com/xiaoyu98628/callum/internal/tool/builtin/filesystem/internal/fileaccess"
)

type directoryTool struct{ protection tool.ProtectedData }

// New 返回独立的目录查看工具。
func New(protection tool.ProtectedData) tool.Tool { return directoryTool{protection} }

func (directoryTool) Definition() tool.Definition {
	return tool.Definition{Name: "list_directory", Description: "查看目录中的文件和子目录。path 支持绝对路径；相对路径需要绑定工作区。只读访问不受工作区写入边界限制，审批由服务端策略判断", Parameters: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"limit":{"type":"integer","minimum":1,"maximum":200}},"required":["path"],"additionalProperties":false}`), Source: "builtin", Version: "3", Effects: []tool.Effect{tool.EffectRead}}
}

func (directoryTool) Validate(raw json.RawMessage) (json.RawMessage, error) {
	a, err := decodeArguments(raw)
	if err != nil {
		return nil, err
	}
	return json.Marshal(a)
}

func (t directoryTool) Prepare(ctx context.Context, scope tool.Scope, raw json.RawMessage) (tool.Invocation, error) {
	a, err := decodeArguments(raw)
	if err != nil {
		return nil, err
	}
	handle, err := fileaccess.Prepare(ctx, scope, a.Path, fileaccess.Directory, t.protection)
	if err != nil {
		return nil, err
	}
	return &invocation{Handle: handle, args: a}, nil
}

type invocation struct {
	*fileaccess.Handle
	args arguments
}

func (p *invocation) Execute(ctx context.Context) (tool.Result, error) {
	if err := p.Check(ctx); err != nil {
		return tool.Result{Outcome: tool.OutcomeNone}, err
	}
	a := p.args
	a.Path = "."
	output, err := list(ctx, p.Root(), a)
	return tool.Result{Output: output, Outcome: tool.OutcomeNone}, err
}

func list(ctx context.Context, root *os.Root, a arguments) (_ json.RawMessage, retErr error) {
	f, err := fileaccess.OpenRead(root, a.Path, nil)
	if err != nil {
		return nil, err
	}
	defer func() { retErr = errors.Join(retErr, f.Close()) }()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	entries, err := f.ReadDir(a.Limit + 1)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	truncated := len(entries) > a.Limit
	if truncated {
		entries = entries[:a.Limit]
	}
	type entry struct {
		Name      string `json:"name"`
		Directory bool   `json:"directory"`
		Symlink   bool   `json:"symlink"`
	}
	out := make([]entry, 0, len(entries))
	for _, e := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		out = append(out, entry{e.Name(), e.IsDir(), e.Type()&os.ModeSymlink != 0})
	}
	return json.Marshal(struct {
		Entries   []entry `json:"entries"`
		Truncated bool    `json:"truncated"`
	}{out, truncated})
}
