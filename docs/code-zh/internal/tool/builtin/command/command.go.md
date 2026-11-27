```go
// 包 命令 汇总命令工具，由具体工具负责参数、审批详情与进程生命周期。
package 命令

import (
	上下文包 "context"

	运行时资源包 "github.com/xiaoyu98628/callum/internal/runtimeassets"
	工具包 "github.com/xiaoyu98628/callum/internal/tool"
	运行包 "github.com/xiaoyu98628/callum/internal/tool/builtin/command/run"
)

// 工具集带有运行时 将只读解释器资源交给现有命令工具，资源生命周期由应用持有。
func 工具集带有运行时(环境 []string, 保护 工具包.受保护数据, 资源包 *运行时资源包.资源包) []工具包.工具 {
	return []工具包.工具{运行包.新建带有运行时(环境, 保护, 资源包)}
}

// 恢复沙箱 在应用取得数据目录实例锁后、迁移和执行恢复之前回收中断的沙箱。
func 恢复沙箱(执行上下文 上下文包.上下文, 数据目录 string) error {
	return 运行包.恢复沙箱(执行上下文, 数据目录)
}
```
