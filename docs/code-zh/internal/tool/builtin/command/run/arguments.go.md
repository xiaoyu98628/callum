```go
package 运行

import (
	JSON包 "encoding/json"
	字符串包 "strings"
	UTF8包 "unicode/utf8"

	工具包 "github.com/xiaoyu98628/callum/internal/tool"
	工具参数包 "github.com/xiaoyu98628/callum/internal/tool/builtin/internal/arguments"
)

const (
	沙箱默认   = "use_default"
	沙箱项目写入 = "workspace_write"
	沙箱提权   = "require_escalated"
)

type 参数 struct {
	命令   []string `json:"command"`
	工作目录 string   `json:"cwd,omitempty"`
	沙箱权限 string   `json:"sandbox_permissions,omitempty"`
	理由   string   `json:"justification,omitempty"`
}

func 解码参数(原始 JSON包.原始JSON消息) (参数, error) {
	var 甲 参数
	已见, 错误 := 工具参数包.解码(原始, &甲)
	if 错误 != nil {
		return 甲, 错误
	}
	if len(甲.命令) == 0 || len(甲.命令) > 256 || 甲.命令[0] == "" {
		return 甲, &工具包.参数错误{字段: "command", 消息: "必须是包含可执行程序的非空字符串数组，长度为 1 至 256"}
	}
	for _, 参数 := range 甲.命令 {
		if !UTF8包.有效字符串(参数) || 字符串包.包含字符(参数, 0) {
			return 甲, 工具包.无效错误
		}
	}
	if 错误 := 校验可执行文件路径(甲.命令[0]); 错误 != nil {
		return 甲, 错误
	}
	if 已见["cwd"] {
		if 甲.工作目录 == "" {
			return 甲, 工具包.无效错误
		}
		路径, 错误 := 清理目录(甲.工作目录)
		if 错误 != nil {
			return 甲, 错误
		}
		甲.工作目录 = 路径
	}
	if 已见["sandbox_permissions"] && 甲.沙箱权限 != 沙箱默认 && 甲.沙箱权限 != 沙箱项目写入 && 甲.沙箱权限 != 沙箱提权 {
		return 甲, 工具包.无效错误
	}
	if 已见["justification"] && (!UTF8包.有效字符串(甲.理由) || 字符串包.包含字符(甲.理由, 0) || 字符串包.裁剪空间(甲.理由) == "" || len(甲.理由) > 2048) {
		return 甲, 工具包.无效错误
	}
	if (甲.沙箱权限 == 沙箱提权 || 甲.沙箱权限 == 沙箱项目写入) && 字符串包.裁剪空间(甲.理由) == "" {
		return 甲, 工具包.无效错误
	}
	if 错误 := 校验Shell(甲.命令); 错误 != nil {
		return 甲, 错误
	}
	return 甲, nil
}
```
