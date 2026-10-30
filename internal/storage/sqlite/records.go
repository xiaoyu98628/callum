package sqlite

import "github.com/xiaoyu98628/callum/internal/model/provider"

// providerRecord 映射 model_providers（模型厂商连接配置表），不直接作为通信返回值。
// 每条记录保存一组协议、地址和凭据，同一厂商可以配置多个连接。
type providerRecord struct {
	// ID 是由服务端生成的 UUIDv7 主键，以标准小写文本保存。
	ID string `gorm:"type:text;primaryKey;not null;check:ck_provider_id,id GLOB '????????-????-7???-[89ab]???-????????????' AND length(replace(id,'-','')) = 32 AND replace(id,'-','') NOT GLOB '*[^0-9a-f]*'"`
	// Name 是厂商连接的展示名称，不要求唯一。
	Name string `gorm:"type:text;not null;check:ck_provider_name,length(trim(name)) > 0"`
	// Protocol 指定连接使用的接口协议，例如 chatcompletions。
	Protocol string `gorm:"type:text;not null;check:ck_provider_protocol,length(trim(protocol)) > 0"`
	// BaseURL 是该连接的 API 根地址，由关联模型共享。
	BaseURL string `gorm:"type:text;not null;check:ck_provider_url,length(trim(base_url)) > 0"`
	// APIKey 保存调用凭据，空字符串表示未配置；不得作为查询结果或日志输出。
	APIKey string `gorm:"type:text;not null;default:''" json:"-"`
	// CreatedAt 是创建时间，使用 UTC Unix 毫秒时间戳。
	CreatedAt int64 `gorm:"not null;autoCreateTime:milli"`
	// UpdatedAt 是最近更新时间，使用 UTC Unix 毫秒时间戳。
	UpdatedAt int64 `gorm:"not null;autoUpdateTime:milli"`
}

// TableName 指定模型厂商连接配置表名，避免依赖 GORM 的默认命名推断。
func (providerRecord) TableName() string { return "model_providers" }

// providerSummary 是厂商公开查询的数据库投影，既不读取明文凭据，也不让 ORM 映射业务类型。
type providerSummary struct {
	ID        string
	Name      string
	Protocol  string
	BaseURL   string
	HasAPIKey bool
	CreatedAt int64
	UpdatedAt int64
}

func (r providerSummary) toProvider() provider.Provider {
	return provider.Provider{ID: r.ID, Name: r.Name, Protocol: r.Protocol, BaseURL: r.BaseURL,
		HasAPIKey: r.HasAPIKey, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}
}

// modelRecord 映射 models（模型配置表），与客户端展示结构分开。
type modelRecord struct {
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
	Provider *providerRecord `gorm:"foreignKey:ProviderID;references:ID;constraint:OnDelete:RESTRICT" json:"-"`
}

// TableName 指定模型配置表名。
func (modelRecord) TableName() string { return "models" }

// toRecord 将配置转为数据库记录，统一将未选择功能保存为空数组。
func toRecord(in provider.ModelInput) modelRecord {
	types := in.ModelType
	if types == nil {
		types = []string{}
	}
	return modelRecord{
		ProviderID: in.ProviderID, DisplayName: in.DisplayName, RemoteModelID: in.RemoteModelID,
		Enabled: new(in.Enabled), ContextWindow: in.ContextWindow, MaxOutputTokens: in.MaxOutputTokens,
		Capabilities: in.Capabilities, ModelType: types, ReasoningLevels: in.ReasoningLevels,
	}
}

// fromRecord 还原数据库映射并检查必需的启用值，配置业务校验由 provider.Service 负责。
func fromRecord(row modelRecord) (provider.Model, error) {
	if row.Enabled == nil {
		return provider.Model{}, provider.ErrInvalidConfig
	}
	in := provider.ModelInput{
		ProviderID: row.ProviderID, DisplayName: row.DisplayName, RemoteModelID: row.RemoteModelID,
		Enabled: *row.Enabled, ContextWindow: row.ContextWindow, MaxOutputTokens: row.MaxOutputTokens,
		Capabilities: row.Capabilities, ModelType: row.ModelType, ReasoningLevels: row.ReasoningLevels,
	}
	return provider.Model{ModelInput: in, ID: row.ID, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}, nil
}
