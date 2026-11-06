package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	domain "github.com/xiaoyu98628/callum/internal/agent"
	"github.com/xiaoyu98628/callum/internal/conversation/compaction"
	"github.com/xiaoyu98628/callum/internal/model"
	"github.com/xiaoyu98628/callum/internal/storage/sqlite/schema"
	"github.com/xiaoyu98628/callum/internal/tool"
	"gorm.io/gorm"
)

func validCheckpoint(c domain.ContextCheckpoint) bool {
	if c.Version != 1 || c.ID == "" || c.SourceRunID == "" || c.ConversationID == "" || c.After < 0 || c.Through <= c.After || len(c.Sources) == 0 || len(c.Sources) > 248 || len(c.Stats) > 4096 || !json.Valid(c.Stats) {
		return false
	}
	canonical, err := compaction.Validate(c.Summary)
	if err != nil || canonical != c.Summary {
		return false
	}
	last := c.After
	for _, source := range c.Sources {
		if source.StepID == "" || source.Through <= last || len(source.Fingerprint) != 64 {
			return false
		}
		if _, err := hex.DecodeString(source.Fingerprint); err != nil {
			return false
		}
		last = source.Through
	}
	return last == c.Through
}

func (r *Repository) LatestCheckpoint(ctx context.Context, conversationID, runID string) (*domain.ContextCheckpoint, error) {
	var row struct{ Output string }
	err := r.db.WithContext(ctx).Model(&schema.Step{}).
		Where("conversation_id=? AND type='context_compression' AND status='completed' AND json_extract(output,'$.checkpoint.source_run_id')=?", conversationID, runID).
		Order("CAST(json_extract(output,'$.checkpoint.through_sequence') AS INTEGER) DESC").Limit(1).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return decodeCheckpoint(row.Output, conversationID, runID)
}

func decodeCheckpoint(output, conversationID, runID string) (*domain.ContextCheckpoint, error) {
	var envelope struct {
		Checkpoint domain.ContextCheckpoint `json:"checkpoint"`
	}
	if json.Unmarshal([]byte(output), &envelope) != nil || !validCheckpoint(envelope.Checkpoint) || envelope.Checkpoint.ConversationID != conversationID || (runID != "" && envelope.Checkpoint.SourceRunID != runID) {
		return nil, model.ErrInvalidResponse
	}
	return &envelope.Checkpoint, nil
}

// readCheckpoints 一次查询每个来源执行的最新摘要，避免逐轮恢复产生 N 次 JSON 扫描。
// 数据行逐条读取并检查累计字节，不能先把所有历史版本加载进内存。
func (r *Repository) readCheckpoints(ctx context.Context, conversationID string, runIDs []string, maxBytes int64) (out map[string]*domain.ContextCheckpoint, retErr error) {
	out = map[string]*domain.ContextCheckpoint{}
	if len(runIDs) == 0 {
		return out, nil
	}
	rows, err := r.db.WithContext(ctx).Raw(`SELECT output FROM (
 SELECT output,ROW_NUMBER() OVER (
 PARTITION BY json_extract(output,'$.checkpoint.source_run_id')
 ORDER BY CAST(json_extract(output,'$.checkpoint.through_sequence') AS INTEGER) DESC) AS ordinal
 FROM conversation_run_steps WHERE conversation_id=? AND type='context_compression' AND status='completed'
 AND json_extract(output,'$.checkpoint.source_run_id') IN ?
 ) WHERE ordinal=1`, conversationID, runIDs).Rows()
	if err != nil {
		return nil, err
	}
	defer func() { retErr = errors.Join(retErr, rows.Close()) }()
	size := int64(0)
	for rows.Next() {
		var output string
		if err := rows.Scan(&output); err != nil {
			return nil, err
		}
		checkpoint, err := decodeCheckpoint(output, conversationID, "")
		if err != nil {
			return nil, err
		}
		raw, err := json.Marshal(checkpoint.Message())
		if err != nil {
			return nil, err
		}
		size += int64(len(raw))
		if size > maxBytes {
			return nil, &model.InputLimitError{Dimension: "历史摘要字节", Actual: size, Limit: maxBytes}
		}
		out[checkpoint.SourceRunID] = checkpoint
	}
	return out, rows.Err()
}

type checkpointRow struct {
	ID, RunID, Status        string
	Sequence                 int64
	ParentStepID, ToolCallID *string
	Message                  string
	Operation                *string
	OperationVersion         int
}

const checkpointColumns = "id,run_id,status,sequence,parent_step_id,tool_call_id,json_extract(output,'$.context_message') AS message,json_extract(output,'$.operation') AS operation,COALESCE(json_extract(input,'$.operation_version'),0) AS operation_version"

func (r *Repository) ContextBatch(ctx context.Context, conversationID, runID string, after int64) (*domain.ContextBatch, error) {
	run, err := r.Get(ctx, runID)
	if err != nil {
		return nil, err
	}
	if run.ConversationID != conversationID || after < 0 {
		return nil, domain.ErrInvalid
	}
	if run.Status != domain.Running && run.Status != domain.Completed {
		var effects int64
		if err := r.db.WithContext(ctx).Model(&schema.Step{}).Where("run_id=? AND type='tool_call' AND json_extract(input,'$.operation_version')=1 AND json_extract(output,'$.operation.outcome') IN ('applied','partially_applied','unknown')", runID).Count(&effects).Error; err != nil {
			return nil, err
		}
		if effects == 0 {
			return nil, nil
		}
	}
	var parent checkpointRow
	err = r.db.WithContext(ctx).Model(&schema.Step{}).Select(checkpointColumns).
		Where("run_id=? AND sequence>? AND type='model_call' AND status='completed' AND COALESCE(json_extract(input,'$.purpose'),'') IN ('','answer') AND json_type(output,'$.context_message')='object'", runID, after).
		Order("sequence").Take(&parent).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	q := r.db.WithContext(ctx).Model(&schema.Step{}).Where("run_id=? AND parent_step_id=? AND type IN ('tool_call','user_input')", runID, parent.ID)
	var pending int64
	if err := q.Session(&gorm.Session{}).Where("status IN ('pending','running','awaiting_approval','awaiting_user_input')").Count(&pending).Error; err != nil {
		return nil, err
	}
	if pending > 0 && run.Status == domain.Running {
		return nil, nil
	}
	var bounds struct{ Size, Count int64 }
	if err := r.db.WithContext(ctx).Model(&schema.Step{}).Where("id=? OR (run_id=? AND parent_step_id=? AND type IN ('tool_call','user_input'))", parent.ID, runID, parent.ID).
		Select("COALESCE(SUM(length(CAST(json_extract(output,'$.context_message') AS BLOB))),0) AS size,COUNT(*) AS count").Scan(&bounds).Error; err != nil {
		return nil, err
	}
	if bounds.Size > 7<<20 {
		return nil, &model.InputLimitError{Dimension: "单个工具批次字节", Actual: bounds.Size, Limit: 7 << 20}
	}
	if bounds.Count > 248 {
		return nil, &model.InputLimitError{Dimension: "单个工具批次消息数量", Actual: bounds.Count, Limit: 248}
	}
	var children []checkpointRow
	if err := q.Select(checkpointColumns).Order("sequence").Find(&children).Error; err != nil {
		return nil, err
	}
	rows := append([]checkpointRow{parent}, children...)
	raw, err := json.Marshal(rows)
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(raw)
	status := run.Status
	if status == domain.Running {
		status = domain.Completed
	}
	projected := make([]transcriptRow, 0, len(rows))
	through := parent.Sequence
	for _, row := range rows {
		through = max(through, row.Sequence)
		if row.OperationVersion == tool.OperationVersion {
			var facts tool.OutcomeRecord
			if row.Operation == nil || json.Unmarshal([]byte(*row.Operation), &facts) != nil || !facts.Valid() {
				return nil, model.ErrInvalidResponse
			}
		}
		if row.Message == "" {
			if run.Status == domain.Running {
				return nil, nil
			}
			continue
		}
		projected = append(projected, transcriptRow{ID: row.ID, RunID: row.RunID, RunStatus: status, ParentStepID: row.ParentStepID, ToolCallID: row.ToolCallID, Message: row.Message, Operation: row.Operation})
	}
	if run.Status == domain.Running {
		var message model.Message
		if json.Unmarshal([]byte(parent.Message), &message) != nil {
			return nil, model.ErrInvalidResponse
		}
		found := map[string]bool{}
		for _, row := range children {
			if row.ToolCallID != nil && row.Message != "" {
				found[*row.ToolCallID] = true
			}
		}
		for _, call := range message.ToolCalls {
			if !found[call.ID] {
				return nil, nil
			}
		}
	}
	messages, err := restoreTranscript(projected)
	if err != nil {
		return nil, err
	}
	if len(messages) > 248 {
		return nil, &model.InputLimitError{Dimension: "单个工具批次消息数量", Actual: int64(len(messages)), Limit: 248}
	}
	return &domain.ContextBatch{StepID: parent.ID, Through: through, Fingerprint: hex.EncodeToString(digest[:]), Messages: messages}, nil
}

func (r *Repository) SaveCheckpoint(ctx context.Context, c domain.ContextCheckpoint) error {
	if !validCheckpoint(c) {
		return domain.ErrInvalid
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		store := &Repository{db: tx}
		current, err := store.LatestCheckpoint(ctx, c.ConversationID, c.SourceRunID)
		if err != nil {
			return err
		}
		if (current == nil && (c.BaseID != "" || c.After != 0)) || (current != nil && (current.ID != c.BaseID || current.Through != c.After)) {
			return domain.ErrConflict
		}
		cursor := c.After
		for _, source := range c.Sources {
			batch, err := store.ContextBatch(ctx, c.ConversationID, c.SourceRunID, cursor)
			if err != nil {
				return err
			}
			if batch == nil || batch.BatchSource != source {
				return domain.ErrConflict
			}
			cursor = batch.Through
		}
		var parent schema.Step
		if err := tx.Joins("JOIN conversation_runs r ON r.id=conversation_run_steps.run_id").Where("conversation_run_steps.id=? AND conversation_run_steps.conversation_id=? AND conversation_run_steps.type='context_compression' AND conversation_run_steps.status='running' AND r.status='running'", c.ID, c.ConversationID).Take(&parent).Error; err != nil {
			return err
		}
		output, err := json.Marshal(map[string]any{"compressed": true, "checkpoint": c})
		if err != nil {
			return err
		}
		now := time.Now().UnixMilli()
		result := tx.Model(&schema.Step{}).Where("id=? AND status='running'", c.ID).Updates(map[string]any{"output": string(output), "status": domain.Completed, "finished_at": now, "updated_at": now})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return domain.ErrConflict
		}
		return nil
	})
}
