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
	now := time.Now().UnixMilli()
	for _, group := range groups {
		id, err := uuid.NewV7()
		if err != nil {
			return err
		}
		value, err := json.Marshal(group.value)
		if err != nil {
			return err
		}
		record := schema.Settings{ID: id.String(), Key: group.key, Value: value, Revision: 1, CreatedAt: now, UpdatedAt: now}
		if err := tx.Create(&record).Error; err != nil {
			return err
		}
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
