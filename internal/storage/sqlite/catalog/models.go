package catalog

import (
	"context"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/xiaoyu98628/callum/internal/credentials"
	"github.com/xiaoyu98628/callum/internal/model/provider"
	"github.com/xiaoyu98628/callum/internal/storage/sqlite/schema"
)

// CreateModel 保存业务层分配标识的模型，保留显式 false，关联约束由数据库检查。
func (s *Repository) CreateModel(ctx context.Context, id string, in provider.ModelInput) error {
	row := toRecord(in)
	row.ID = id
	if err := s.db.WithContext(ctx).Omit(clause.Associations).Create(&row).Error; err != nil {
		return storageError(err)
	}
	return nil
}

// UpdateModel 替换完整模型配置，保留记录标识和创建时间。
func (s *Repository) UpdateModel(ctx context.Context, id string, in provider.ModelInput) error {
	row := toRecord(in)
	row.UpdatedAt = time.Now().UTC().UnixMilli()
	// 显式选择完整配置字段，包含 false、空数组以及需要清空的 NULL。
	return changed(s.db.WithContext(ctx).Model(&schema.Model{}).Where("id = ?", id).
		Select("ProviderID", "DisplayName", "RemoteModelID", "Enabled", "ContextWindow",
			"MaxOutputTokens", "Capabilities", "ModelType", "ReasoningLevels", "UpdatedAt").Updates(&row))
}

// GetModel 查询模型配置，包含禁用的记录。
func (s *Repository) GetModel(ctx context.Context, id string) (provider.Model, error) {
	var row schema.Model
	if err := s.db.WithContext(ctx).Where("id = ?", id).Take(&row).Error; err != nil {
		return provider.Model{}, storageError(err)
	}
	return fromRecord(row)
}

// ListModels 按本地标识排序返回模型；selectableOnly 为 true 时只返回已启用记录。
// 实际调用前还需检查协议适配器是否支持配置中启用的功能。
func (s *Repository) ListModels(ctx context.Context, selectableOnly bool, options ...provider.ListOptions) (out provider.Page[provider.Model], err error) {
	value, err := provider.NormalizeListOptions(options...)
	if err != nil {
		return out, err
	}
	out.Items = []provider.Model{}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		query := tx.Model(&schema.Model{})
		if selectableOnly {
			query = query.Where("enabled = ?", true)
		}
		// 只预读有界元数据，再加载同一事务内的记录，避免先把大目录全部解码。
		ids, more, err := listPageIDs(query, value, `length(CAST(display_name AS BLOB))+length(CAST(remote_model_id AS BLOB))+length(CAST(provider_id AS BLOB))+COALESCE(length(CAST(capabilities AS BLOB)),0)+COALESCE(length(CAST(model_type AS BLOB)),0)+COALESCE(length(CAST(reasoning_levels AS BLOB)),0)+1024`)
		if err != nil {
			return err
		}
		if len(ids) == 0 {
			return nil
		}
		var rows []schema.Model
		if err := tx.Where("id IN ?", ids).Order("id").Find(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			m, err := fromRecord(row)
			if err != nil {
				return err
			}
			out.Items = append(out.Items, m)
		}
		out.HasMore = more
		if more {
			out.NextAfterID = ids[len(ids)-1]
		}
		return nil
	})
	return out, storageError(err)
}

func listPageIDs(query *gorm.DB, options provider.ListOptions, byteExpression string) ([]string, bool, error) {
	var sizes []struct {
		ID    string
		Bytes int64
	}
	if err := query.Where("id > ?", options.AfterID).Select("id, " + byteExpression + " AS bytes").Order("id").Limit(options.Limit + 1).Scan(&sizes).Error; err != nil {
		return nil, false, err
	}
	ids := make([]string, 0, min(options.Limit, len(sizes)))
	var total int64
	for _, row := range sizes {
		if len(ids) == options.Limit || total+row.Bytes > provider.MaxListSourceBytes {
			if len(ids) == 0 {
				return nil, false, provider.ErrInvalidConfig
			}
			return ids, true, nil
		}
		ids = append(ids, row.ID)
		total += row.Bytes
	}
	return ids, false, nil
}

// SetModelEnabled 控制模型能否用于新的运行配置解析。
func (s *Repository) SetModelEnabled(ctx context.Context, id string, enabled bool) error {
	return changed(s.db.WithContext(ctx).Model(&schema.Model{}).Where("id = ?", id).Updates(map[string]any{
		"enabled": enabled, "updated_at": time.Now().UTC().UnixMilli(),
	}))
}

// DeleteModel 删除模型配置，不存在时返回 provider.ErrNotFound。
func (s *Repository) DeleteModel(ctx context.Context, id string) error {
	return changed(s.db.WithContext(ctx).Omit(clause.Associations).Where("id = ?", id).Delete(&schema.Model{}))
}

// LoadRuntime 使用一次 JOIN 读取模型与厂商快照，不持有跨越模型调用的事务。
// 此处只读取数据，启用规则及配置校验由业务服务负责。
func (s *Repository) LoadRuntime(ctx context.Context, id string) (provider.RuntimeConfig, error) {
	var row struct {
		Record                  schema.Model `gorm:"embedded"`
		RuntimeProtocol         string
		RuntimeProviderName     string
		RuntimeBaseURL          string
		RuntimeAPIKey           string `json:"-"`
		RuntimeCredentialFormat int
	}
	err := s.db.WithContext(ctx).Model(&schema.Model{}).
		Select("models.*, p.name AS runtime_provider_name, p.protocol AS runtime_protocol, p.base_url AS runtime_base_url, p.api_key AS runtime_api_key, p.credential_format AS runtime_credential_format").
		Joins("JOIN model_providers p ON p.id = models.provider_id").Where("models.id = ?", id).Take(&row).Error
	if err != nil {
		return provider.RuntimeConfig{}, storageError(err)
	}
	m, err := fromRecord(row.Record)
	if err != nil {
		return provider.RuntimeConfig{}, err
	}
	if row.RuntimeAPIKey != "" && row.RuntimeCredentialFormat != 1 {
		return provider.RuntimeConfig{}, storageError(credentials.ErrInvalid)
	}
	key, err := s.cipher.Open(ctx, m.ProviderID, row.RuntimeAPIKey)
	if err != nil {
		return provider.RuntimeConfig{}, storageError(err)
	}
	return provider.RuntimeConfig{Model: m, ProviderName: row.RuntimeProviderName, Protocol: row.RuntimeProtocol, BaseURL: row.RuntimeBaseURL, APIKey: key}, nil
}
