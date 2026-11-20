package agent

import (
	"context"
	"encoding/json"
	"slices"

	"gorm.io/gorm"

	domain "github.com/xiaoyu98628/callum/internal/agent"
	"github.com/xiaoyu98628/callum/internal/agent/transcript"
	"github.com/xiaoyu98628/callum/internal/conversation/history"
	"github.com/xiaoyu98628/callum/internal/model"
	"github.com/xiaoyu98628/callum/internal/storage/sqlite/schema"
)

type transcriptRow struct {
	ID, RunID, RunStatus     string
	ParentStepID, ToolCallID *string
	Message                  string
	Sequence                 int64
	Steering                 bool
	Operation                *string
}

// Transcript 恢复当前分支的成功轨迹、已被显式继续的轨迹及失败执行留下的副作用事实。
// 查询前检查累计大小，只加载统一上下文，不加载大型原始模型结果。
func (r *Repository) Transcript(ctx context.Context, id string, after ...int64) (map[string][]model.Message, error) {
	floor := int64(0)
	if len(after) > 0 {
		floor = after[0]
	}
	return r.readTranscript(ctx, id, floor, 8<<20, 256, 0, "")
}

// ContextTranscript 为压缩准备读取有界的未覆盖轨迹，最终模型请求仍遵守原上限。
func (r *Repository) ContextTranscript(ctx context.Context, id string, floor int64, before ...int64) (map[string][]model.Message, error) {
	ceiling := int64(0)
	if len(before) > 0 {
		ceiling = before[0]
	}
	return r.readTranscript(ctx, id, floor, 16<<20, 512, ceiling, "")
}

// ResumeTranscript 读取当前执行已保存的轨迹，包含已生效的引导和已有的压缩检查点。
func (r *Repository) ResumeTranscript(ctx context.Context, conversationID, runID string) ([]model.Message, error) {
	traces, err := r.readTranscript(ctx, conversationID, 0, 16<<20, 512, 0, runID)
	return traces[runID], err
}

func resumeSteps(db *gorm.DB) *gorm.DB {
	return db.Model(&schema.Step{}).Where("type='model_call' AND json_type(input,'$.resume')='object'")
}

func (r *Repository) readTranscript(ctx context.Context, id string, floor int64, maxBytes int64, maxMessages int, before int64, resumeRunID string) (map[string][]model.Message, error) {
	base := func() *gorm.DB {
		q := r.db.WithContext(ctx).Model(&schema.Step{}).
			Joins("JOIN conversation_runs r ON r.id=conversation_run_steps.run_id").
			Joins("JOIN conversation_messages m ON m.run_id=r.id AND m.role='user'").
			Where("conversation_run_steps.conversation_id=? AND m.conversation_id=? AND m.sequence>?", id, id, floor)
		if resumeRunID != "" {
			return q.Where("r.id=? AND r.status='running'", resumeRunID)
		}
		q = q.Where("r.status IN ('completed','failed','cancelled','interrupted')")
		if before > 0 {
			q = q.Where("m.sequence<?", before)
		}
		return q
	}
	// 新步骤缺失事实属于损坏记录，不能沿用旧契约而静默丢失操作。
	var invalid int64
	if err := base().Where(`conversation_run_steps.type='tool_call'
		AND json_extract(conversation_run_steps.input,'$.operation_version')=1 AND (
			COALESCE(json_extract(conversation_run_steps.output,'$.operation.version'),0)<>1
			OR COALESCE(json_extract(conversation_run_steps.output,'$.operation.outcome'),'') NOT IN ('none','not_applied','applied','partially_applied','unknown')
			OR COALESCE(json_type(conversation_run_steps.output,'$.context_message'),'')<>'object'
		)`).Count(&invalid).Error; err != nil {
		return nil, err
	}
	if invalid != 0 {
		return nil, model.ErrInvalidResponse
	}
	visible := func() *gorm.DB {
		if resumeRunID != "" {
			return base()
		}
		effects := r.db.WithContext(ctx).Model(&schema.Step{}).Table("conversation_run_steps AS t").Select("1").
			Where("t.run_id=r.id AND t.type=?", "tool_call").
			Where("json_extract(t.input,'$.operation_version')=1 AND json_extract(t.output,'$.operation.version')=1").
			Where("json_extract(t.output,'$.operation.outcome') IN ?", []string{"applied", "partially_applied", "unknown"})
		continued := r.db.WithContext(ctx).Table("conversation_messages AS u").Select("1").
			Joins("JOIN conversation_run_steps AS next ON next.run_id=u.run_id AND next.sequence=1").
			Where("u.conversation_id=r.conversation_id AND u.role='user' AND u.sequence>m.sequence").
			Where("json_extract(next.input,'$.continued_from_run_id')=r.id")
		resumed := resumeSteps(r.db.WithContext(ctx)).Select("1").Where("run_id=r.id")
		return base().Where("(r.status=? OR EXISTS (?) OR EXISTS (?) OR EXISTS (?))", "completed", effects, continued, resumed)
	}
	var runIDs []string
	if err := visible().Distinct("r.id").Limit(maxMessages+1).Pluck("r.id", &runIDs).Error; err != nil {
		return nil, err
	}
	if len(runIDs) > maxMessages {
		return nil, &model.InputLimitError{Dimension: "历史执行数量", Actual: int64(len(runIDs)), Limit: int64(maxMessages)}
	}
	var boundaries []struct {
		RunID    string
		Sequence int64
	}
	if len(runIDs) > 0 {
		if err := resumeSteps(r.db.WithContext(ctx)).Where("run_id IN ?", runIDs).
			Select("run_id,MAX(sequence) AS sequence").Group("run_id").Scan(&boundaries).Error; err != nil {
			return nil, err
		}
	}
	resumedAt := make(map[string]int64, len(boundaries))
	for _, row := range boundaries {
		resumedAt[row.RunID] = row.Sequence
	}
	checkpoints, err := r.readCheckpoints(ctx, id, runIDs, maxBytes)
	if err != nil {
		return nil, err
	}
	query := func() *gorm.DB {
		q := visible().Where("json_type(conversation_run_steps.output,'$.context_message')='object'")
		// 已验证的固定范围直接参与查询，不对每条消息再关联扫描全部压缩步骤。
		for runID, checkpoint := range checkpoints {
			q = q.Where("(conversation_run_steps.run_id<>? OR conversation_run_steps.sequence>?)", runID, checkpoint.Through)
		}
		return q
	}
	var bounds struct{ Size, Count int64 }
	if err := query().Select("COALESCE(SUM(length(CAST(json_extract(conversation_run_steps.output,'$.context_message') AS BLOB))),0) AS size,COUNT(*) AS count").Scan(&bounds).Error; err != nil {
		return nil, err
	}
	for _, checkpoint := range checkpoints {
		raw, err := json.Marshal(checkpoint.Message())
		if err != nil {
			return nil, err
		}
		bounds.Size += int64(len(raw))
		bounds.Count++
	}
	steering := func() *gorm.DB {
		q := r.db.WithContext(ctx).Table("conversation_followups AS f").
			Joins("JOIN conversation_runs r ON r.id=f.expected_run_id").
			Joins("JOIN conversation_messages m ON m.run_id=r.id AND m.role='user'").
			Joins("JOIN conversation_run_steps s ON s.id=f.applied_step_id AND s.run_id=r.id").
			Where("f.conversation_id=? AND m.conversation_id=? AND m.sequence>? AND f.status='applied'", id, id, floor)
		if resumeRunID != "" {
			return q.Where("r.id=? AND r.status='running'", resumeRunID)
		}
		q = q.Where("r.finished_at IS NOT NULL")
		if before > 0 {
			q = q.Where("m.sequence<?", before)
		}
		return q
	}
	var steeringBounds struct{ Size, Count int64 }
	if err := steering().Select("COALESCE(SUM(length(CAST(f.content AS BLOB))),0) AS size, COUNT(*) AS count").Scan(&steeringBounds).Error; err != nil {
		return nil, err
	}
	bounds.Size += steeringBounds.Size
	bounds.Count += steeringBounds.Count
	if bounds.Size > maxBytes {
		return nil, &model.InputLimitError{Dimension: "历史轨迹字节", Actual: bounds.Size, Limit: maxBytes}
	}
	if bounds.Count > int64(maxMessages) {
		return nil, &model.InputLimitError{Dimension: "历史轨迹消息数量", Actual: bounds.Count, Limit: int64(maxMessages)}
	}
	var rows []transcriptRow
	if err := query().Select("conversation_run_steps.id,conversation_run_steps.run_id,r.status AS run_status,conversation_run_steps.sequence,conversation_run_steps.parent_step_id,conversation_run_steps.tool_call_id,json_extract(conversation_run_steps.output,'$.context_message') AS message,json_extract(conversation_run_steps.output,'$.operation') AS operation").
		Order("r.created_at").Order("r.id").Order("conversation_run_steps.sequence").Scan(&rows).Error; err != nil {
		return nil, err
	}
	var steeringRows []struct {
		ID, RunID, RunStatus, Content string
		Sequence                      int64
	}
	if err := steering().Select("f.id,r.id AS run_id,r.status AS run_status,f.content,s.sequence").
		Order("f.position, f.id").Scan(&steeringRows).Error; err != nil {
		return nil, err
	}
	for _, row := range steeringRows {
		var content []model.Block
		if err := json.Unmarshal([]byte(row.Content), &content); err != nil {
			return nil, err
		}
		raw, err := json.Marshal(model.Message{Role: model.RoleUser, Blocks: history.ModelContent(content)})
		if err != nil {
			return nil, err
		}
		rows = append(rows, transcriptRow{ID: row.ID, RunID: row.RunID, RunStatus: row.RunStatus, Sequence: row.Sequence, Steering: true, Message: string(raw)})
	}
	// 相同序号时，用户引导位于实际使用该消息的模型响应之前。
	slices.SortStableFunc(rows, func(a, b transcriptRow) int {
		if a.RunID != b.RunID {
			if a.RunID < b.RunID {
				return -1
			}
			return 1
		}
		if a.Sequence != b.Sequence {
			if a.Sequence < b.Sequence {
				return -1
			}
			return 1
		}
		if a.Steering == b.Steering {
			return 0
		}
		if a.Steering {
			return -1
		}
		return 1
	})
	out := map[string][]model.Message{}
	for runID, checkpoint := range checkpoints {
		out[runID] = []model.Message{checkpoint.Message()}
	}
	count := len(checkpoints)
	for start := 0; start < len(rows); {
		end := start + 1
		for end < len(rows) && rows[end].RunID == rows[start].RunID {
			end++
		}
		runID := rows[start].RunID
		// 之前停止的模型批次可能只有部分工具结果。即使最终成功，也按中断规则恢复这些批次。
		split := start
		for split < end && rows[split].Sequence < resumedAt[runID] {
			split++
		}
		stopped, err := restoreTranscript(domain.Cancelled, rows[start:split])
		if err != nil {
			return nil, err
		}
		messages, err := restoreTranscript(rows[start].RunStatus, rows[split:end])
		if err != nil {
			return nil, err
		}
		count += len(stopped) + len(messages)
		if count > maxMessages {
			return nil, &model.InputLimitError{Dimension: "历史轨迹消息数量", Actual: int64(count), Limit: int64(maxMessages)}
		}
		out[runID] = append(append(out[runID], stopped...), messages...)
		start = end
	}
	encoded, err := json.Marshal(out)
	if err != nil {
		return nil, err
	}
	if int64(len(encoded)) > maxBytes {
		return nil, &model.InputLimitError{Dimension: "历史轨迹字节", Actual: int64(len(encoded)), Limit: maxBytes}
	}
	return out, nil
}

// restoreTranscript 仅将查询投影转换为业务输入，恢复规则由 transcript 统一实现。
func restoreTranscript(status string, rows []transcriptRow) ([]model.Message, error) {
	entries := make([]transcript.Entry, len(rows))
	for i, row := range rows {
		entries[i] = transcript.Entry{StepID: row.ID, ParentStepID: row.ParentStepID, ToolCallID: row.ToolCallID, Message: json.RawMessage(row.Message)}
		if row.Operation != nil {
			entries[i].Operation = json.RawMessage(*row.Operation)
		}
	}
	return transcript.Restore(status, entries)
}
