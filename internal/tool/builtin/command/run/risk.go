package run

import (
	"path/filepath"
	"runtime"
	"slices"
	"strings"
)

// commandRisk 保留已识别破坏性操作的人工复核；任意脚本仍由系统沙箱限制实际访问。
// 这不是命令白名单，也不声称能判断所有程序的业务副作用。
func commandRisk(command []string) string {
	if len(command) == 0 {
		return "unclassified_operation"
	}
	words := make([]string, 0, len(command))
	for _, part := range command {
		words = append(words, strings.FieldsFunc(strings.ToLower(part), func(r rune) bool {
			return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '/')
		})...)
	}
	for _, word := range words {
		if runtime.GOOS == "windows" {
			switch word {
			case "del", "erase", "rd", "remove-item", "clear-content", "format", "diskpart", "takeown", "icacls", "reg", "shutdown", "stop-process":
				return "destructive_command"
			}
		}
		switch filepath.Base(word) {
		case "rm", "rmdir", "sudo", "su", "diskutil", "dd", "mkfs", "chmod", "chown":
			return "destructive_command"
		}
	}
	for _, word := range words {
		if word == "-delete" || word == "-exec" || word == "-execdir" {
			return "destructive_command"
		}
	}
	if slices.ContainsFunc(words, func(word string) bool { return filepath.Base(word) == "git" }) {
		for _, word := range words {
			if word == "clean" || word == "reset" || word == "restore" || word == "push" {
				return "sensitive_git_command"
			}
		}
	}
	return ""
}
