```go
package 运行

import (
	文件路径包 "path/filepath"
	字符串包 "strings"

	语法包 "mvdan.cc/sh/v3/syntax"

	工具包 "github.com/xiaoyu98628/callum/internal/tool"
)

// 校验Shell 只识别 sh/Bash 的简单内联程序，不解释脚本、不展开变量或执行任何内容。
// 复杂或不支持的语法交给既有沙箱；这里是调用形式约束，不是副作用或权限判定。
func 校验Shell(命令 []string) error {
	名称 := 字符串包.裁剪后缀(字符串包.目标小写(文件路径包.基准(命令[0])), ".exe")
	变体 := 语法包.语言POSIX
	switch 名称 {
	case "sh":
	case "bash":
		变体 = 语法包.语言Bash
	default:
		return nil
	}
	脚本 := ""
	for 索引 := 1; 索引 < len(命令); 索引++ {
		命令行参数 := 命令[索引]
		if 命令行参数 == "--" || !字符串包.存在前缀(命令行参数, "-") {
			return nil
		}
		if 命令行参数 == "--noprofile" || 命令行参数 == "--norc" {
			continue
		}
		if len(命令行参数) > 1 && !字符串包.存在前缀(命令行参数, "--") && 字符串包.存在后缀(命令行参数, "c") && 字符串包.裁剪(命令行参数[1:], "abefhkmnptuvxBCEHPTilrcs") == "" {
			if 索引+1 >= len(命令) {
				return nil
			}
			脚本 = 命令[索引+1]
			break
		}
		// 带参数的选项不猜测其后参数是脚本还是选项值。
		if 命令行参数 != "-l" && 命令行参数 != "-e" && 命令行参数 != "-u" && 命令行参数 != "-x" {
			return nil
		}
	}
	if 脚本 == "" {
		return nil
	}
	文件, 错误 := 语法包.新建解析器(语法包.变体(变体)).解析(字符串包.新建读取器(脚本), "")
	if 错误 != nil || len(文件.语句集) == 0 {
		return nil
	}
	外部 := false
	for _, 语句 := range 文件.语句集 {
		调用, 成功 := 语句.命令.(*语法包.调用表达式)
		if !成功 || 语句.取反 || 语句.后台上下文 || 语句.协进程 || 语句.解除进程关联 || len(语句.重定向集) > 0 || len(调用.分配) > 0 || len(调用.启动参数) == 0 {
			return nil
		}
		单词集 := make([]string, len(调用.启动参数))
		for 索引, 单词 := range 调用.启动参数 {
			var 成功 bool
			单词集[索引], 成功 = 字面量Shell单词(单词.部分集, false)
			if !成功 {
				return nil
			}
		}
		// 状态性内建命令、环境操作和 Shell 自省依赖解释器，保留其原有语义。
		switch 单词集[0] {
		case ".", "source", "cd", "export", "readonly", "unset", "set", "shift", "eval", "exec", "trap", "wait", "read", "getopts", "alias", "unalias", "umask", "ulimit", "return", "exit", "break", "continue", "command", "builtin", "enable", "hash", "type", "help", "jobs", "fg", "bg", "disown", "pushd", "popd", "dirs", "local", "declare", "typeset", "let", "shopt", "bind", "history", "complete", "compgen", "compopt", "mapfile", "readarray", ":", "true", "false", "pwd", "times", "kill":
			return nil
		case "echo", "printf":
			if 单词集[0] == "printf" && len(单词集) > 1 && 单词集[1] == "-v" {
				return nil
			}
		default:
			外部 = true
		}
	}
	if 外部 {
		return &工具包.参数错误{字段: "command", 消息: "该 sh/bash -c 仅包装单个程序或用分号串联独立命令；本次未执行。请直接提交程序和参数数组，多个独立操作分别调用工具，目录查看使用 list_directory。不要改换 Shell 绕过此校验；真正需要管道、重定向或条件逻辑时才使用 Shell。"}
	}
	return nil
}

func 字面量Shell单词(部分集 []语法包.单词部分, 已引用 bool) (string, bool) {
	var 值 字符串包.构建器
	for _, 部分 := range 部分集 {
		switch 当前对象 := 部分.(type) {
		case *语法包.字面量:
			if 字符串包.包含字符(当前对象.值, '\\') || (!已引用 && 字符串包.包含任一字符(当前对象.值, "*?[]{}~")) {
				return "", false
			}
			值.写入字符串(当前对象.值)
		case *语法包.单引号已引用:
			if 当前对象.美元符 {
				return "", false
			}
			值.写入字符串(当前对象.值)
		case *语法包.双引号已引用:
			if 当前对象.美元符 {
				return "", false
			}
			文本, 成功 := 字面量Shell单词(当前对象.部分集, true)
			if !成功 {
				return "", false
			}
			值.写入字符串(文本)
		default:
			return "", false
		}
	}
	return 值.字符串(), true
}
```
