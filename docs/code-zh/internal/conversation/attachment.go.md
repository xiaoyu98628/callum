```go
package 会话

import 错误判断包 "errors"

var 附件变化通知错误 = 错误判断包.新建("附件所属消息已变化，请从当前消息重新另存")

// 附件分片 是按固定消息索引读取的有界原始附件片段；数据 由 JSON 编码为 基准64。
type 附件分片 struct {
	执行标识   string `json:"run_id"`
	SHA256 string `json:"sha256"`
	名称     string `json:"name"`
	MIME   string `json:"mime"`
	大小     int64  `json:"size"`
	偏移     int64  `json:"offset"`
	数据     []byte `json:"data"`
	下一个偏移  *int64 `json:"next_offset"`
}
```
