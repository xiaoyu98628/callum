package run

import (
	"encoding/json"
	"strings"
	"unicode/utf8"

	"github.com/xiaoyu98628/callum/internal/tool"
	toolargs "github.com/xiaoyu98628/callum/internal/tool/builtin/internal/arguments"
)

const (
	sandboxDefault      = "use_default"
	sandboxProjectWrite = "workspace_write"
	sandboxEscalated    = "require_escalated"
)

type arguments struct {
	Command            []string `json:"command"`
	Cwd                string   `json:"cwd,omitempty"`
	SandboxPermissions string   `json:"sandbox_permissions,omitempty"`
	Justification      string   `json:"justification,omitempty"`
}

func decodeArguments(raw json.RawMessage) (arguments, error) {
	var a arguments
	seen, err := toolargs.Decode(raw, &a)
	if err != nil {
		return a, err
	}
	if len(a.Command) == 0 || len(a.Command) > 256 || a.Command[0] == "" {
		return a, tool.ErrInvalid
	}
	for _, arg := range a.Command {
		if !utf8.ValidString(arg) || strings.ContainsRune(arg, 0) {
			return a, tool.ErrInvalid
		}
	}
	if err := validateExecutablePath(a.Command[0]); err != nil {
		return a, err
	}
	if seen["cwd"] {
		if a.Cwd == "" {
			return a, tool.ErrInvalid
		}
		path, err := cleanDirectory(a.Cwd)
		if err != nil {
			return a, err
		}
		a.Cwd = path
	}
	if seen["sandbox_permissions"] && a.SandboxPermissions != sandboxDefault && a.SandboxPermissions != sandboxProjectWrite && a.SandboxPermissions != sandboxEscalated {
		return a, tool.ErrInvalid
	}
	if seen["justification"] && (!utf8.ValidString(a.Justification) || strings.ContainsRune(a.Justification, 0) || strings.TrimSpace(a.Justification) == "" || len(a.Justification) > 2048) {
		return a, tool.ErrInvalid
	}
	if (a.SandboxPermissions == sandboxEscalated || a.SandboxPermissions == sandboxProjectWrite) && strings.TrimSpace(a.Justification) == "" {
		return a, tool.ErrInvalid
	}
	return a, nil
}
