```go
package 准备模块

import (
	十六进制包 "encoding/hex"
	错误判断包 "errors"
	字符串包 "strings"

	运行时资源包 "github.com/xiaoyu98628/callum/internal/runtimeassets"
)

// 工作区来源 记录用户指定技能运行时的离线归档，不在准备或运行期间联网获取。
type 工作区来源 struct {
	运行时 运行时资源包.工作区运行时 `json:"runtime"`
	归档集 []工作区归档       `json:"archives"`
}

type 工作区归档 struct {
	归档     string `json:"archive"`
	SHA256 string `json:"sha256"`
}

func 校验工作区(来源 *工作区来源) error {
	if 来源 == nil {
		return nil
	}
	当前读取器 := 来源.运行时
	if 当前读取器.版本 == "" || len(来源.归档集) == 0 || len(来源.归档集) > 32 {
		return 错误判断包.新建("工作区依赖资源版本或归档数量无效")
	}
	for _, 名称 := range []string{当前读取器.节点模块集, 当前读取器.二进制目录} {
		if !安全路径(名称) {
			return 错误判断包.新建("工作区依赖入口路径无效")
		}
	}
	if !字符串包.存在前缀(当前读取器.节点模块集, "node/") || 当前读取器.二进制目录 != "native" {
		return 错误判断包.新建("技能依赖必须位于 node 与 native 目录")
	}
	已见 := map[string]bool{}
	for _, 归档 := range 来源.归档集 {
		_, 错误 := 十六进制包.解码字符串(归档.SHA256)
		if !简单名称(归档.归档) || !字符串包.存在后缀(归档.归档, ".tar.gz") || 已见[归档.归档] || 错误 != nil || len(归档.SHA256) != 64 {
			return 错误判断包.新建("工作区依赖归档或校验值无效")
		}
		已见[归档.归档] = true
	}
	return nil
}

func 相同工作区(清单 *运行时资源包.工作区运行时, 来源 *工作区来源) bool {
	if 来源 == nil || 清单 == nil {
		return 来源 == nil && 清单 == nil
	}
	return *清单 == 来源.运行时
}
```
