// Package read 提供固定目标范围的按行文本读取工具。
package read

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"

	"github.com/xiaoyu98628/callum/internal/tool"
	"github.com/xiaoyu98628/callum/internal/tool/builtin/filesystem/internal/fileaccess"
)

type readTool struct{}

// New 返回独立的按行文本读取工具。
func New() tool.Tool { return readTool{} }

func (readTool) Definition() tool.Definition {
	return tool.Definition{Name: "read_file", Description: "按行读取 UTF-8 文本文件。path 支持绝对路径；相对路径需要绑定工作区。工作区外访问需要审批", Parameters: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"start_line":{"type":"integer","minimum":1},"end_line":{"type":"integer","minimum":1}},"required":["path"],"additionalProperties":false}`), Source: "builtin", Version: "2", Effects: []tool.Effect{tool.EffectRead}}
}

func (readTool) Validate(raw json.RawMessage) (json.RawMessage, error) {
	a, err := decodeArguments(raw)
	if err != nil {
		return nil, err
	}
	return json.Marshal(a)
}

func (readTool) Prepare(ctx context.Context, scope tool.Scope, raw json.RawMessage) (tool.Invocation, error) {
	a, err := decodeArguments(raw)
	if err != nil {
		return nil, err
	}
	handle, err := fileaccess.Prepare(ctx, scope, a.Path, fileaccess.File)
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
	data, err := fileaccess.ReadText(ctx, p.File())
	if err != nil {
		return tool.Result{Outcome: tool.OutcomeNone}, err
	}
	a := p.args
	a.Path = p.Target().Path
	output, err := read(a, data)
	return tool.Result{Output: output, Outcome: tool.OutcomeNone}, err
}

func read(a arguments, data []byte) (json.RawMessage, error) {
	lines := strings.Split(string(data), "\n")
	start := min(a.Start-1, len(lines))
	end := min(a.End, len(lines))
	text := strings.Join(lines[start:end], "\n")
	if len(text) > tool.MaxResultBytes-4096 {
		return nil, tool.ErrLimit
	}
	sum := sha256.Sum256(data)
	return json.Marshal(struct {
		Path      string `json:"path"`
		Start     int    `json:"start_line"`
		End       int    `json:"end_line"`
		Text      string `json:"text"`
		Truncated bool   `json:"truncated"`
		SHA256    string `json:"sha256"`
	}{a.Path, a.Start, end, text, end < len(lines), hex.EncodeToString(sum[:])})
}
