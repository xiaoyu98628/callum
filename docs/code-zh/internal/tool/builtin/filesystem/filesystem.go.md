```go
// 包 文件系统 汇总固定目标范围的文件工具，工作区是可选路径基准。
package 文件系统

import (
	工具包 "github.com/xiaoyu98628/callum/internal/tool"
	列表包 "github.com/xiaoyu98628/callum/internal/tool/builtin/filesystem/list"
	补丁包 "github.com/xiaoyu98628/callum/internal/tool/builtin/filesystem/patch"
	读取包 "github.com/xiaoyu98628/callum/internal/tool/builtin/filesystem/read"
	搜索包 "github.com/xiaoyu98628/callum/internal/tool/builtin/filesystem/search"
)

// 工具集 返回独立的工具实例，不读取工作区或注册全局状态。
func 工具集(保护 工具包.受保护数据) []工具包.工具 {
	return []工具包.工具{补丁包.新建(保护), 列表包.新建(保护), 读取包.新建(保护), 搜索包.新建(保护)}
}
```
