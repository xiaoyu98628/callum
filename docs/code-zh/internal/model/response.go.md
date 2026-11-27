```go
package 模型

import JSON包 "encoding/json"

type 响应 struct {
	// 推理 为空表示供应商未提供；非空指针保留显式空字段以便工具轮回传。
	推理    *string `json:"reasoning,omitempty"`
	文本    string
	请求标识  string
	内容    []内容块
	输出    JSON包.原始JSON消息
	结束原因  string
	工具调用集 []工具调用
	用量    JSON包.原始JSON消息
	// 输入词元 是适配器从实际 用量 提取的输入用量；缺失时为空，不估算补齐。
	输入词元 *int64
}
```
