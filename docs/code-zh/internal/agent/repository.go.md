```go
package 智能体

import (
	上下文包 "context"

	模型包 "github.com/xiaoyu98628/callum/internal/model"
)

// 仓储 保存执行及步骤；恢复只能在取得应用数据目录独占运行权后调用。
type 仓储 interface {
	用户输入仓储
	检查点仓储
	获取(上下文包.上下文, string) (执行, error)
	列表(上下文包.上下文, string, int, int) ([]执行, error)
	获取步骤(上下文包.上下文, string) (步骤, error)
	步骤集(上下文包.上下文, string, int64, int) ([]步骤, error)
	获取步骤摘要(上下文包.上下文, string) (步骤摘要, error)
	步骤摘要集(上下文包.上下文, string, int64, int) ([]步骤摘要, error)
	步骤数据(上下文包.上下文, string, string, int, int) (步骤数据, error)
	创建步骤(上下文包.上下文, 步骤) (步骤, error)
	保存步骤(上下文包.上下文, 步骤) error
	保存消息(上下文包.上下文, string, []模型包.内容块, string) error
	结束(上下文包.上下文, 执行) error
	恢复(上下文包.上下文) error
	// 开始工具 原子认领尚未执行的工具步骤，核验审批及当前执行仍有效。
	开始工具(上下文包.上下文, string, string, string, string) (步骤, error)
	继续(上下文包.上下文, string) error
	// 执行轨迹 读取当前分支的已完成轨迹及未完成执行中的副作用事实，保证调用结果配对。
	// 不包含辅助标题、失败的最终回答或重发旧分支，不推断旧记录的操作结果。
	执行轨迹(上下文包.上下文, string, ...int64) (map[string][]模型包.消息, error)
	上下文执行轨迹(上下文包.上下文, string, int64, ...int64) (map[string][]模型包.消息, error)
	继续执行轨迹(上下文包.上下文, string, string) ([]模型包.消息, error)
	// 命令历史键 从可信请求快照识别同一命令，用于恢复历史权限约束。
	命令历史键(上下文包.上下文, []byte, string) (string, error)
	已拒绝操作键集(上下文包.上下文, string) ([]string, error)
	// 操作事实集 独立于摘要读取待核实操作，不允许模型摘要解除安全约束。
	操作事实集(上下文包.上下文, string) ([]操作事实, error)
}
```
