```go
package 执行

import (
	推理调用包 "github.com/xiaoyu98628/callum/internal/inference"
)

// 发出事件 将业务执行事件映射为协议通知，供装配层作为事件回调注入执行管理器。
// 本方法支持并发调用，错误描述在协议边界转换，原始错误链不参与序列化。
func 发出事件(通知 func(string, any) error, 事件 推理调用包.事件) error {
	return 发布持久化(通知, 推理调用包.公开事件(事件))
}
```
