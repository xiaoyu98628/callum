```go
package 路径集

import (
	错误判断包 "errors"
	格式化包 "fmt"
	操作系统包 "os"
	文件路径包 "path/filepath"
	字符串转换包 "strconv"
	字符串包 "strings"
)

// 解析 将显式或默认数据目录转换为绝对路径，不创建目录或文件。
func 解析(数据目录 string) (string, error) {
	if 数据目录 == "" {
		var 错误 error
		数据目录, 错误 = 默认数据目录()
		if 错误 != nil {
			return "", 错误
		}
	}
	绝对值, 错误 := 文件路径包.绝对值(数据目录)
	if 错误 != nil {
		return "", 格式化包.格式化创建错误("计算数据目录: %w", 错误)
	}
	return 绝对值, nil
}

func 默认数据目录() (string, error) {
	if 释放构建 {
		家目录, 错误 := 操作系统包.用户家目录目录()
		if 错误 != nil {
			return "", 格式化包.格式化创建错误("读取用户家目录: %w", 错误)
		}
		if !文件路径包.是否绝对路径(家目录) {
			return "", 错误判断包.新建("用户家目录必须是绝对路径")
		}
		return 文件路径包.合并(家目录, ".callum"), nil
	}
	根, 错误 := 查找项目根()
	if 错误 != nil {
		return "", 错误
	}
	return 文件路径包.合并(根, ".callum", "dev"), nil
}

func 查找项目根() (string, error) {
	目录, 错误 := 操作系统包.获取工作目录()
	if 错误 != nil {
		return "", 格式化包.格式化创建错误("读取开发工作目录: %w", 错误)
	}
	for {
		内容, 错误 := 操作系统包.读取文件(文件路径包.合并(目录, "go.mod"))
		if 错误 != nil && !错误判断包.是否属于(错误, 操作系统包.不存在错误) {
			return "", 格式化包.格式化创建错误("读取开发项目标识: %w", 错误)
		}
		if 错误 == nil && 是Callum模块(内容) {
			return 目录, nil
		}
		父级 := 文件路径包.目录(目录)
		if 父级 == 目录 {
			return "", 错误判断包.新建("未找到 Callum 项目根目录，请从项目目录启动或显式指定数据目录")
		}
		目录 = 父级
	}
}

func 是Callum模块(内容 []byte) bool {
	for 行 := range 字符串包.行集(string(内容)) {
		行, _, _ = 字符串包.截断(行, "//")
		字段集 := 字符串包.字段集(行)
		if len(字段集) != 2 || 字段集[0] != "module" {
			continue
		}
		模块 := 字段集[1]
		if 字符串包.存在前缀(模块, "\"") || 字符串包.存在前缀(模块, "`") {
			var 错误 error
			模块, 错误 = 字符串转换包.去除引号(模块)
			if 错误 != nil {
				return false
			}
		}
		// 校验模块标识，避免在其他项目中启动时使用错误的数据目录。
		return 模块 == "github.com/xiaoyu98628/callum"
	}
	return false
}
```
