```go
package 搜索

import (
	JSON包 "encoding/json"
	UTF8包 "unicode/utf8"

	工具包 "github.com/xiaoyu98628/callum/internal/tool"
	文件访问包 "github.com/xiaoyu98628/callum/internal/tool/builtin/filesystem/internal/fileaccess"
	工具参数包 "github.com/xiaoyu98628/callum/internal/tool/builtin/internal/arguments"
)

type 参数 struct {
	路径 string `json:"path"`
	上限 int    `json:"limit,omitempty"`
	查询 string `json:"query,omitempty"`
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
	if 甲.查询 == "" || len(甲.查询) > 1024 || !UTF8包.有效字符串(甲.查询) || 甲.上限 < 0 || 甲.上限 > 100 {
		return 甲, 工具包.无效错误
	}
	if 甲.上限 == 0 {
		甲.上限 = 50
	}
	return 甲, nil
}
```
