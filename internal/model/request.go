package model

type Request struct {
	// Model 是实际发送给远端接口的模型 ID。
	Model    string
	Messages []Message
	// Tools 由服务端提供，模型自主决定是否调用。
	Tools []ToolDefinition
	// CaptureResult 要求流式调用返回组装结果及原始 usage，不改变正文增量。
	CaptureResult bool
	// AllowImageInput 由已验证的模型功能选择设置，历史图片也必须受此限制。
	AllowImageInput bool
}
