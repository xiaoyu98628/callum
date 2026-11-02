package catalog

import (
	"github.com/xiaoyu98628/callum/internal/model/provider"
	"github.com/xiaoyu98628/callum/internal/storage/sqlite/schema"
)

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

// toRecord 将配置转为数据库记录，统一将未选择功能保存为空数组。
func toRecord(in provider.ModelInput) schema.Model {
	types := in.ModelType
	if types == nil {
		types = []string{}
	}
	return schema.Model{
		ProviderID: in.ProviderID, DisplayName: in.DisplayName, RemoteModelID: in.RemoteModelID,
		Enabled: new(in.Enabled), ContextWindow: in.ContextWindow, MaxOutputTokens: in.MaxOutputTokens,
		Capabilities: in.Capabilities, ModelType: types, ReasoningLevels: in.ReasoningLevels,
	}
}

// fromRecord 还原数据库映射并检查必需的启用值，完整配置校验由业务服务负责。
func fromRecord(row schema.Model) (provider.Model, error) {
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
