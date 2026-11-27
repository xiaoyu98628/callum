```go
package 智能体

// 操作事实 是当前消息分支上仍需核实的服务端操作事实，不含原始工具结果或凭据。
type 操作事实 struct {
	步骤标识 string `json:"step_id"`
	执行标识 string `json:"run_id"`
	工具   string `json:"tool"`
	请求哈希 string `json:"request_hash"`
	操作结果 string `json:"outcome"`
	历史键  string `json:"-"`
}
```
