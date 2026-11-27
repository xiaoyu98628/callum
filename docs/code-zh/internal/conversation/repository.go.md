```go
package 会话

import (
	上下文包 "context"
	模型包 "github.com/xiaoyu98628/callum/internal/model"
)

// 仓储 按业务动作保证跨会话、消息、执行和步骤的事务一致性。
// 开始 返回后不得占用连接；生成标题只允许条件更新未被用户修改的默认标题。
type 仓储 interface {
	上下文仓储
	获取(上下文包.上下文, string) (会话, error)
	列表(上下文包.上下文, 筛选) ([]会话, error)
	更新(上下文包.上下文, string, 变更) (会话, error)
	消息(上下文包.上下文, string, string) (消息, error)
	附件(上下文包.上下文, string, string) (模型包.内容块, error)
	消息集(上下文包.上下文, string, int64, int) ([]消息, error)
	历史(上下文包.上下文, string, string, int) (历史页, error)
	后续消息集(上下文包.上下文, string) ([]后续消息, error)
	后续消息(上下文包.上下文, string, string) (后续消息, error)
	创建后续消息(上下文包.上下文, 后续消息) (后续消息, error)
	变更后续消息(上下文包.上下文, 后续消息变更, []模型包.内容块) (后续消息, error)
	待处理引导(上下文包.上下文, string) ([]后续消息, error)
	应用引导(上下文包.上下文, string, string, []string) ([]消息, error)
	开始(上下文包.上下文, 开始) (执行, error)
	应用标题(上下文包.上下文, string, string, string, string) (会话, bool, error)
}
```
