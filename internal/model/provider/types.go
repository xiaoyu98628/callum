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
	// Capabilities 记录模型拥有的能力，不表示用户已启用这些能力。
	Capabilities map[string]bool
	// ModelType 保存用户选中的内容类型；包含 image 时启用图片输入，不作为工具开关。
	ModelType       []string
	ReasoningLevels []ReasoningLevel
}

// ValidModelType 判断用户选择的内容类型是否有效；适配器仍只提供已经实现的输入和输出。
func ValidModelType(kind string) bool {
	switch kind {
	case "text", "image", "audio", "video":
		return true
	default:
		return false
	}
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
	Model        Model
	ProviderName string
	Protocol     string
	BaseURL      string
	APIKey       string `json:"-"`
}

// ListOptions 使用不可变标识作为游标，不依赖会因增删而漂移的 offset。
type ListOptions struct {
	AfterID string `json:"after_id,omitempty"`
	Limit   int    `json:"limit,omitempty"`
}

// Page 是有界目录查询，只有 HasMore 时才提供下一页游标。
type Page[T any] struct {
	Items       []T    `json:"items"`
	HasMore     bool   `json:"has_more"`
	NextAfterID string `json:"next_after_id"`
}

const DefaultListLimit = 100
const MaxListLimit = 200

// MaxListSourceBytes 限制一页从存储读取的配置字节，通信层另检查实际 JSON 字节。
const MaxListSourceBytes = 2 << 20

// NormalizeListOptions 拒绝无效分页，不把错误参数解释成全量查询。
func NormalizeListOptions(values ...ListOptions) (ListOptions, error) {
	if len(values) > 1 {
		return ListOptions{}, ErrInvalidConfig
	}
	var value ListOptions
	if len(values) == 1 {
		value = values[0]
	}
	if value.Limit == 0 {
		value.Limit = DefaultListLimit
	}
	if value.Limit < 1 || value.Limit > MaxListLimit || (value.AfterID != "" && !ValidID(value.AfterID)) {
		return ListOptions{}, ErrInvalidConfig
	}
	return value, nil
}
