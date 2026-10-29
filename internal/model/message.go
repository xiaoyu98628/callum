package model

type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// Message 是第一阶段使用的文本消息。
type Message struct {
	Role    Role
	Content string
}
