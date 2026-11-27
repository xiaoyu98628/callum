```go
package 工作区

import (
	上下文包 "context"
	错误判断包 "errors"

	协议包 "github.com/xiaoyu98628/callum/internal/stdio/protocol"
	业务域包 "github.com/xiaoyu98628/callum/internal/workspace"
)

func 公开错误(错误 error) *协议包.错误 {
	if 错误 == nil {
		return nil
	}
	switch {
	case 错误判断包.是否属于(错误, 业务域包.无效错误):
		return 协议包.无效参数()
	case 错误判断包.是否属于(错误, 业务域包.未找到错误):
		return &协议包.错误{代码: 1001, 消息: "工作区不存在"}
	case 错误判断包.是否属于(错误, 业务域包.繁忙错误):
		return &协议包.错误{代码: 1006, 消息: "工作区有未结束执行"}
	case 错误判断包.是否属于(错误, 业务域包.冲突错误):
		return &协议包.错误{代码: 1003, 消息: "工作区路径已存在"}
	case 错误判断包.是否属于(错误, 业务域包.变化通知错误):
		return &协议包.错误{代码: 1003, 消息: "工作区文件夹已更新，请重新打开项目编辑"}
	case 错误判断包.是否属于(错误, 上下文包.已取消错误):
		return &协议包.错误{代码: 1007, 消息: "操作已取消"}
	case 错误判断包.是否属于(错误, 上下文包.超出截止时间错误):
		return &协议包.错误{代码: 1008, 消息: "操作超时"}
	default:
		return &协议包.错误{代码: -32603, 消息: "工作区操作失败"}
	}
}
```
