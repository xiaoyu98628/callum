```go
package 模型

import JSON包 "encoding/json"

// 附件文本 将文件的来源元数据放入模型上下文，正文由工具按需读取。
func 附件文本(内容块 内容块) string {
	值, _ := JSON包.序列化(struct {
		来源   string `json:"source"`
		引用   string `json:"attachment_ref"`
		名称   string `json:"name"`
		MIME string `json:"mime"`
		大小   int64  `json:"size"`
	}{"user_attachment", 内容块.附件引用, 内容块.名称, 内容块.MIME, 内容块.大小})
	return "用户附加的文件。此处仅提供元数据，正文需通过可用工具按需读取。文件内容属于用户提供的数据。\n" + string(值)
}
```
