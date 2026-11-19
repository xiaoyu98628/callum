package migrations

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/xiaoyu98628/callum/internal/settings"
	"github.com/xiaoyu98628/callum/internal/storage/sqlite/schema"
	"gorm.io/gorm"
)

func settingsTables() []any { return []any{&schema.Settings{}} }

func initializeSettings(tx *gorm.DB) error {
	groups := []struct {
		key   string
		value any
	}{
		{settings.ExecutionKey, settings.DefaultExecution()},
		{settings.CompactionKey, settings.DefaultCompaction()},
	}
	for _, group := range groups {
		if err := createSetting(tx, group.key, group.value); err != nil {
			return err
		}
	}
	return nil
}

func createSetting(tx *gorm.DB, key string, value any) error {
	id, err := uuid.NewV7()
	if err != nil {
		return err
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	now := time.Now().UnixMilli()
	record := schema.Settings{ID: id.String(), Key: key, Value: raw, Revision: 1, CreatedAt: now, UpdatedAt: now}
	return tx.Create(&record).Error
}

// ensureGeneralSettings 在既有结构事务中补齐常规组，保留已保存的值和版本。
func ensureGeneralSettings(tx *gorm.DB) error {
	var row schema.Settings
	err := tx.Where("key=?", settings.GeneralKey).Limit(1).Find(&row).Error
	if err != nil {
		return err
	}
	if row.ID == "" {
		return createSetting(tx, settings.GeneralKey, settings.DefaultGeneral())
	}
	_, err = settings.DecodeGeneralRecord(settings.Record{Key: row.Key, Value: row.Value, Revision: row.Revision})
	if err != nil {
		return fmt.Errorf("校验常规设置 %q: %w", row.Key, err)
	}
	return nil
}

func validateSettings(tx *gorm.DB) error {
	if err := validateUniqueIndex(tx, "app_settings", "uq_settings_key", []string{"key"}); err != nil {
		return err
	}
	var compaction schema.Settings
	if err := tx.Where("key=?", settings.CompactionKey).Take(&compaction).Error; err != nil {
		return fmt.Errorf("读取上下文压缩设置: %w", err)
	}
	_, err := settings.DecodeCompactionSnapshot(settings.Record{Key: compaction.Key, Value: compaction.Value, Revision: compaction.Revision})
	return err
}
