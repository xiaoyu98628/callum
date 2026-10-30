package sqlite

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/xiaoyu98628/callum/internal/model/provider"
	"gorm.io/gorm"
	sqliteengine "modernc.org/sqlite"
	sqlitelib "modernc.org/sqlite/lib"
)

// CatalogRepository 使用 GORM 实现厂商与模型配置存储，数据库所有权仍属于调用方。
type CatalogRepository struct{ db *gorm.DB }

var _ provider.Repository = (*CatalogRepository)(nil)

// NewCatalogRepository 创建配置存储实现，不创建或关闭数据库。
func NewCatalogRepository(db *gorm.DB) (*CatalogRepository, error) {
	if db == nil {
		return nil, errors.New("数据库连接不能为空")
	}
	return &CatalogRepository{db: db}, nil
}

func storageError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, sql.ErrNoRows) {
		return errors.Join(provider.ErrNotFound, err)
	}
	if e, ok := errors.AsType[*sqliteengine.Error](err); ok {
		switch e.Code() {
		case sqlitelib.SQLITE_CONSTRAINT_UNIQUE:
			return errors.Join(provider.ErrConflict, err)
		case sqlitelib.SQLITE_CONSTRAINT_FOREIGNKEY, sqlitelib.SQLITE_CONSTRAINT_TRIGGER:
			// 当前结构没有自定义触发器；SQLite 的 RESTRICT 外键检查可能返回触发器约束码。
			return errors.Join(provider.ErrReferenced, err)
		case sqlitelib.SQLITE_CONSTRAINT_CHECK, sqlitelib.SQLITE_CONSTRAINT_NOTNULL, sqlitelib.SQLITE_CONSTRAINT_DATATYPE:
			return errors.Join(provider.ErrInvalidConfig, err)
		}
	}
	return fmt.Errorf("配置存储操作失败: %w", err)
}

func changed(result *gorm.DB) error {
	if result.Error != nil {
		return storageError(result.Error)
	}
	if result.RowsAffected == 0 {
		return provider.ErrNotFound
	}
	return nil
}
