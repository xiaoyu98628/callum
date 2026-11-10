package migrations

import (
	"errors"
	"strings"

	"gorm.io/gorm"
)

const legacyReviewerCheck = "json_extract(settings_snapshot,'$.approvals_reviewer') IS 'user'"
const autoReviewerCheck = "COALESCE(json_extract(settings_snapshot,'$.approvals_reviewer') IN ('user','auto_review'),0)"

func runTableDDL(tx *gorm.DB) (string, error) {
	var ddl string
	err := tx.Raw("SELECT sql FROM sqlite_schema WHERE type='table' AND name='conversation_runs'").Scan(&ddl).Error
	return ddl, err
}

func validateAutoReviewer(tx *gorm.DB) error {
	ddl, err := runTableDDL(tx)
	if err != nil {
		return err
	}
	if strings.Count(ddl, autoReviewerCheck) != 1 {
		return errors.New("执行快照审批人约束与当前版本不匹配")
	}
	return nil
}

// allowAutoReviewer 只替换审批人的 CHECK 表达式。调用方固定连接并关闭外键，事务失败时恢复整张表。
func allowAutoReviewer(tx *gorm.DB) error {
	ddl, err := runTableDDL(tx)
	if err != nil {
		return err
	}
	if strings.Contains(ddl, autoReviewerCheck) {
		return nil
	}
	if strings.Count(ddl, legacyReviewerCheck) != 1 {
		return errors.New("未找到可升级的执行快照审批人约束")
	}
	var statements []string
	if err := tx.Raw("SELECT sql FROM sqlite_schema WHERE tbl_name='conversation_runs' AND type IN ('index','trigger') AND sql IS NOT NULL ORDER BY type,name").Scan(&statements).Error; err != nil {
		return err
	}
	ddl = strings.Replace(ddl, legacyReviewerCheck, autoReviewerCheck, 1)
	ddl = strings.Replace(ddl, "conversation_runs", "conversation_runs_v3", 1)
	for _, query := range []string{ddl, "INSERT INTO conversation_runs_v3 SELECT * FROM conversation_runs", "DROP TABLE conversation_runs", "ALTER TABLE conversation_runs_v3 RENAME TO conversation_runs"} {
		if err := tx.Exec(query).Error; err != nil {
			return err
		}
	}
	for _, statement := range statements {
		if err := tx.Exec(statement).Error; err != nil {
			return err
		}
	}
	var invalid int
	if err := tx.Raw("SELECT count(*) FROM (SELECT 1 FROM pragma_foreign_key_check LIMIT 1)").Scan(&invalid).Error; err != nil {
		return err
	}
	if invalid != 0 {
		return errors.New("审批人约束升级后外键校验失败")
	}
	return nil
}
