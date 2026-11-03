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
	Role    Role
	Content string
	Blocks  []Block `json:"-"`
}
