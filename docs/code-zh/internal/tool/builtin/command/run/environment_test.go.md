```go
package 运行

import (
	文件路径包 "path/filepath"
	运行时包 "runtime"
	字符串包 "strings"
	测试包 "testing"
)

func Test沙箱缓存集使用私有目录(测试器 *测试包.测试器) {
	目录 := 测试器.创建临时目录()
	来源 := []string{"HOME=user-home", "TMPDIR=old", "GOCACHE=old", "npm_config_cache=old", "PIP_CACHE_DIR=old"}
	if 运行时包.目标操作系统 == "windows" {
		来源 = append(来源, "gocache=lowercase-old", "tmpdir=lowercase-old")
	}
	环境 := 沙箱环境(来源, 目录)
	for _, 键 := range []string{"TMPDIR", "TMP", "TEMP", "GOCACHE", "GOTMPDIR", "npm_config_cache", "PIP_CACHE_DIR"} {
		预期 := 目录
		if 键 == "GOCACHE" {
			预期 = 文件路径包.合并(目录, "go-build")
		}
		if 键 == "npm_config_cache" {
			预期 = 文件路径包.合并(目录, "npm-cache")
		}
		数量 := 0
		for _, 条目 := range 环境 {
			名称, 值, _ := 字符串包.截断(条目, "=")
			匹配集 := 名称 == 键 || (运行时包.目标操作系统 == "windows" && 字符串包.相等折叠(名称, 键))
			if 匹配集 {
				数量++
				if 值 != 预期 {
					测试器.格式化报告致命错误("缓存仍指向非私有目录: %s", 条目)
				}
			}
		}
		if 数量 != 1 {
			测试器.格式化报告致命错误("缓存配置重复或缺失: %s %d", 键, 数量)
		}
	}
	if 环境值(环境, "HOME") != "user-home" {
		测试器.报告致命错误("非缓存环境被改写")
	}
}
```
