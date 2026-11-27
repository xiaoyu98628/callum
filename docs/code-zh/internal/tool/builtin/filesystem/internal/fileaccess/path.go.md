```go
package 文件访问

import (
	文件路径包 "path/filepath"
	字符串包 "strings"
	UTF8包 "unicode/utf8"

	工具包 "github.com/xiaoyu98628/callum/internal/tool"
)

// 清理路径 校验文件工具的路径格式，不解析目标或决定访问权限。
func 清理路径(路径 string) (string, error) {
	if 路径 == "" || !UTF8包.有效字符串(路径) || (!文件路径包.是否绝对路径(路径) && !文件路径包.是否本地路径(路径)) || 字符串包.包含字符(路径, 0) {
		return "", 工具包.无效错误
	}
	return 文件路径包.清理(路径), nil
}
```
