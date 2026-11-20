package agent

import (
	"gorm.io/gorm"

	domain "github.com/xiaoyu98628/callum/internal/agent"
	"github.com/xiaoyu98628/callum/internal/storage/sqlite/schema"
)

func step(row schema.Step) domain.Step {
	return domain.Step{ID: row.ID, ConversationID: row.ConversationID, RunID: row.RunID, ParentStepID: row.ParentStepID, Sequence: row.Sequence, Type: row.Type, Name: row.Name, Definition: row.Definition, Status: row.Status, Input: row.Input, Output: row.Output, Usage: row.Usage, Model: row.Model, ProviderRequestID: row.ProviderRequestID, ToolCallID: row.ToolCallID, ErrorKind: row.ErrorKind, ErrorMessage: row.ErrorMessage, CreatedAt: row.CreatedAt, StartedAt: row.StartedAt, UpdatedAt: row.UpdatedAt, FinishedAt: row.FinishedAt}
}
func record(in domain.Step) schema.Step {
	return schema.Step{ID: in.ID, ConversationID: in.ConversationID, RunID: in.RunID, ParentStepID: in.ParentStepID, Sequence: in.Sequence, Type: in.Type, Name: in.Name, Definition: in.Definition, Status: in.Status, Input: in.Input, Output: in.Output, Usage: in.Usage, Model: in.Model, ProviderRequestID: in.ProviderRequestID, ToolCallID: in.ToolCallID, ErrorKind: in.ErrorKind, ErrorMessage: in.ErrorMessage, CreatedAt: in.CreatedAt, StartedAt: in.StartedAt, UpdatedAt: in.UpdatedAt, FinishedAt: in.FinishedAt}
}

// RunRow 包含普通执行行与由步骤推导的恢复时间，不参与数据库结构检查。
type RunRow struct {
	schema.Run `gorm:"embedded"`
	ResumedAt  *int64
}

// Run 从 SQLite 查询投影转换执行状态，供历史与独立执行查询共享。
func Run(row RunRow) domain.Run {
	return domain.Run{ID: row.ID, ConversationID: row.ConversationID, ReplacesRunID: row.ReplacesRunID, Model: row.Model, Stream: row.Stream, SettingsSnapshot: row.SettingsSnapshot, Status: row.Status, ErrorKind: row.ErrorKind, ErrorMessage: row.ErrorMessage, CreatedAt: row.CreatedAt, ResumedAt: row.ResumedAt, UpdatedAt: row.UpdatedAt, FinishedAt: row.FinishedAt}
}

// WithResumeTime 投影最新恢复步骤的受理时间，保留执行原创建时间用于排序。
func WithResumeTime(db *gorm.DB) *gorm.DB {
	latest := resumeSteps(db.Session(&gorm.Session{NewDB: true})).Select("created_at").
		Where("run_id=conversation_runs.id").Order("sequence DESC").Limit(1)
	return db.Model(&schema.Run{}).Select("conversation_runs.*, (?) AS resumed_at", latest)
}
