package sqlite

import (
	"context"
	"time"

	"github.com/xiaoyu98628/callum/internal/model/provider"
	"gorm.io/gorm/clause"
)

// CreateModel 保存业务层分配标识的模型，保留显式 false，关联约束由数据库检查。
func (s *CatalogRepository) CreateModel(ctx context.Context, id string, in provider.ModelInput) error {
	row := toRecord(in)
	row.ID = id
	if err := s.db.WithContext(ctx).Omit(clause.Associations).Create(&row).Error; err != nil {
		return storageError(err)
	}
	return nil
}

// UpdateModel 替换完整模型配置，保留记录标识和创建时间。
func (s *CatalogRepository) UpdateModel(ctx context.Context, id string, in provider.ModelInput) error {
	row := toRecord(in)
	row.UpdatedAt = time.Now().UTC().UnixMilli()
	// 显式选择完整配置字段，包含 false、空数组以及需要清空的 NULL。
	return changed(s.db.WithContext(ctx).Model(&modelRecord{}).Where("id = ?", id).
		Select("ProviderID", "DisplayName", "RemoteModelID", "Enabled", "ContextWindow",
			"MaxOutputTokens", "Capabilities", "ModelType", "ReasoningLevels", "UpdatedAt").Updates(&row))
}

// GetModel 查询模型配置，包含禁用的记录。
func (s *CatalogRepository) GetModel(ctx context.Context, id string) (provider.Model, error) {
	var row modelRecord
	if err := s.db.WithContext(ctx).Where("id = ?", id).Take(&row).Error; err != nil {
		return provider.Model{}, storageError(err)
	}
	return fromRecord(row)
}

// ListModels 按本地标识排序返回模型；selectableOnly 为 true 时只返回已启用记录。
// 实际调用前还需检查协议适配器是否支持配置中启用的功能。
func (s *CatalogRepository) ListModels(ctx context.Context, selectableOnly bool) ([]provider.Model, error) {
	query := s.db.WithContext(ctx).Model(&modelRecord{}).Order("id")
	if selectableOnly {
		query = query.Where("enabled = ?", true)
	}
	var rows []modelRecord
	if err := query.Find(&rows).Error; err != nil {
		return nil, storageError(err)
	}
	out := make([]provider.Model, 0, len(rows))
	for _, row := range rows {
		m, err := fromRecord(row)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, nil
}

// SetModelEnabled 控制模型能否用于新的运行配置解析。
func (s *CatalogRepository) SetModelEnabled(ctx context.Context, id string, enabled bool) error {
	return changed(s.db.WithContext(ctx).Model(&modelRecord{}).Where("id = ?", id).Updates(map[string]any{
		"enabled": enabled, "updated_at": time.Now().UTC().UnixMilli(),
	}))
}

// DeleteModel 删除模型配置，不存在时返回 provider.ErrNotFound。
func (s *CatalogRepository) DeleteModel(ctx context.Context, id string) error {
	return changed(s.db.WithContext(ctx).Omit(clause.Associations).Where("id = ?", id).Delete(&modelRecord{}))
}

// LoadRuntime 使用一次 JOIN 读取模型与厂商快照，不持有跨越模型调用的事务。
// 此处只读取数据，启用规则及配置业务校验由 provider.Service 负责。
func (s *CatalogRepository) LoadRuntime(ctx context.Context, id string) (provider.RuntimeConfig, error) {
	var row struct {
		Record          modelRecord `gorm:"embedded"`
		RuntimeProtocol string
		RuntimeBaseURL  string
		RuntimeAPIKey   string `json:"-"`
	}
	err := s.db.WithContext(ctx).Model(&modelRecord{}).
		Select("models.*, p.protocol AS runtime_protocol, p.base_url AS runtime_base_url, p.api_key AS runtime_api_key").
		Joins("JOIN model_providers p ON p.id = models.provider_id").Where("models.id = ?", id).Take(&row).Error
	if err != nil {
		return provider.RuntimeConfig{}, storageError(err)
	}
	m, err := fromRecord(row.Record)
	if err != nil {
		return provider.RuntimeConfig{}, err
	}
	return provider.RuntimeConfig{Model: m, Protocol: row.RuntimeProtocol, BaseURL: row.RuntimeBaseURL, APIKey: row.RuntimeAPIKey}, nil
}
