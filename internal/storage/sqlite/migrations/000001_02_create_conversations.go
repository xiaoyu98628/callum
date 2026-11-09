package migrations

import (
	"errors"
	"strings"

	"github.com/xiaoyu98628/callum/internal/storage/sqlite/schema"
	"gorm.io/gorm"
)

// 会话、执行和步骤按外键依赖排列，消息及上下文版本依赖此前的表。
func conversationTables() []any {
	return []any{&schema.Workspace{}, &schema.Conversation{}, &schema.Run{}, &schema.Message{}, &schema.Step{}, &schema.ContextVersion{}}
}

func validateContextIndex(tx *gorm.DB) error {
	if err := validateUniqueIndex(tx, "conversation_context_versions", "uq_context_version", []string{"conversation_id", "version"}); err != nil {
		return err
	}
	var ddl string
	if err := tx.Raw("SELECT sql FROM sqlite_schema WHERE type='index' AND name='uq_context_active' AND tbl_name='conversation_context_versions'").Scan(&ddl).Error; err != nil {
		return err
	}
	if !strings.Contains(ddl, "UNIQUE INDEX") || !strings.HasSuffix(ddl, "(`conversation_id`) WHERE status = 'active'") {
		return errors.New("有效上下文唯一索引不匹配")
	}
	return nil
}
