package agent

// OperationFact 是当前消息分支上仍需核实的服务端操作事实，不含原始工具结果或凭据。
type OperationFact struct {
	StepID      string `json:"step_id"`
	RunID       string `json:"run_id"`
	Tool        string `json:"tool"`
	RequestHash string `json:"request_hash"`
	Outcome     string `json:"outcome"`
}
