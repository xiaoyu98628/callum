package provider

import (
	"context"
	"time"

	"gorm.io/gorm/clause"
)

const publicProviderColumns = "id, name, protocol, base_url, api_key <> '' AS has_api_key, created_at, updated_at"

// CreateProvider 新增厂商配置并返回本地记录标识，未提供凭据时保存空字符串。
func (s *Store) CreateProvider(ctx context.Context, in ProviderInput) (string, error) {
	if err := validateProvider(in); err != nil {
		return "", err
	}
	id, err := newID()
	if err != nil {
		return "", err
	}
	row := ProviderRecord{ID: id, Name: in.Name, Protocol: in.Protocol, BaseURL: in.BaseURL}
	if in.APIKey != nil {
		row.APIKey = *in.APIKey
	}
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		return "", storageError(err)
	}
	return row.ID, nil
}

// UpdateProvider 替换厂商连接信息；APIKey 为 nil 时保留原凭据，指向空字符串时清空。
func (s *Store) UpdateProvider(ctx context.Context, id string, in ProviderInput) error {
	if !ValidID(id) {
		return ErrInvalidConfig
	}
	if err := validateProvider(in); err != nil {
		return err
	}
	fields := map[string]any{
		"name": in.Name, "protocol": in.Protocol, "base_url": in.BaseURL,
		"updated_at": time.Now().UTC().UnixMilli(),
	}
	if in.APIKey != nil {
		fields["api_key"] = *in.APIKey
	}
	return changed(s.db.WithContext(ctx).Model(&ProviderRecord{}).Where("id = ?", id).Updates(fields))
}

// GetProvider 查询不含明文凭据的厂商信息。
func (s *Store) GetProvider(ctx context.Context, id string) (Provider, error) {
	if !ValidID(id) {
		return Provider{}, ErrInvalidConfig
	}
	var out Provider
	err := s.db.WithContext(ctx).Model(&ProviderRecord{}).Select(publicProviderColumns).Where("id = ?", id).Take(&out).Error
	return out, storageError(err)
}

// ListProviders 按本地标识排序返回厂商信息，查询结果不包含明文凭据。
func (s *Store) ListProviders(ctx context.Context) ([]Provider, error) {
	out := []Provider{}
	err := s.db.WithContext(ctx).Model(&ProviderRecord{}).Select(publicProviderColumns).Order("id").Find(&out).Error
	return out, storageError(err)
}

// DeleteProvider 删除厂商配置，仍被模型引用时返回 ErrReferenced。
func (s *Store) DeleteProvider(ctx context.Context, id string) error {
	if !ValidID(id) {
		return ErrInvalidConfig
	}
	return changed(s.db.WithContext(ctx).Omit(clause.Associations).Where("id = ?", id).Delete(&ProviderRecord{}))
}
