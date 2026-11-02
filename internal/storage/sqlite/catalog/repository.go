package catalog

import (
	"errors"

	"github.com/xiaoyu98628/callum/internal/model/provider"
	"gorm.io/gorm"
)

// Repository 使用 GORM 实现厂商与模型配置存储，数据库所有权仍属于调用方。
type Repository struct{ db *gorm.DB }

var _ provider.Repository = (*Repository)(nil)

// NewRepository 创建配置存储实现，不创建或关闭数据库。
func NewRepository(db *gorm.DB) (*Repository, error) {
	if db == nil {
		return nil, errors.New("数据库连接不能为空")
	}
	return &Repository{db: db}, nil
}
