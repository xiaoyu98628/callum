package catalog

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/xiaoyu98628/callum/internal/model/provider"
	"gorm.io/gorm"
	sqliteengine "modernc.org/sqlite"
	sqlitelib "modernc.org/sqlite/lib"
)

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
