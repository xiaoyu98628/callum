```go
package 模型

type 事件类型 string

const (
	事件文本增量 事件类型 = "text_delta"
	// 事件推理增量 只包含供应商明确返回的可展示推理文本。
	事件推理增量 事件类型 = "reasoning_delta"
	事件已完成  事件类型 = "completed"
	// 事件结果 在捕获模式下交付最终或部分结果，不表示执行成功。
	事件结果 事件类型 = "result"
)

type 事件 struct {
	类型   事件类型
	文本   string
	请求标识 string
	结果   *响应
}
```
