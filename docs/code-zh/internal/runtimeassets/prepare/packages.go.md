```go
package 准备模块

import (
	上下文包 "context"
	十六进制包 "encoding/hex"
	错误判断包 "errors"
	URL包 "net/url"
	操作系统包 "os"
	文件路径包 "path/filepath"
	字符串包 "strings"
	Unicode包 "unicode"

	运行时资源包 "github.com/xiaoyu98628/callum/internal/runtimeassets"
)

func 包目录(目标 目标) string {
	if 目标.平台 == "windows" {
		return "python/Lib/site-packages"
	}
	部分集 := 字符串包.拆分(目标.Python.版本, ".")
	if len(部分集) < 2 {
		return ""
	}
	for _, 部分 := range 部分集[:2] {
		if 部分 == "" || 字符串包.包含函数(部分, func(当前读取器 rune) bool { return !Unicode包.是数字(当前读取器) || 当前读取器 > 127 }) {
			return ""
		}
	}
	return "python/lib/python" + 字符串包.合并(部分集[:2], ".") + "/site-packages"
}

func 校验包集(目标 目标) error {
	if len(目标.Python包集) == 0 {
		return nil
	}
	if len(目标.Python包集) > 32 || 包目录(目标) == "" {
		return 错误判断包.新建("Python 依赖数量或版本无效")
	}
	已见 := map[string]bool{}
	for _, 依赖 := range 目标.Python包集 {
		已解析, 错误 := URL包.解析(依赖.URL)
		_, 哈希错误 := 十六进制包.解码字符串(依赖.SHA256)
		if !简单名称(依赖.名称) || 已见[依赖.名称] || 错误 != nil || 已解析.方案 != "https" || 已解析.主机 != "files.pythonhosted.org" || 已解析.用户 != nil || 已解析.原始查询 != "" || 已解析.片段 != "" || 哈希错误 != nil || len(依赖.SHA256) != 64 || 依赖.版本 == "" || !简单名称(依赖.归档) || !字符串包.存在后缀(依赖.归档, ".whl") || 依赖.前缀 != "." || !安全路径(依赖.条目) {
			return 错误判断包.新建("Python wheel 来源、校验值或模块入口无效")
		}
		已见[依赖.名称] = true
	}
	return nil
}

func 包清单(目标 目标) []运行时资源包.Python包 {
	var 包集 []运行时资源包.Python包
	for _, 依赖 := range 目标.Python包集 {
		包集 = append(包集, 运行时资源包.Python包{名称: 依赖.名称, 版本: 依赖.版本, 路径: 包目录(目标) + "/" + 依赖.条目})
	}
	return 包集
}

func 安装包集(执行上下文 上下文包.上下文, 目标 目标, 阶段 string, 归档集 []string) error {
	if len(归档集) == 0 {
		return nil
	}
	if 错误 := 校验包集(目标); 错误 != nil {
		return 错误
	}
	if len(归档集) != len(目标.Python包集) {
		return 错误判断包.新建("Python 依赖归档数量不一致")
	}
	目录 := 文件路径包.合并(阶段, 文件路径包.来源斜杠(包目录(目标)))
	if 错误 := 操作系统包.创建目录全部(目录, 0755); 错误 != nil {
		return 错误
	}
	for _, 归档 := range 归档集 {
		if 错误 := 提取进入(执行上下文, 归档, 目录, "."); 错误 != nil {
			return 错误
		}
	}
	return nil
}
```
