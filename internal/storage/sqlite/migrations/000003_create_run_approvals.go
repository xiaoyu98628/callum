package migrations

import (
	"errors"
	"slices"

	"gorm.io/gorm"
)

const approvalVersion = 3

// createRunApprovals 创建审批表；审批只等待决定或取消，不保存过期时间。
func createRunApprovals(tx *gorm.DB, step, approval any) error {
	if tx.Migrator().HasTable(approval) {
		return errors.New("已有审批表与待执行迁移冲突")
	}
	// 组合外键要求精确匹配的唯一索引，防止审批绑定其他执行的步骤。
	if !tx.Migrator().HasIndex(step, "uq_step_run") {
		if err := tx.Migrator().CreateIndex(step, "uq_step_run"); err != nil {
			return err
		}
	}
	if err := validateUniqueIndex(tx, "conversation_run_steps", "uq_step_run", []string{"id", "run_id"}); err != nil {
		return err
	}
	return tx.Set("gorm:table_options", "STRICT").Migrator().CreateTable(approval)
}

func validateRunApprovals(tx *gorm.DB, step, approval any) error {
	if !tx.Migrator().HasTable(approval) || !tx.Migrator().HasIndex(step, "uq_step_run") {
		return errors.New("审批结构版本 3 缺少必要结构")
	}
	if err := validateCoreTable(tx, approval); err != nil {
		return err
	}
	// 开发阶段直接维护本迁移定义，结构不匹配时拒绝打开，不自动清理已有库。
	columns, err := tx.Migrator().ColumnTypes(approval)
	if err != nil {
		return err
	}
	statement := &gorm.Statement{DB: tx}
	if err := statement.Parse(approval); err != nil {
		return err
	}
	expected := 0
	for _, field := range statement.Schema.Fields {
		if field.DBName != "" {
			expected++
		}
	}
	if len(columns) != expected {
		return errors.New("审批表字段与当前开发结构不匹配，请使用独立开发数据目录")
	}
	for _, name := range []string{"uq_approval_step", "ix_approval_run", "ix_approval_pending"} {
		if !tx.Migrator().HasIndex(approval, name) {
			return errors.New("审批表缺少必要索引")
		}
	}
	if err := validateUniqueIndex(tx, "conversation_run_steps", "uq_step_run", []string{"id", "run_id"}); err != nil {
		return err
	}
	return validateUniqueIndex(tx, "conversation_run_approvals", "uq_approval_step", []string{"step_id"})
}

// 使用有界元数据查询校验真实唯一索引，避免仅凭索引名称接受损坏的同版本结构。
func validateUniqueIndex(tx *gorm.DB, table, name string, want []string) error {
	var info struct {
		Name    string
		Unique  int
		Partial int
	}
	if err := tx.Raw(`SELECT name,"unique",partial FROM pragma_index_list(?) WHERE name=?`, table, name).Scan(&info).Error; err != nil {
		return err
	}
	var columns []string
	if err := tx.Raw("SELECT name FROM pragma_index_info(?) ORDER BY seqno", name).Scan(&columns).Error; err != nil {
		return err
	}
	if info.Name != name || info.Unique != 1 || info.Partial != 0 || !slices.Equal(columns, want) {
		return errors.New("唯一索引结构不匹配")
	}
	return nil
}
