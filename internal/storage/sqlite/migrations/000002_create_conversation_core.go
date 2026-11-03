package migrations

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"gorm.io/gorm"
)

const conversationVersion = 2

// createConversationCore 在既有事务中创建工作区和普通对话所需五张表。
func createConversationCore(tx *gorm.DB, models []any) error {
	for _, table := range models {
		if tx.Migrator().HasTable(table) {
			return errors.New("已有对话表与待执行迁移冲突")
		}
	}
	if err := tx.Set("gorm:table_options", "STRICT").Migrator().CreateTable(models...); err != nil {
		return fmt.Errorf("创建会话结构: %w", err)
	}
	return nil
}

// validateConversationCore 防止旧版本号碰撞或缺表被当作可用结构。
func validateConversationCore(tx *gorm.DB, models []any) error {
	for _, table := range models {
		if !tx.Migrator().HasTable(table) {
			return errors.New("会话结构版本 2 缺少必要表")
		}
		if err := validateCoreTable(tx, table, "settings_snapshot"); err != nil {
			return fmt.Errorf("会话结构版本 2 不匹配: %w", err)
		}
	}
	return nil
}

func validateCoreTable(tx *gorm.DB, table any, skipColumns ...string) error {
	statement := &gorm.Statement{DB: tx}
	if err := statement.Parse(table); err != nil {
		return err
	}
	var ddl string
	if err := tx.Raw("SELECT sql FROM sqlite_schema WHERE type='table' AND name=?", statement.Schema.Table).Scan(&ddl).Error; err != nil {
		return err
	}
	if !strings.HasSuffix(strings.ToUpper(strings.TrimSpace(ddl)), "STRICT") {
		return fmt.Errorf("%s 不是 STRICT 表", statement.Schema.Table)
	}
	columns, err := tx.Migrator().ColumnTypes(table)
	if err != nil {
		return err
	}
	actual := make(map[string]gorm.ColumnType, len(columns))
	primary := 0
	for _, column := range columns {
		actual[column.Name()] = column
		if value, known := column.PrimaryKey(); known && value {
			primary++
		}
	}
	if primary != 1 {
		return errors.New("会话表必须具有单列主键")
	}
	for _, field := range statement.Schema.Fields {
		if field.DBName == "" {
			continue
		}
		// 后续版本新增的列由对应迁移校验，不能阻止旧版本进入升级事务。
		if slices.Contains(skipColumns, field.DBName) {
			continue
		}
		column, exists := actual[field.DBName]
		if !exists {
			return fmt.Errorf("%s 缺少字段 %s", statement.Schema.Table, field.DBName)
		}
		want := field.TagSettings["TYPE"]
		if want == "" {
			want = "integer"
		}
		if !strings.EqualFold(column.DatabaseTypeName(), want) {
			return fmt.Errorf("%s.%s 类型不匹配", statement.Schema.Table, field.DBName)
		}
		if nullable, known := column.Nullable(); known && field.NotNull && nullable {
			return fmt.Errorf("%s.%s 缺少非空约束", statement.Schema.Table, field.DBName)
		}
		if field.PrimaryKey {
			if primary, known := column.PrimaryKey(); !known || !primary {
				return errors.New("会话表主键不匹配")
			}
		}
		if check := field.TagSettings["CHECK"]; check != "" {
			name, _, _ := strings.Cut(check, ",")
			if !tx.Migrator().HasConstraint(table, name) {
				return fmt.Errorf("缺少检查约束 %s", name)
			}
		}
	}
	indexes := map[string][]string{"workspaces": {"uq_workspace_path"}, "conversations": {"ix_conversation_workspace", "ix_conversation_listing"}, "conversation_runs": {"uq_run_identity", "uq_active_conversation", "ix_run_history"}, "conversation_messages": {"uq_message_sequence", "uq_message_run_role"}, "conversation_run_steps": {"uq_step_identity", "uq_step_sequence"}}
	for _, name := range indexes[statement.Schema.Table] {
		if !tx.Migrator().HasIndex(table, name) {
			return fmt.Errorf("缺少索引 %s", name)
		}
	}
	for name := range statement.Schema.Relationships.Relations {
		if !tx.Migrator().HasConstraint(table, name) {
			return fmt.Errorf("缺少关联约束 %s", name)
		}
	}
	return nil
}
