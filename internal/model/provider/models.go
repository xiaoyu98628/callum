package provider

import (
	"context"
	"time"

	"gorm.io/gorm/clause"
)

// CreateModel 新增模型配置；调用方必须明确指定 Enabled，返回本地模型记录标识。
func (s *Store) CreateModel(ctx context.Context, in ModelInput) (string, error) {
	if err := validateModel(in); err != nil {
		return "", err
	}
	row := toRecord(in)
	id, err := newID()
	if err != nil {
		return "", err
	}
	row.ID = id
	if err := s.db.WithContext(ctx).Omit(clause.Associations).Create(&row).Error; err != nil {
		return "", storageError(err)
	}
	return row.ID, nil
}

// UpdateModel 替换完整模型配置，保留记录标识和创建时间。
func (s *Store) UpdateModel(ctx context.Context, id string, in ModelInput) error {
	if !ValidID(id) {
		return ErrInvalidConfig
	}
	if err := validateModel(in); err != nil {
		return err
	}
	row := toRecord(in)
	row.UpdatedAt = time.Now().UTC().UnixMilli()
	// 显式选择完整配置字段，包含 false、空数组以及需要清空的 NULL。
	return changed(s.db.WithContext(ctx).Model(&ModelRecord{}).Where("id = ?", id).
		Select("ProviderID", "DisplayName", "RemoteModelID", "Enabled", "ContextWindow",
			"MaxOutputTokens", "Capabilities", "ModelType", "ReasoningLevels", "UpdatedAt").Updates(&row))
}

// GetModel 查询模型配置，包含禁用的记录。
func (s *Store) GetModel(ctx context.Context, id string) (Model, error) {
	if !ValidID(id) {
		return Model{}, ErrInvalidConfig
	}
	var row ModelRecord
	if err := s.db.WithContext(ctx).Where("id = ?", id).Take(&row).Error; err != nil {
		return Model{}, storageError(err)
	}
	return fromRecord(row)
}

// ListModels 按本地标识排序返回模型；selectableOnly 为 true 时只返回已启用记录。
// 实际调用前还需检查协议适配器是否支持配置中启用的功能。
func (s *Store) ListModels(ctx context.Context, selectableOnly bool) ([]Model, error) {
	query := s.db.WithContext(ctx).Model(&ModelRecord{}).Order("id")
	if selectableOnly {
		query = query.Where("enabled = ?", true)
	}
	var rows []ModelRecord
	if err := query.Find(&rows).Error; err != nil {
		return nil, storageError(err)
	}
	out := make([]Model, 0, len(rows))
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
func (s *Store) SetModelEnabled(ctx context.Context, id string, enabled bool) error {
	if !ValidID(id) {
		return ErrInvalidConfig
	}
	return changed(s.db.WithContext(ctx).Model(&ModelRecord{}).Where("id = ?", id).Updates(map[string]any{
		"enabled": enabled, "updated_at": time.Now().UTC().UnixMilli(),
	}))
}

// DeleteModel 删除模型配置，不存在时返回 ErrNotFound。
func (s *Store) DeleteModel(ctx context.Context, id string) error {
	if !ValidID(id) {
		return ErrInvalidConfig
	}
	return changed(s.db.WithContext(ctx).Omit(clause.Associations).Where("id = ?", id).Delete(&ModelRecord{}))
}

// Resolve 使用一次 JOIN 查询读取模型与厂商配置，不在模型网络调用期间持有事务。
func (s *Store) Resolve(ctx context.Context, id string) (RuntimeConfig, error) {
	if !ValidID(id) {
		return RuntimeConfig{}, ErrInvalidConfig
	}
	var row struct {
		ModelRecord     `gorm:"embedded"`
		RuntimeProtocol string
		RuntimeBaseURL  string
		RuntimeAPIKey   string `json:"-"`
	}
	err := s.db.WithContext(ctx).Model(&ModelRecord{}).
		Select("models.*, p.protocol AS runtime_protocol, p.base_url AS runtime_base_url, p.api_key AS runtime_api_key").
		Joins("JOIN model_providers p ON p.id = models.provider_id").Where("models.id = ?", id).Take(&row).Error
	if err != nil {
		return RuntimeConfig{}, storageError(err)
	}
	m, err := fromRecord(row.ModelRecord)
	if err != nil {
		return RuntimeConfig{}, err
	}
	if !m.Enabled {
		return RuntimeConfig{}, ErrDisabled
	}
	return RuntimeConfig{Model: m, Protocol: row.RuntimeProtocol, BaseURL: row.RuntimeBaseURL, APIKey: row.RuntimeAPIKey}, nil
}
