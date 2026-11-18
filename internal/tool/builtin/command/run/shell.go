package run

import (
	"path/filepath"
	"strings"

	"mvdan.cc/sh/v3/syntax"

	"github.com/xiaoyu98628/callum/internal/tool"
)

// validateShell 只识别 sh/bash 的简单内联程序，不解释脚本、不展开变量或执行任何内容。
// 复杂或不支持的语法交给既有沙箱；这里是调用形式约束，不是副作用或权限判定。
func validateShell(command []string) error {
	name := strings.TrimSuffix(strings.ToLower(filepath.Base(command[0])), ".exe")
	variant := syntax.LangPOSIX
	switch name {
	case "sh":
	case "bash":
		variant = syntax.LangBash
	default:
		return nil
	}
	script := ""
	for i := 1; i < len(command); i++ {
		flag := command[i]
		if flag == "--" || !strings.HasPrefix(flag, "-") {
			return nil
		}
		if flag == "--noprofile" || flag == "--norc" {
			continue
		}
		if len(flag) > 1 && !strings.HasPrefix(flag, "--") && strings.HasSuffix(flag, "c") && strings.Trim(flag[1:], "abefhkmnptuvxBCEHPTilrcs") == "" {
			if i+1 >= len(command) {
				return nil
			}
			script = command[i+1]
			break
		}
		// 带参数的选项不猜测其后参数是脚本还是选项值。
		if flag != "-l" && flag != "-e" && flag != "-u" && flag != "-x" {
			return nil
		}
	}
	if script == "" {
		return nil
	}
	file, err := syntax.NewParser(syntax.Variant(variant)).Parse(strings.NewReader(script), "")
	if err != nil || len(file.Stmts) == 0 {
		return nil
	}
	external := false
	for _, stmt := range file.Stmts {
		call, ok := stmt.Cmd.(*syntax.CallExpr)
		if !ok || stmt.Negated || stmt.Background || stmt.Coprocess || stmt.Disown || len(stmt.Redirs) > 0 || len(call.Assigns) > 0 || len(call.Args) == 0 {
			return nil
		}
		words := make([]string, len(call.Args))
		for i, word := range call.Args {
			var ok bool
			words[i], ok = literalShellWord(word.Parts, false)
			if !ok {
				return nil
			}
		}
		// 状态性内建命令、环境操作和 Shell 自省依赖解释器，保留其原有语义。
		switch words[0] {
		case ".", "source", "cd", "export", "readonly", "unset", "set", "shift", "eval", "exec", "trap", "wait", "read", "getopts", "alias", "unalias", "umask", "ulimit", "return", "exit", "break", "continue", "command", "builtin", "enable", "hash", "type", "help", "jobs", "fg", "bg", "disown", "pushd", "popd", "dirs", "local", "declare", "typeset", "let", "shopt", "bind", "history", "complete", "compgen", "compopt", "mapfile", "readarray", ":", "true", "false", "pwd", "times", "kill":
			return nil
		case "echo", "printf":
			if words[0] == "printf" && len(words) > 1 && words[1] == "-v" {
				return nil
			}
		default:
			external = true
		}
	}
	if external {
		return &tool.ArgumentError{Field: "command", Message: "该 sh/bash -c 仅包装单个程序或用分号串联独立命令；本次未执行。请直接提交程序和参数数组，多个独立操作分别调用工具，目录查看使用 list_directory。不要改换 Shell 绕过此校验；真正需要管道、重定向或条件逻辑时才使用 Shell。"}
	}
	return nil
}

func literalShellWord(parts []syntax.WordPart, quoted bool) (string, bool) {
	var value strings.Builder
	for _, part := range parts {
		switch p := part.(type) {
		case *syntax.Lit:
			if strings.ContainsRune(p.Value, '\\') || (!quoted && strings.ContainsAny(p.Value, "*?[]{}~")) {
				return "", false
			}
			value.WriteString(p.Value)
		case *syntax.SglQuoted:
			if p.Dollar {
				return "", false
			}
			value.WriteString(p.Value)
		case *syntax.DblQuoted:
			if p.Dollar {
				return "", false
			}
			text, ok := literalShellWord(p.Parts, true)
			if !ok {
				return "", false
			}
			value.WriteString(text)
		default:
			return "", false
		}
	}
	return value.String(), true
}
