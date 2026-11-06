package migrations

import (
	"errors"

	"gorm.io/gorm"
)

const directoryGrantVersion = 6

func createDirectoryGrants(tx *gorm.DB, mapping any) error {
	if tx.Migrator().HasTable(mapping) {
		return errors.New("已有目录授权表与迁移冲突")
	}
	return tx.Set("gorm:table_options", "STRICT").Migrator().CreateTable(mapping)
}

func validateDirectoryGrants(tx *gorm.DB, mapping any) error {
	if err := validateCoreTable(tx, mapping); err != nil {
		return err
	}
	if !tx.Migrator().HasIndex(mapping, "ix_directory_grant_conversation") {
		return errors.New("目录授权表缺少会话索引")
	}
	return validateUniqueIndex(tx, "conversation_directory_grants", "uq_directory_grant_approval", []string{"approval_id"})
}
