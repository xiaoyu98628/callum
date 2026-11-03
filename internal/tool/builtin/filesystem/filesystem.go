// Package filesystem 提供限定在工作区内的有界只读文件工具。
package filesystem

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/xiaoyu98628/callum/internal/tool"
)

type fileTool struct {
	name, description string
	schema            json.RawMessage
}

// Tools 返回独立的工具实例，不读取工作区或注册全局状态。
func Tools() []tool.Tool {
	return []tool.Tool{
		fileTool{"list_directory", "查看工作区目录中的文件和子目录", json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"limit":{"type":"integer","minimum":1,"maximum":200}},"required":["path"],"additionalProperties":false}`)},
		fileTool{"read_file", "按行读取工作区中的 UTF-8 文本文件", json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"start_line":{"type":"integer","minimum":1},"end_line":{"type":"integer","minimum":1}},"required":["path"],"additionalProperties":false}`)},
		fileTool{"search_text", "在工作区目录的 UTF-8 文本文件中搜索字面文本", json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"query":{"type":"string"},"limit":{"type":"integer","minimum":1,"maximum":100}},"required":["path","query"],"additionalProperties":false}`)},
	}
}

func (t fileTool) Definition() tool.Definition {
	return tool.Definition{Name: t.name, Description: t.description, Parameters: append(json.RawMessage(nil), t.schema...), Source: "builtin", Version: "1", ReadOnly: true}
}

type arguments struct {
	Path  string `json:"path"`
	Limit int    `json:"limit,omitempty"`
	Start int    `json:"start_line,omitempty"`
	End   int    `json:"end_line,omitempty"`
	Query string `json:"query,omitempty"`
}

func (t fileTool) Validate(raw json.RawMessage) (json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || fields == nil {
		return nil, tool.ErrInvalid
	}
	allowed := map[string]bool{"path": true}
	switch t.name {
	case "list_directory":
		allowed["limit"] = true
	case "read_file":
		allowed["start_line"], allowed["end_line"] = true, true
	case "search_text":
		allowed["query"], allowed["limit"] = true, true
	}
	for key, value := range fields {
		if !allowed[key] || string(value) == "null" {
			return nil, tool.ErrInvalid
		}
	}
	var a arguments
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&a) != nil || a.Path == "" || !utf8.ValidString(a.Path) || filepath.IsAbs(a.Path) || !filepath.IsLocal(a.Path) || strings.ContainsRune(a.Path, 0) {
		return nil, tool.ErrInvalid
	}
	a.Path = filepath.Clean(a.Path)
	if a.Limit < 0 || a.Start < 0 || a.End < 0 {
		return nil, tool.ErrInvalid
	}
	switch t.name {
	case "list_directory":
		if a.Limit == 0 {
			a.Limit = 100
		}
		if a.Limit > 200 {
			return nil, tool.ErrInvalid
		}
	case "read_file":
		if a.Start == 0 {
			a.Start = 1
		}
		if a.End == 0 {
			a.End = a.Start + 199
		}
		if a.Start > 1_000_000 || a.End < a.Start || a.End-a.Start >= 1000 {
			return nil, tool.ErrInvalid
		}
	case "search_text":
		if a.Query == "" || len(a.Query) > 1024 || !utf8.ValidString(a.Query) {
			return nil, tool.ErrInvalid
		}
		if a.Limit == 0 {
			a.Limit = 50
		}
		if a.Limit > 100 {
			return nil, tool.ErrInvalid
		}
	}
	return json.Marshal(a)
}

func (t fileTool) Execute(ctx context.Context, root *os.Root, raw json.RawMessage) (json.RawMessage, error) {
	var a arguments
	if err := json.Unmarshal(raw, &a); err != nil {
		return nil, tool.ErrInvalid
	}
	switch t.name {
	case "list_directory":
		return list(ctx, root, a)
	case "read_file":
		return read(ctx, root, a)
	case "search_text":
		return search(ctx, root, a)
	}
	return nil, tool.ErrUnavailable
}

// open 使用非阻塞标志打开路径，避免命名管道阻塞取消；只读普通文件和目录才可使用。
func open(root *os.Root, path string) (*os.File, error) {
	f, err := root.OpenFile(path, os.O_RDONLY|nonblock, 0)
	if err != nil {
		return nil, tool.ErrDenied
	}
	return f, nil
}

func textFile(ctx context.Context, root *os.Root, path string) (data []byte, retErr error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f, err := open(root, path)
	if err != nil {
		return nil, err
	}
	defer func() { retErr = errors.Join(retErr, f.Close()) }()
	stat, err := f.Stat()
	if err != nil {
		return nil, tool.ErrUnavailable
	}
	if !stat.Mode().IsRegular() {
		return nil, tool.ErrUnavailable
	}
	if stat.Size() > 1<<20 {
		return nil, tool.ErrLimit
	}
	data, err = io.ReadAll(io.LimitReader(f, (1<<20)+1))
	if err != nil {
		return nil, tool.ErrUnavailable
	}
	if len(data) > 1<<20 {
		return nil, tool.ErrLimit
	}
	if !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 {
		return nil, tool.ErrUnavailable
	}
	return data, ctx.Err()
}
