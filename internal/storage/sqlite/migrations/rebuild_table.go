package migrations

import (
	"errors"
	"slices"

	"gorm.io/gorm"
)

// rebuildTable 在调用方的事务中重建表、恢复索引及触发器，再校验外键。
// 调用方在固定连接上临时关闭外键，负责回滚失败的迁移。
func rebuildTable(tx *gorm.DB, table, foreignKeyFailure string, queries ...string) error {
	var statements []string
	if err := tx.Raw("SELECT sql FROM sqlite_schema WHERE tbl_name=? AND type IN ('index','trigger') AND sql IS NOT NULL ORDER BY type,name", table).Scan(&statements).Error; err != nil {
		return err
	}
	for _, statement := range slices.Concat(queries, statements) {
		if err := tx.Exec(statement).Error; err != nil {
			return err
		}
	}
	var invalid int
	if err := tx.Raw("SELECT count(*) FROM (SELECT 1 FROM pragma_foreign_key_check LIMIT 1)").Scan(&invalid).Error; err != nil {
		return err
	}
	if invalid != 0 {
		return errors.New(foreignKeyFailure)
	}
	return nil
}
