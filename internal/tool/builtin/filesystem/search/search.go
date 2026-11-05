// Package search 提供固定目标范围的字面文本搜索工具。
package search

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/xiaoyu98628/callum/internal/tool"
	"github.com/xiaoyu98628/callum/internal/tool/builtin/filesystem/internal/fileaccess"
)

type searchTool struct{}

// New 返回独立的字面文本搜索工具。
func New() tool.Tool { return searchTool{} }

func (searchTool) Definition() tool.Definition {
	return tool.Definition{Name: "search_text", Description: "在目录或文件中搜索字面文本。path 支持绝对路径；相对路径需要绑定工作区。只读访问不受工作区写入边界限制，审批由服务端策略判断", Parameters: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"query":{"type":"string"},"limit":{"type":"integer","minimum":1,"maximum":100}},"required":["path","query"],"additionalProperties":false}`), Source: "builtin", Version: "3", Effects: []tool.Effect{tool.EffectRead}}
}

func (searchTool) Validate(raw json.RawMessage) (json.RawMessage, error) {
	a, err := decodeArguments(raw)
	if err != nil {
		return nil, err
	}
	return json.Marshal(a)
}

func (searchTool) Prepare(ctx context.Context, scope tool.Scope, raw json.RawMessage) (tool.Invocation, error) {
	a, err := decodeArguments(raw)
	if err != nil {
		return nil, err
	}
	handle, err := fileaccess.Prepare(ctx, scope, a.Path, fileaccess.FileOrDirectory)
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
	output, err := search(ctx, p.Root(), p.File(), a, p.Target().Path)
	return tool.Result{Output: output, Outcome: tool.OutcomeNone}, err
}

type match struct {
	Path string `json:"path"`
	Line int    `json:"line"`
	Text string `json:"text"`
}

func search(ctx context.Context, root *os.Root, file *os.File, a arguments, base string) (json.RawMessage, error) {
	out := make([]match, 0)
	visited, bytesRead := 0, 0
	truncated := false
	var walk func(string, int) error
	walk = func(path string, depth int) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if depth > 32 || visited >= 1000 || bytesRead >= 8<<20 || len(out) >= a.Limit {
			truncated = true
			return nil
		}
		visited++
		var stat os.FileInfo
		var err error
		if file != nil {
			stat, err = file.Stat()
		} else {
			stat, err = root.Lstat(path)
		}
		if err != nil {
			return tool.ErrDenied
		}
		// 目录搜索不跟随子项符号链接，不扩大本次固定目录的访问范围。
		if stat.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		if stat.IsDir() {
			f, err := fileaccess.OpenRead(root, path)
			if err != nil {
				return err
			}
			entries, readErr := f.ReadDir(1001)
			closeErr := f.Close()
			// 空目录返回 EOF，属于正常遍历结果；关闭错误仍需保留。
			if errors.Is(readErr, io.EOF) {
				readErr = nil
			}
			if err := errors.Join(readErr, closeErr); err != nil {
				return err
			}
			for _, entry := range entries {
				if visited >= 1000 || len(out) >= a.Limit || bytesRead >= 8<<20 {
					truncated = true
					break
				}
				if entry.Name() == ".git" {
					continue
				}
				if err := walk(filepath.Join(path, entry.Name()), depth+1); err != nil {
					return err
				}
			}
			if len(entries) > 1000 {
				truncated = true
			}
			return nil
		}
		if !stat.Mode().IsRegular() {
			return nil
		}
		var data []byte
		if file != nil {
			data, err = fileaccess.ReadText(ctx, file)
		} else {
			data, err = fileaccess.ReadTextFile(ctx, root, path)
		}
		if errors.Is(err, tool.ErrLimit) || errors.Is(err, tool.ErrUnavailable) {
			truncated = true
			return nil
		}
		if err != nil {
			return err
		}
		bytesRead += len(data)
		for i, line := range strings.Split(string(data), "\n") {
			if strings.Contains(line, a.Query) {
				if len(line) > 1024 {
					line = line[:1024]
					for len(line) > 0 && !utf8.ValidString(line) {
						line = line[:len(line)-1]
					}
					truncated = true
				}
				out = append(out, match{filepath.Join(base, path), i + 1, line})
				if len(out) >= a.Limit {
					truncated = true
					break
				}
			}
		}
		return nil
	}
	if err := walk(a.Path, 0); err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Matches   []match `json:"matches"`
		Truncated bool    `json:"truncated"`
	}{out, truncated})
}
