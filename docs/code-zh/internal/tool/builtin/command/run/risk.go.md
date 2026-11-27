```go
package 运行

import (
	文件路径包 "path/filepath"
	运行时包 "runtime"
	切片包 "slices"
	字符串包 "strings"
)

// 命令风险 保留已识别破坏性操作的人工复核；任意脚本仍由系统沙箱限制实际访问。
// 这不是命令白名单，也不声称能判断所有程序的业务副作用。
func 命令风险(命令 []string) string {
	if len(命令) == 0 {
		return "unclassified_operation"
	}
	单词集 := make([]string, 0, len(命令))
	for _, 部分 := range 命令 {
		单词集 = append(单词集, 字符串包.字段集函数(字符串包.目标小写(部分), func(当前读取器 rune) bool {
			return !(当前读取器 >= 'a' && 当前读取器 <= 'z' || 当前读取器 >= '0' && 当前读取器 <= '9' || 当前读取器 == '-' || 当前读取器 == '_' || 当前读取器 == '/')
		})...)
	}
	for _, 单词 := range 单词集 {
		if 运行时包.目标操作系统 == "windows" {
			switch 单词 {
			case "del", "erase", "rd", "remove-item", "clear-content", "format", "diskpart", "takeown", "icacls", "reg", "shutdown", "stop-process":
				return "destructive_command"
			}
		}
		switch 文件路径包.基准(单词) {
		case "rm", "rmdir", "sudo", "su", "diskutil", "dd", "mkfs", "chmod", "chown":
			return "destructive_command"
		}
	}
	for _, 单词 := range 单词集 {
		if 单词 == "-delete" || 单词 == "-exec" || 单词 == "-execdir" {
			return "destructive_command"
		}
	}
	if 切片包.包含函数(单词集, func(单词 string) bool { return 文件路径包.基准(单词) == "git" }) {
		for _, 单词 := range 单词集 {
			if 单词 == "clean" || 单词 == "reset" || 单词 == "restore" || 单词 == "push" {
				return "sensitive_git_command"
			}
		}
	}
	return ""
}
```
