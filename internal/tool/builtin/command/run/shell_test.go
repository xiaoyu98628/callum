package run

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/xiaoyu98628/callum/internal/tool"
)

func TestSimpleShellWrappersRejectedBeforePreparation(t *testing.T) {
	for _, script := range []string{
		"git log --oneline -6; echo '=== status ==='; git status --short; echo '=== 模块 ==='; ls internal",
		"git log --oneline -6; echo '=== status ==='; git status --short",
		"git status --short", "git status\nls internal", `git log --format='a|b;secret-value'`,
	} {
		for _, shell := range []string{"sh", "bash"} {
			raw, _ := json.Marshal(arguments{Command: []string{shell, "-c", script}})
			_, err := New(nil, tool.ProtectedData{}).Validate(raw)
			detail, ok := errors.AsType[*tool.ArgumentError](err)
			if !ok || detail.Field != "command" || !errors.Is(err, tool.ErrInvalid) || strings.Contains(detail.Error(), "secret-value") {
				t.Fatalf("没有安全拒绝简单 Shell: %q %v", script, err)
			}
		}
	}
}

func TestNecessaryShellSyntaxAndOtherShellsPreserved(t *testing.T) {
	for _, command := range [][]string{
		{"sh", "-c", "find internal -name '*.go' | xargs wc -l"},
		{"bash", "-c", `git status > "$1"`, "sh", "output"},
		{"sh", "-c", "git status && git log"},
		{"sh", "-c", "if test -d .git; then git status; fi"},
		{"sh", "-c", "cd project; git status"},
		{"sh", "-c", `PATH=/custom git status`},
		{"sh", "-c", `git show "$revision"`},
		{"sh", "-c", `cat *.go`},
		{"sh", "-c", `printf '%s' hello`},
		{"sh", "script.sh"}, {"zsh", "-c", "git status"},
		{"powershell.exe", "-NoProfile", "-Command", "Get-ChildItem"}, {"cmd.exe", "/d", "/c", "dir"},
		{"git", "status", "--short"},
	} {
		raw, _ := json.Marshal(arguments{Command: command})
		if _, err := New(nil, tool.ProtectedData{}).Validate(raw); err != nil {
			t.Fatalf("误拒绝必要语法或其他 Shell: %v %v", command, err)
		}
	}
}

func TestCommandArrayTypeFeedback(t *testing.T) {
	_, err := New(nil, tool.ProtectedData{}).Validate(json.RawMessage(`{"command":"[\"git\",\"status\"]"}`))
	detail, ok := errors.AsType[*tool.ArgumentError](err)
	if !ok || detail.Field != "command" || !strings.Contains(detail.Message, "字符串数组") {
		t.Fatalf("缺少字段纠正说明: %v", err)
	}
}
