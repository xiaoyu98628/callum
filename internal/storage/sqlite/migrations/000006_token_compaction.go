package migrations

import (
	"encoding/json"
	"errors"
	"math"
	"time"

	"gorm.io/gorm"

	"github.com/xiaoyu98628/callum/internal/settings"
)

const tokenCompactionVersion = 6

func migrateTokenCompaction(tx *gorm.DB) error {
	var row struct {
		ID       string
		Value    string
		Revision int64
	}
	if err := tx.Table("app_settings").Where("key=?", settings.CompactionKey).Take(&row).Error; err != nil {
		return err
	}
	value, changed, err := settings.UpgradeCompaction(json.RawMessage(row.Value))
	if err != nil {
		return err
	}
	if !changed {
		return nil
	}
	if row.Revision < 1 || row.Revision == math.MaxInt64 {
		return errors.New("压缩设置版本无法递增")
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	result := tx.Table("app_settings").Where("id=? AND revision=?", row.ID, row.Revision).Updates(map[string]any{"value": string(encoded), "revision": row.Revision + 1, "updated_at": time.Now().UnixMilli()})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errors.New("压缩设置迁移未更新唯一记录")
	}
	return nil
}

func validateTokenCompaction(tx *gorm.DB) error {
	var row struct {
		Value    string
		Revision int64
	}
	if err := tx.Table("app_settings").Where("key=?", settings.CompactionKey).Take(&row).Error; err != nil {
		return err
	}
	_, err := settings.DecodeCompactionSnapshot(settings.Record{Key: settings.CompactionKey, Value: json.RawMessage(row.Value), Revision: row.Revision})
	return err
}
