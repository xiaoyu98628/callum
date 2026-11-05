package migrations

import (
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/xiaoyu98628/callum/internal/settings"
)

const contextVersion = 5

func createContextVersions(tx *gorm.DB, mapping any) error {
	if tx.Migrator().HasTable(mapping) {
		return errors.New("已有上下文版本表与迁移冲突")
	}
	if err := tx.Set("gorm:table_options", "STRICT").Migrator().CreateTable(mapping); err != nil {
		return err
	}
	// 旧快照保持原值；只初始化新的独立设置组，不覆盖 execution 权限设置。
	id, err := uuid.NewV7()
	if err != nil {
		return err
	}
	// 固定版本 5 的原始配置结构；后续 Token 策略必须经版本 6 显式迁移。
	const value = `{"enabled":true,"message_threshold":128,"byte_threshold":4194304,"retain_turns":4}`
	now := time.Now().UnixMilli()
	return tx.Table("app_settings").Create(map[string]any{"id": id.String(), "key": settings.CompactionKey, "value": value, "revision": 1, "created_at": now, "updated_at": now}).Error
}

func validateContextVersions(tx *gorm.DB, mapping any) error {
	if err := validateCoreTable(tx, mapping); err != nil {
		return err
	}
	if err := validateUniqueIndex(tx, "conversation_context_versions", "uq_context_version", []string{"conversation_id", "version"}); err != nil {
		return err
	}
	var ddl string
	if err := tx.Raw("SELECT sql FROM sqlite_schema WHERE type='index' AND name='uq_context_active' AND tbl_name='conversation_context_versions'").Scan(&ddl).Error; err != nil {
		return err
	}
	if !strings.Contains(ddl, "UNIQUE INDEX") || !strings.HasSuffix(ddl, "(`conversation_id`) WHERE status = 'active'") {
		return errors.New("有效上下文唯一索引不匹配")
	}
	return nil
}
