package sqlite

import (
	"context"
	"time"

	"github.com/xiaoyu98628/callum/internal/model/provider"
	"gorm.io/gorm/clause"
)

const publicProviderColumns = "id, name, protocol, base_url, api_key <> '' AS has_api_key, created_at, updated_at"

// CreateProvider 保存业务层分配标识的厂商，未提供凭据时保存空字符串。
func (s *CatalogRepository) CreateProvider(ctx context.Context, id string, in provider.ProviderInput) error {
	row := providerRecord{ID: id, Name: in.Name, Protocol: in.Protocol, BaseURL: in.BaseURL}
	if in.APIKey != nil {
		row.APIKey = *in.APIKey
	}
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		return storageError(err)
	}
	return nil
}

// UpdateProvider 替换厂商连接信息；APIKey 为 nil 时保留原凭据，指向空字符串时清空。
func (s *CatalogRepository) UpdateProvider(ctx context.Context, id string, in provider.ProviderInput) error {
	fields := map[string]any{
		"name": in.Name, "protocol": in.Protocol, "base_url": in.BaseURL,
		"updated_at": time.Now().UTC().UnixMilli(),
	}
	if in.APIKey != nil {
		fields["api_key"] = *in.APIKey
	}
	return changed(s.db.WithContext(ctx).Model(&providerRecord{}).Where("id = ?", id).Updates(fields))
}

// GetProvider 查询不含明文凭据的厂商信息。
func (s *CatalogRepository) GetProvider(ctx context.Context, id string) (provider.Provider, error) {
	var row providerSummary
	err := s.db.WithContext(ctx).Model(&providerRecord{}).Select(publicProviderColumns).Where("id = ?", id).Take(&row).Error
	if err != nil {
		return provider.Provider{}, storageError(err)
	}
	return row.toProvider(), nil
}

// ListProviders 按本地标识排序返回厂商信息，查询结果不包含明文凭据。
func (s *CatalogRepository) ListProviders(ctx context.Context) ([]provider.Provider, error) {
	var rows []providerSummary
	if err := s.db.WithContext(ctx).Model(&providerRecord{}).Select(publicProviderColumns).Order("id").Find(&rows).Error; err != nil {
		return nil, storageError(err)
	}
	out := make([]provider.Provider, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.toProvider())
	}
	return out, nil
}

// DeleteProvider 删除厂商配置，仍被模型引用时返回 provider.ErrReferenced。
func (s *CatalogRepository) DeleteProvider(ctx context.Context, id string) error {
	return changed(s.db.WithContext(ctx).Omit(clause.Associations).Where("id = ?", id).Delete(&providerRecord{}))
}
