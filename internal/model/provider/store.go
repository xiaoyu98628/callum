package provider

import (
	"database/sql"
	"errors"
	"fmt"

	"gorm.io/gorm"
	"modernc.org/sqlite"
	sqlitelib "modernc.org/sqlite/lib"
)

var (
	ErrNotFound      = errors.New("配置不存在")
	ErrConflict      = errors.New("模型标识重复")
	ErrReferenced    = errors.New("关联配置不存在或仍被引用")
	ErrInvalidConfig = errors.New("配置无效")
	ErrDisabled      = errors.New("模型已禁用")
)

// Store 使用调用方持有的 GORM 实例，不负责关闭数据库。
type Store struct{ db *gorm.DB }

// NewStore 创建配置访问对象，数据库所有权仍属于调用方。
func NewStore(db *gorm.DB) (*Store, error) {
	if db == nil {
		return nil, errors.New("数据库连接不能为空")
	}
	return &Store{db: db}, nil
}

func storageError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, sql.ErrNoRows) {
		return errors.Join(ErrNotFound, err)
	}
	if e, ok := errors.AsType[*sqlite.Error](err); ok {
		switch e.Code() {
		case sqlitelib.SQLITE_CONSTRAINT_UNIQUE:
			return errors.Join(ErrConflict, err)
		case sqlitelib.SQLITE_CONSTRAINT_FOREIGNKEY, sqlitelib.SQLITE_CONSTRAINT_TRIGGER:
			// 当前结构没有自定义触发器；SQLite 的 RESTRICT 外键检查可能返回触发器约束码。
			return errors.Join(ErrReferenced, err)
		case sqlitelib.SQLITE_CONSTRAINT_CHECK, sqlitelib.SQLITE_CONSTRAINT_NOTNULL, sqlitelib.SQLITE_CONSTRAINT_DATATYPE:
			return errors.Join(ErrInvalidConfig, err)
		}
	}
	return fmt.Errorf("配置存储操作失败: %w", err)
}

func changed(result *gorm.DB) error {
	if result.Error != nil {
		return storageError(result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}
