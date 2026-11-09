package migrations

import (
	"errors"

	"github.com/xiaoyu98628/callum/internal/storage/sqlite/schema"
	"gorm.io/gorm"
)

func approvalTables() []any {
	return []any{&schema.Approval{}, &schema.DirectoryGrant{}}
}

func validateApprovals(tx *gorm.DB) error {
	// 组合外键需要步骤表中精确匹配的唯一索引。
	if err := validateUniqueIndex(tx, "conversation_run_steps", "uq_step_run", []string{"id", "run_id"}); err != nil {
		return err
	}
	columns, err := tx.Migrator().ColumnTypes(&schema.Approval{})
	if err != nil {
		return err
	}
	statement := &gorm.Statement{DB: tx}
	if err := statement.Parse(&schema.Approval{}); err != nil {
		return err
	}
	if len(columns) != len(statement.Schema.DBNames) {
		return errors.New("审批表字段与当前结构不匹配")
	}
	if err := validateUniqueIndex(tx, "conversation_run_approvals", "uq_approval_step", []string{"step_id"}); err != nil {
		return err
	}
	return validateUniqueIndex(tx, "conversation_directory_grants", "uq_directory_grant_approval", []string{"approval_id"})
}
