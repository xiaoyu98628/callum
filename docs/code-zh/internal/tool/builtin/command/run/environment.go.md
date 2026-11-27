```go
package 运行

import (
	文件路径包 "path/filepath"
	运行时包 "runtime"
	切片包 "slices"
	字符串包 "strings"
	UTF8包 "unicode/utf8"
)

// 白名单只承载基础路径和语言环境，不继承 API 键、令牌、代理凭据或任意工具配置。
func 基准环境(来源 []string) []string {
	值集 := map[string]string{}
	for _, 条目 := range 来源 {
		键, 值, 成功 := 字符串包.截断(条目, "=")
		if !成功 || !UTF8包.有效字符串(值) || 字符串包.包含字符(值, 0) {
			continue
		}
		if 运行时包.目标操作系统 == "windows" {
			键 = 字符串包.目标大写(键)
		}
		switch 键 {
		case "PATH", "HOME", "USERPROFILE", "TMPDIR", "TEMP", "TMP", "SYSTEMROOT", "WINDIR", "COMSPEC", "PATHEXT", "LANG", "LC_ALL", "LC_CTYPE":
			值集[键] = 值
		}
	}
	// 可执行文件查找和子进程环境均不使用隐含当前目录或相对 路径 项。
	var 路径集 []string
	for _, 路径 := range 文件路径包.拆分列表(值集["PATH"]) {
		if 文件路径包.是否绝对路径(路径) {
			路径集 = append(路径集, 文件路径包.清理(路径))
		}
	}
	值集["PATH"] = 字符串包.合并(路径集, string(文件路径包.列表分隔符))
	键集 := make([]string, 0, len(值集))
	for 键 := range 值集 {
		键集 = append(键集, 键)
	}
	切片包.排序(键集)
	输出 := make([]string, 0, len(键集))
	for _, 键 := range 键集 {
		输出 = append(输出, 键+"="+值集[键])
	}
	return 输出
}

func 环境值(环境 []string, 键 string) string {
	for _, 条目 := range 环境 {
		名称, 值, _ := 字符串包.截断(条目, "=")
		if 名称 == 键 {
			return 值
		}
	}
	return ""
}

// 沙箱环境 将常用缓存统一定向到本次私有目录，保留平台环境变量名比较规则。
func 沙箱环境(环境 []string, 目录 string) []string {
	for _, 键 := range []string{"TMPDIR", "TMP", "TEMP", "GOCACHE", "GOTMPDIR", "npm_config_cache", "PIP_CACHE_DIR"} {
		环境 = 切片包.删除函数(环境, func(条目 string) bool {
			名称, _, _ := 字符串包.截断(条目, "=")
			if 运行时包.目标操作系统 == "windows" {
				return 字符串包.相等折叠(名称, 键)
			}
			return 名称 == 键
		})
		值 := 目录
		switch 键 {
		case "GOCACHE":
			值 = 文件路径包.合并(目录, "go-build")
		case "npm_config_cache":
			值 = 文件路径包.合并(目录, "npm-cache")
		}
		环境 = append(环境, 键+"="+值)
	}
	return 环境
}
```
