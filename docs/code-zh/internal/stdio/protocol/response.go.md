```go
package 协议

import JSON包 "encoding/json"

// 响应 保留请求 标识，只输出结果或受控错误。
type 响应 struct {
	JSONRPC string         `json:"jsonrpc"`
	标识      JSON包.原始JSON消息 `json:"id"`
	结果      any            `json:"result,omitempty"`
	错误      *错误            `json:"error,omitempty"`
}

// 通知 是无请求 标识 的服务端事件。
type 通知 struct {
	JSONRPC string `json:"jsonrpc"`
	方法      string `json:"method"`
	参数      any    `json:"params"`
}
```
