package provider

// Provider 是不含凭据的厂商配置，可用于列表和编辑信息查询。
type Provider struct {
	ID        string
	Name      string
	Protocol  string
	BaseURL   string
	HasAPIKey bool
	CreatedAt int64
	UpdatedAt int64
}

// ProviderInput 是完整配置更新；APIKey 为 nil 时保留已有凭据，非 nil 时替换。
// 此结构供服务内部使用，不直接作为通信协议结构。
type ProviderInput struct {
	Name     string
	Protocol string
	BaseURL  string
	APIKey   *string `json:"-"`
}

// ReasoningLevel 保存推理等级的本地名称和远端参数值。
type ReasoningLevel struct {
	DisplayName string `json:"display_name"`
	RemoteValue string `json:"remote_value"`
}

// ModelInput 保存模型规格及启用设置。
// 能力、推理等级和 Token 容量为 nil 时表示未知；ModelType 为 nil 时按空数组保存。
type ModelInput struct {
	ProviderID      string
	DisplayName     string
	RemoteModelID   string
	Enabled         bool
	ContextWindow   *int64
	MaxOutputTokens *int64
	Capabilities    map[string]bool
	ModelType       []string
	ReasoningLevels []ReasoningLevel
}

// Model 是包含本地记录标识与时间戳的模型配置，不代表适配器已实现其声明的能力。
type Model struct {
	ModelInput
	ID        string
	CreatedAt int64
	UpdatedAt int64
}

// RuntimeConfig 仅用于内部模型调用装配，包含凭据，不得整体写入日志。
type RuntimeConfig struct {
	Model    Model
	Protocol string
	BaseURL  string
	APIKey   string `json:"-"`
}
