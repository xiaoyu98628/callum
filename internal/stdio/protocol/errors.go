package protocol

// Error 是可公开输出的协议错误，不包含原始错误链或凭据。
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// InvalidParams 返回参数校验失败的受控描述。
func InvalidParams() *Error {
	return &Error{Code: -32602, Message: "参数无效，请检查字段、类型和必填值"}
}
