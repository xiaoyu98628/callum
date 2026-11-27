```go
package 运行

import (
	JSON包 "encoding/json"
	错误判断包 "errors"
	字符串包 "strings"
	测试包 "testing"

	工具包 "github.com/xiaoyu98628/callum/internal/tool"
)

func Test简单Shell包装器集已拒绝之前准备(测试器 *测试包.测试器) {
	for _, 脚本 := range []string{
		"git log --oneline -6; echo '=== status ==='; git status --short; echo '=== 模块 ==='; ls internal",
		"git log --oneline -6; echo '=== status ==='; git status --short",
		"git status --short", "git status\nls internal", `git log --format='a|b;secret-value'`,
	} {
		for _, Shell := range []string{"sh", "bash"} {
			原始, _ := JSON包.序列化(参数{命令: []string{Shell, "-c", 脚本}})
			_, 错误 := 新建(nil, 工具包.受保护数据{}).校验(原始)
			详情, 成功 := 错误判断包.按类型匹配[*工具包.参数错误](错误)
			if !成功 || 详情.字段 != "command" || !错误判断包.是否属于(错误, 工具包.无效错误) || 字符串包.包含(详情.错误文本(), "secret-value") {
				测试器.格式化报告致命错误("没有安全拒绝简单 Shell: %q %v", 脚本, 错误)
			}
		}
	}
}

func Test必要Shell语法并且其他Shell集已保留(测试器 *测试包.测试器) {
	for _, 命令 := range [][]string{
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
		原始, _ := JSON包.序列化(参数{命令: 命令})
		if _, 错误 := 新建(nil, 工具包.受保护数据{}).校验(原始); 错误 != nil {
			测试器.格式化报告致命错误("误拒绝必要语法或其他 Shell: %v %v", 命令, 错误)
		}
	}
}

func Test命令数组类型反馈(测试器 *测试包.测试器) {
	_, 错误 := 新建(nil, 工具包.受保护数据{}).校验(JSON包.原始JSON消息(`{"command":"[\"git\",\"status\"]"}`))
	详情, 成功 := 错误判断包.按类型匹配[*工具包.参数错误](错误)
	if !成功 || 详情.字段 != "command" || !字符串包.包含(详情.消息, "字符串数组") {
		测试器.格式化报告致命错误("缺少字段纠正说明: %v", 错误)
	}
}
```
