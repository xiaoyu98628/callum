package filesystem

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"

	"github.com/xiaoyu98628/callum/internal/tool"
)

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
