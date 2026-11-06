package model

type EventType string

const (
	EventTextDelta EventType = "text_delta"
	// EventReasoningDelta 只包含供应商明确返回的可展示推理文本。
	EventReasoningDelta EventType = "reasoning_delta"
	EventCompleted      EventType = "completed"
	// EventResult 在捕获模式下交付最终或部分结果，不表示执行成功。
	EventResult EventType = "result"
)

type Event struct {
	Type      EventType
	Text      string
	RequestID string
	Result    *Response
}
