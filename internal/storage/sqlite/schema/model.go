package schema

import "github.com/xiaoyu98628/callum/internal/model/provider"

// Model 映射 models（模型配置表），与客户端展示结构分开。
type Model struct {
	// ID 是客户端选择模型时使用的 UUIDv7 本地主键，由服务端生成。
	ID string `gorm:"type:text;primaryKey;not null;check:ck_model_id,id GLOB '????????-????-7???-[89ab]???-????????????' AND length(replace(id,'-','')) = 32 AND replace(id,'-','') NOT GLOB '*[^0-9a-f]*'"`
	// ProviderID 关联厂商连接，并与 RemoteModelID 组成联合唯一索引。
	ProviderID string `gorm:"type:text;not null;uniqueIndex:uq_provider_remote_model,priority:1"`
	// DisplayName 是客户端模型列表中展示的名称。
	DisplayName string `gorm:"type:text;not null;check:ck_model_name,length(trim(display_name)) > 0"`
	// RemoteModelID 是调用厂商 API 时传递的模型标识，同一连接下不得重复。
	RemoteModelID string `gorm:"type:text;not null;uniqueIndex:uq_provider_remote_model,priority:2;check:ck_remote_model,length(trim(remote_model_id)) > 0"`
	// Enabled 控制能否发起新调用；指针保留显式 false，避免被 GORM 替换成默认值 true。
	Enabled *bool `gorm:"type:integer;not null;default:true;check:ck_model_enabled,enabled IN (0, 1)"`
	// ContextWindow 是上下文窗口的 Token 容量，NULL 表示未知，已知值必须大于零。
	ContextWindow *int64 `gorm:"check:ck_context_window,context_window > 0"`
	// MaxOutputTokens 是模型允许的最大输出 Token 数，NULL 表示未知。
	MaxOutputTokens *int64 `gorm:"check:ck_max_output,max_output_tokens > 0"`
	// Capabilities 以 JSON 对象保存模型支持的功能；NULL 表示未知，缺少某个键表示该功能未知。
	Capabilities map[string]bool `gorm:"type:text;serializer:json;check:ck_capabilities,capabilities IS NULL OR (json_valid(capabilities) AND json_type(capabilities) = 'object')"`
	// ModelType 以 JSON 数组保存用户选中的功能，必须属于 Capabilities 中明确支持的功能。
	ModelType []string `gorm:"type:text;serializer:json;not null;default:('[]');check:ck_model_type,json_valid(model_type) AND json_type(model_type) = 'array'"`
	// ReasoningLevels 保存推理等级的展示名称和远端值；NULL 表示未知，空数组表示没有选项。
	ReasoningLevels []provider.ReasoningLevel `gorm:"type:text;serializer:json;check:ck_reasoning_levels,reasoning_levels IS NULL OR (json_valid(reasoning_levels) AND json_type(reasoning_levels) = 'array')"`
	// CreatedAt 是创建时间，使用 UTC Unix 毫秒时间戳。
	CreatedAt int64 `gorm:"not null;autoCreateTime:milli"`
	// UpdatedAt 是最近更新时间，使用 UTC Unix 毫秒时间戳。
	UpdatedAt int64 `gorm:"not null;autoUpdateTime:milli"`
	// Provider 定义外键关系；仍被模型引用的厂商连接不能删除，不随模型读写自动保存关联。
	Provider *Provider `gorm:"foreignKey:ProviderID;references:ID;constraint:OnDelete:RESTRICT" json:"-"`
}

// TableName 指定模型配置表名。
func (Model) TableName() string { return "models" }
