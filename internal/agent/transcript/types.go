// Package transcript 定义执行轨迹的恢复规则，不依赖存储实现或通信协议。
package transcript

import "encoding/json"

// Entry 是已按执行内顺序读取的轨迹投影；JSON 和指针内容在调用期间只读。
// Message 为统一模型消息，Operation 为独立保存的操作事实。
type Entry struct {
	StepID       string
	ParentStepID *string
	ToolCallID   *string
	Message      json.RawMessage
	Operation    json.RawMessage
}
