package list

import (
	"encoding/json"

	"github.com/xiaoyu98628/callum/internal/tool"
	"github.com/xiaoyu98628/callum/internal/tool/builtin/filesystem/internal/fileaccess"
	toolargs "github.com/xiaoyu98628/callum/internal/tool/builtin/internal/arguments"
)

type arguments struct {
	Path  string `json:"path"`
	Limit int    `json:"limit,omitempty"`
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
	if a.Limit < 0 || a.Limit > 200 {
		return a, tool.ErrInvalid
	}
	if a.Limit == 0 {
		a.Limit = 100
	}
	return a, nil
}
