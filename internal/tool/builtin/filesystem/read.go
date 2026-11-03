package filesystem

import (
	"context"
	"encoding/json"
	"os"
	"strings"

	"github.com/xiaoyu98628/callum/internal/tool"
)

func read(ctx context.Context, root *os.Root, a arguments) (json.RawMessage, error) {
	data, err := textFile(ctx, root, a.Path)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(data), "\n")
	start := min(a.Start-1, len(lines))
	end := min(a.End, len(lines))
	text := strings.Join(lines[start:end], "\n")
	if len(text) > tool.MaxResultBytes-4096 {
		return nil, tool.ErrLimit
	}
	return json.Marshal(struct {
		Path      string `json:"path"`
		Start     int    `json:"start_line"`
		End       int    `json:"end_line"`
		Text      string `json:"text"`
		Truncated bool   `json:"truncated"`
	}{a.Path, a.Start, end, text, end < len(lines)})
}
