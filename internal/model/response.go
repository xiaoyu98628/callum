package model

import "encoding/json"

type Response struct {
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
