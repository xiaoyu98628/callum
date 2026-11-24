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
	ddl = strings.Replace(ddl, legacyReviewerCheck, autoReviewerCheck, 1)
	ddl = strings.Replace(ddl, "conversation_runs", "conversation_runs_v3", 1)
	return rebuildTable(tx, "conversation_runs", "审批人约束升级后外键校验失败",
		ddl,
		"INSERT INTO conversation_runs_v3 SELECT * FROM conversation_runs",
		"DROP TABLE conversation_runs",
		"ALTER TABLE conversation_runs_v3 RENAME TO conversation_runs",
	)
}
