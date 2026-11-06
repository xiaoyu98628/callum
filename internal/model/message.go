package model

type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// Message 是统一模型消息；Blocks 非空时使用内容块，Content 保留文本调用兼容性。
type Message struct {
	// Reasoning 为空表示供应商未提供；非空指针保留显式空字段以便工具轮回传。
	Reasoning  *string    `json:"reasoning,omitempty"`
	Role       Role       `json:"role"`
	Content    string     `json:"content,omitempty"`
	Blocks     []Block    `json:"blocks,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}
