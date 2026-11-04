package search

import (
	"encoding/json"
	"unicode/utf8"

	"github.com/xiaoyu98628/callum/internal/tool"
	"github.com/xiaoyu98628/callum/internal/tool/builtin/filesystem/internal/fileaccess"
	toolargs "github.com/xiaoyu98628/callum/internal/tool/builtin/internal/arguments"
)

type arguments struct {
	Path  string `json:"path"`
	Limit int    `json:"limit,omitempty"`
	Query string `json:"query,omitempty"`
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
	if a.Query == "" || len(a.Query) > 1024 || !utf8.ValidString(a.Query) || a.Limit < 0 || a.Limit > 100 {
		return a, tool.ErrInvalid
	}
	if a.Limit == 0 {
		a.Limit = 50
	}
	return a, nil
}
