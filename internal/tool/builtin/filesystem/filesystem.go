// Package filesystem 汇总固定目标范围的文件工具，工作区是可选路径基准。
package filesystem

import (
	"github.com/xiaoyu98628/callum/internal/tool"
	"github.com/xiaoyu98628/callum/internal/tool/builtin/filesystem/list"
	"github.com/xiaoyu98628/callum/internal/tool/builtin/filesystem/patch"
	"github.com/xiaoyu98628/callum/internal/tool/builtin/filesystem/read"
	"github.com/xiaoyu98628/callum/internal/tool/builtin/filesystem/search"
)

// Tools 返回独立的工具实例，不读取工作区或注册全局状态。
func Tools(protection tool.ProtectedData) []tool.Tool {
	return []tool.Tool{patch.New(protection), list.New(protection), read.New(protection), search.New(protection)}
}
