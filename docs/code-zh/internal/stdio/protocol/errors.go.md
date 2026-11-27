```go
package 协议

// 记录错误 是可公开输出的协议错误，不包含原始错误链或凭据。
type 错误 struct {
	代码 int    `json:"code"`
	消息 string `json:"message"`
	数据 any    `json:"data,omitempty"`
}

// 无效参数 返回参数校验失败的受控描述。
func 无效参数() *错误 {
	return &错误{代码: -32602, 消息: "参数无效，请检查字段、类型和必填值"}
}

// 方法未找到 表示当前服务没有注册该接口。
func 方法未找到() *错误 { return &错误{代码: -32601, 消息: "接口不存在"} }
```
