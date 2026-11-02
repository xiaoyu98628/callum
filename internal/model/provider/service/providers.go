package service

import (
	"context"

	"github.com/xiaoyu98628/callum/internal/model/provider"
)

// CreateProvider 校验连接配置并分配 UUIDv7，未提供密钥时保存空字符串。
func (s *Service) CreateProvider(ctx context.Context, in provider.ProviderInput) (string, error) {
	if err := validateProvider(in); err != nil {
		return "", err
	}
	id, err := newID()
	if err != nil {
		return "", err
	}
	if err := s.repository.CreateProvider(ctx, id, in); err != nil {
		return "", err
	}
	return id, nil
}

// UpdateProvider 替换完整连接配置；APIKey 为 nil 保留原值，指向空字符串时清空。
func (s *Service) UpdateProvider(ctx context.Context, id string, in provider.ProviderInput) error {
	if !provider.ValidID(id) {
		return provider.ErrInvalidConfig
	}
	if err := validateProvider(in); err != nil {
		return err
	}
	return s.repository.UpdateProvider(ctx, id, in)
}

// GetProvider 返回不含明文密钥的厂商配置。
func (s *Service) GetProvider(ctx context.Context, id string) (provider.Provider, error) {
	if !provider.ValidID(id) {
		return provider.Provider{}, provider.ErrInvalidConfig
	}
	return s.repository.GetProvider(ctx, id)
}

// ListProviders 返回不含明文密钥的厂商列表，按本地标识排序。
func (s *Service) ListProviders(ctx context.Context) ([]provider.Provider, error) {
	return s.repository.ListProviders(ctx)
}

// DeleteProvider 删除未被模型引用的厂商，关联约束由存储原子检查。
func (s *Service) DeleteProvider(ctx context.Context, id string) error {
	if !provider.ValidID(id) {
		return provider.ErrInvalidConfig
	}
	return s.repository.DeleteProvider(ctx, id)
}
