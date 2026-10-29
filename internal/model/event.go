package model

type EventType string

const (
	EventTextDelta EventType = "text_delta"
	EventCompleted EventType = "completed"
)

type Event struct {
	Type      EventType
	Text      string
	RequestID string
}
