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
}
