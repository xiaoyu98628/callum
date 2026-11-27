```go
package 模型

// 内容块 是对话内容块。技能 使用 名称 和 路径 保存已核验的技能引用。
// 图片来源是导入路径或服务端保存的附件引用。
type 内容块 struct {
	类型   string `json:"type"`
	文本   string `json:"text,omitempty"`
	路径   string `json:"path,omitempty"`
	附件引用 string `json:"attachment_ref,omitempty"`
	名称   string `json:"name,omitempty"`
	MIME string `json:"mime,omitempty"`
	大小   int64  `json:"size,omitempty"`
}

// 快照 是不含凭据的模型身份快照，不替代实时运行配置。
type 快照 struct {
	标识     string `json:"id"`
	显示名称   string `json:"display_name"`
	远程模型标识 string `json:"remote_model_id"`
	供应商    供应商快照  `json:"provider"`
}

// 供应商快照 保存历史展示所需的厂商身份。
type 供应商快照 struct {
	标识 string `json:"id"`
	名称 string `json:"name"`
}
```
