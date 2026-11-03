package model

import "encoding/json"

// ToolDefinition 描述提供给模型的可用工具，不包含执行权限。
type ToolDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// ToolCall 是模型提出的完整调用；接收完整且校验成功后才可执行。
type ToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}
