package database

import (
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/xiaoyu98628/callum/internal/model/provider"
	"gorm.io/gorm"
)

// migrateUUIDs 在同一事务中转换整数主键和关联外键，所有配置与时间戳原样保留。
// SQLite 不支持直接修改主键类型，因此暂存旧表，复制成功并验证关联后再移除旧表。
func migrateUUIDs(tx *gorm.DB) error {
	var custom int64
	if err := tx.Raw(`SELECT count(*) FROM sqlite_schema WHERE tbl_name IN ('model_providers','models')
		AND type IN ('index','trigger') AND sql IS NOT NULL AND name <> 'uq_provider_remote_model'`).Scan(&custom).Error; err != nil {
		return err
	}
	if custom != 0 {
		return errors.New("旧表包含自定义索引或触发器，需要确认兼容迁移")
	}
	// 整数主键可由 database/sql 扫描为字符串，用旧值建立映射，不作为新 UUID 使用。
	var providers []struct{ ID string }
	var models []struct{ ID, ProviderID string }
	if err := tx.Table("model_providers").Select("id").Order("id").Find(&providers).Error; err != nil {
		return err
	}
	if err := tx.Table("models").Select("id", "provider_id").Order("id").Find(&models).Error; err != nil {
		return err
	}
	if err := tx.Migrator().RenameTable("models", "callum_models_v1"); err != nil {
		return err
	}
	if err := tx.Migrator().RenameTable("model_providers", "callum_model_providers_v1"); err != nil {
		return err
	}
	// 索引名在整个数据库中唯一，事务内释放旧名称，失败时会随旧表一起恢复。
	if err := tx.Migrator().DropIndex("callum_models_v1", "uq_provider_remote_model"); err != nil {
		return err
	}
	if err := tx.Set("gorm:table_options", "STRICT").Migrator().CreateTable(&provider.ProviderRecord{}, &provider.ModelRecord{}); err != nil {
		return err
	}
	ids := make(map[string]string, len(providers))
	for _, p := range providers {
		id, err := uuid.NewV7()
		if err != nil {
			return err
		}
		ids[p.ID] = id.String()
		// 直接复制列，保留原始 JSON、NULL、凭据及零时间戳，不经过 ORM 的默认值转换。
		if err := tx.Exec(`INSERT INTO model_providers (id,name,protocol,base_url,api_key,created_at,updated_at)
            SELECT ?,name,protocol,base_url,api_key,created_at,updated_at FROM callum_model_providers_v1 WHERE id = ?`, id.String(), p.ID).Error; err != nil {
			return err
		}
	}
	for _, m := range models {
		pid, ok := ids[m.ProviderID]
		if !ok {
			return errors.New("旧模型存在无效厂商关联")
		}
		id, err := uuid.NewV7()
		if err != nil {
			return err
		}
		if err := tx.Exec(`INSERT INTO models (id,provider_id,display_name,remote_model_id,enabled,context_window,max_output_tokens,capabilities,model_type,reasoning_levels,created_at,updated_at)
            SELECT ?,?,display_name,remote_model_id,enabled,context_window,max_output_tokens,capabilities,model_type,reasoning_levels,created_at,updated_at
            FROM callum_models_v1 WHERE id = ?`, id.String(), pid, m.ID).Error; err != nil {
			return err
		}
	}
	// 分别删除子表、父表，保持外键检查开启，不依赖字符串表名的 ORM 关系排序。
	if err := tx.Migrator().DropTable("callum_models_v1"); err != nil {
		return err
	}
	if err := tx.Migrator().DropTable("callum_model_providers_v1"); err != nil {
		return err
	}
	var violations []struct{ Table string }
	if err := tx.Raw("PRAGMA foreign_key_check").Scan(&violations).Error; err != nil {
		return err
	}
	if len(violations) != 0 {
		return fmt.Errorf("迁移后存在 %d 条外键异常", len(violations))
	}
	return nil
}
