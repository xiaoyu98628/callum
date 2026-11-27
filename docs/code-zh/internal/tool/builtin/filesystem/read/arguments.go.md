```go
package 读取

import (
	JSON包 "encoding/json"

	工具包 "github.com/xiaoyu98628/callum/internal/tool"
	文件访问包 "github.com/xiaoyu98628/callum/internal/tool/builtin/filesystem/internal/fileaccess"
	工具参数包 "github.com/xiaoyu98628/callum/internal/tool/builtin/internal/arguments"
)

type 参数 struct {
	路径 string `json:"path"`
	启动 int    `json:"start_line,omitempty"`
	结束 int    `json:"end_line,omitempty"`
}

func 解码参数(原始 JSON包.原始JSON消息) (参数, error) {
	var 甲 参数
	if _, 错误 := 工具参数包.解码(原始, &甲); 错误 != nil {
		return 甲, 错误
	}
	路径, 错误 := 文件访问包.清理路径(甲.路径)
	if 错误 != nil {
		return 甲, 错误
	}
	甲.路径 = 路径
	if 甲.启动 < 0 || 甲.结束 < 0 {
		return 甲, 工具包.无效错误
	}
	if 甲.启动 == 0 {
		甲.启动 = 1
	}
	if 甲.结束 == 0 {
		甲.结束 = 甲.启动 + 199
	}
	if 甲.启动 > 1_000_000 || 甲.结束 < 甲.启动 || 甲.结束-甲.启动 >= 1000 {
		return 甲, 工具包.无效错误
	}
	return 甲, nil
}
```
