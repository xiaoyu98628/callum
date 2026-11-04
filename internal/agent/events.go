package agent

// ToolOutput 是即时工具输出，不逐片持久化；Data 为 base64，避免拆分 UTF-8 字符或破坏 JSONL。
// 最终有界结果由工具步骤保存，Sequence 仅在对应步骤内递增。
type ToolOutput struct {
	Sequence int64  `json:"sequence"`
	Stream   string `json:"stream"`
	Encoding string `json:"encoding"`
	Data     string `json:"data"`
}

// StepSummary 是通知和列表使用的步骤元数据，大型原始结果通过查询读取。
type StepSummary struct {
	Step
	OutputBytes     int `json:"output_bytes"`
	UsageBytes      int `json:"usage_bytes"`
	DefinitionBytes int `json:"definition_bytes"`
}

// SummarizeStep 在事件进入订阅队列之前去掉原始结果，避免队列持有大型响应。
func SummarizeStep(step Step) StepSummary {
	out := StepSummary{Step: step, OutputBytes: len(step.Output), UsageBytes: len(step.Usage), DefinitionBytes: len(step.Definition)}
	out.Output, out.Usage, out.Definition = nil, nil, nil
	// 事件可能晚于步骤更新才被消费，不能继续引用执行过程中的可变指针。
	out.ParentStepID = snapshotPointer(step.ParentStepID)
	out.Model = snapshotPointer(step.Model)
	out.ProviderRequestID = snapshotPointer(step.ProviderRequestID)
	out.ToolCallID = snapshotPointer(step.ToolCallID)
	out.ErrorKind = snapshotPointer(step.ErrorKind)
	out.ErrorMessage = snapshotPointer(step.ErrorMessage)
	out.StartedAt = snapshotPointer(step.StartedAt)
	out.FinishedAt = snapshotPointer(step.FinishedAt)
	return out
}

func snapshotPointer[T any](value *T) *T {
	if value == nil {
		return nil
	}
	return new(*value)
}
