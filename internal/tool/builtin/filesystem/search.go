package filesystem

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/xiaoyu98628/callum/internal/tool"
)

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
			f, err := open(root, path)
			if err != nil {
				return err
			}
			entries, readErr := f.ReadDir(1001)
			closeErr := f.Close()
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
			data, err = fileText(ctx, file)
		} else {
			data, err = textFile(ctx, root, path)
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
