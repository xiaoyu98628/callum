```go
package 模型

import JSON包 "encoding/json"

// 工具定义 描述提供给模型的可用工具，不包含执行权限。
type 工具定义 struct {
	名称 string         `json:"name"`
	说明 string         `json:"description"`
	参数 JSON包.原始JSON消息 `json:"parameters"`
}

// 工具调用 是模型提出的完整调用；接收完整且校验成功后才可执行。
type 工具调用 struct {
	标识 string         `json:"id"`
	名称 string         `json:"name"`
	参数 JSON包.原始JSON消息 `json:"arguments"`
}
```
