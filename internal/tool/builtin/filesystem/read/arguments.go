package read

import (
	"encoding/json"

	"github.com/xiaoyu98628/callum/internal/tool"
	"github.com/xiaoyu98628/callum/internal/tool/builtin/filesystem/internal/fileaccess"
	toolargs "github.com/xiaoyu98628/callum/internal/tool/builtin/internal/arguments"
)

type arguments struct {
	Path  string `json:"path"`
	Start int    `json:"start_line,omitempty"`
	End   int    `json:"end_line,omitempty"`
}

func decodeArguments(raw json.RawMessage) (arguments, error) {
	var a arguments
	if _, err := toolargs.Decode(raw, &a); err != nil {
		return a, err
	}
	path, err := fileaccess.CleanPath(a.Path)
	if err != nil {
		return a, err
	}
	a.Path = path
	if a.Start < 0 || a.End < 0 {
		return a, tool.ErrInvalid
	}
	if a.Start == 0 {
		a.Start = 1
	}
	if a.End == 0 {
		a.End = a.Start + 199
	}
	if a.Start > 1_000_000 || a.End < a.Start || a.End-a.Start >= 1000 {
		return a, tool.ErrInvalid
	}
	return a, nil
}
