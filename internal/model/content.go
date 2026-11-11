package model

// Block 是对话内容块。图片来源是导入路径或服务端保存的附件引用。
type Block struct {
	Type          string `json:"type"`
	Text          string `json:"text,omitempty"`
	Path          string `json:"path,omitempty"`
	AttachmentRef string `json:"attachment_ref,omitempty"`
	Name          string `json:"name,omitempty"`
	MIME          string `json:"mime,omitempty"`
	Size          int64  `json:"size,omitempty"`
}

// Snapshot 是不含凭据的模型身份快照，不替代实时运行配置。
type Snapshot struct {
	ID            string           `json:"id"`
	DisplayName   string           `json:"display_name"`
	RemoteModelID string           `json:"remote_model_id"`
	Provider      ProviderSnapshot `json:"provider"`
}

// ProviderSnapshot 保存历史展示所需的厂商身份。
type ProviderSnapshot struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
