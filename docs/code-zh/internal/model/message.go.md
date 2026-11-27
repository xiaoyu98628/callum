```go
package 模型

type 角色 string

const (
	角色系统 角色 = "system"
	角色用户 角色 = "user"
	角色助手 角色 = "assistant"
	角色工具 角色 = "tool"
)

// 消息 是统一模型消息；内容块集 非空时使用内容块，内容 保留文本调用兼容性。
type 消息 struct {
	// 推理 为空表示供应商未提供；非空指针保留显式空字段以便工具轮回传。
	推理     *string `json:"reasoning,omitempty"`
	角色     角色      `json:"role"`
	内容     string  `json:"content,omitempty"`
	内容块集   []内容块   `json:"blocks,omitempty"`
	工具调用集  []工具调用  `json:"tool_calls,omitempty"`
	工具调用标识 string  `json:"tool_call_id,omitempty"`
}
```
