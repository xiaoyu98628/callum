```go
// 包 执行轨迹 定义执行轨迹的恢复规则，不依赖存储实现或通信协议。
package 执行轨迹

import JSON包 "encoding/json"

// 条目 是已按执行内顺序读取的轨迹投影；JSON 和指针内容在调用期间只读。
// 消息 为统一模型消息，操作 为独立保存的操作事实。
type 条目 struct {
	步骤标识   string
	父级步骤标识 *string
	工具调用标识 *string
	消息     JSON包.原始JSON消息
	操作     JSON包.原始JSON消息
}
```
