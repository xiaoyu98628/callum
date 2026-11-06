package model

import "encoding/json"

type Response struct {
	// Reasoning 为空表示供应商未提供；非空指针保留显式空字段以便工具轮回传。
	Reasoning    *string `json:"reasoning,omitempty"`
	Text         string
	RequestID    string
	Content      []Block
	Output       json.RawMessage
	FinishReason string
	ToolCalls    []ToolCall
	Usage        json.RawMessage
	// InputTokens 是适配器从实际 usage 提取的输入用量；缺失时为空，不估算补齐。
	InputTokens *int64
}
