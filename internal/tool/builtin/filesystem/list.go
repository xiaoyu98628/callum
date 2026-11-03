package filesystem

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
)

func list(ctx context.Context, root *os.Root, a arguments) (_ json.RawMessage, retErr error) {
	f, err := open(root, a.Path)
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
