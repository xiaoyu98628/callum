```go
package 服务

import (
	上下文包 "context"

	智能体包 "github.com/xiaoyu98628/callum/internal/agent"
	执行器包 "github.com/xiaoyu98628/callum/internal/agent/executor"
	会话包 "github.com/xiaoyu98628/callum/internal/conversation"
)

// 执行协调器 是会话受理实际需要的执行协调能力，不包含工具、审批及步骤写入实现。
// 具体执行器由装配层注入；会话服务仍负责自己的持久化操作与提交后通知。
type 执行协调器 interface {
	准备执行(*执行器包.执行租约, 会话包.执行, 智能体包.已解析) func(上下文包.上下文)
	放弃(*执行器包.执行租约, 会话包.执行, error)
	执行压缩(上下文包.上下文, *执行器包.执行租约, 会话包.执行, 智能体包.已解析)
	引导执行租约(string) *执行器包.执行租约
	接受引导(*执行器包.执行租约, func() (会话包.后续消息, error)) error
	获取(上下文包.上下文, string) (智能体包.执行, error)
	实时上下文用量(会话包.上下文用量请求) (会话包.上下文用量, bool, error)
	预览上下文(上下文包.上下文, 会话包.上下文用量请求, 会话包.上下文状态, 智能体包.已解析) (会话包.上下文用量, error)
}
```
