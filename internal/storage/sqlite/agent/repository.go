package agent

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"

	domain "github.com/xiaoyu98628/callum/internal/agent"
	"github.com/xiaoyu98628/callum/internal/model"
	"github.com/xiaoyu98628/callum/internal/storage/sqlite/schema"
	"github.com/xiaoyu98628/callum/internal/tool"
)

// Repository 持久化运行状态，数据库由 app 创建及关闭。
type Repository struct{ db *gorm.DB }

var _ domain.Repository = (*Repository)(nil)

func NewRepository(db *gorm.DB) (*Repository, error) {
	if db == nil {
		return nil, domain.ErrInvalid
	}
	return &Repository{db: db}, nil
}

func notFound(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.ErrNotFound
	}
	return err
}

func (r *Repository) Get(ctx context.Context, id string) (domain.Run, error) {
	var row schema.Run
	err := r.db.WithContext(ctx).Where("id = ?", id).Take(&row).Error
	return execution(row), notFound(err)
}

func (r *Repository) List(ctx context.Context, id string, limit, offset int) ([]domain.Run, error) {
	if limit < 1 || limit > 200 || offset < 0 {
		return nil, domain.ErrInvalid
	}
	var rows []schema.Run
	if err := r.db.WithContext(ctx).Where("conversation_id = ?", id).Order("created_at DESC").Order("id DESC").Limit(limit).Offset(offset).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]domain.Run, 0, len(rows))
	for _, row := range rows {
		out = append(out, execution(row))
	}
	return out, nil
}

func (r *Repository) GetStep(ctx context.Context, id string) (domain.Step, error) {
	var row schema.Step
	err := r.db.WithContext(ctx).Where("id = ?", id).Take(&row).Error
	return step(row), notFound(err)
}

func (r *Repository) Steps(ctx context.Context, id string, after int64, limit int) ([]domain.Step, error) {
	if limit < 1 || limit > 200 || after < 0 {
		return nil, domain.ErrInvalid
	}
	if _, err := r.Get(ctx, id); err != nil {
		return nil, err
	}
	var rows []schema.Step
	if err := r.db.WithContext(ctx).Where("run_id = ? AND sequence > ?", id, after).Order("sequence").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]domain.Step, 0, len(rows))
	for _, row := range rows {
		out = append(out, step(row))
	}
	return out, nil
}

func (r *Repository) CreateStep(ctx context.Context, in domain.Step) (out domain.Step, err error) {
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var parent schema.Run
		if err := tx.Where("id = ? AND conversation_id = ? AND status = 'running'", in.RunID, in.ConversationID).Take(&parent).Error; err != nil {
			return notFound(err)
		}
		if in.ParentStepID != nil {
			var existing schema.Step
			if err := tx.Where("id = ? AND run_id = ? AND conversation_id = ?", *in.ParentStepID, in.RunID, in.ConversationID).Take(&existing).Error; err != nil {
				return notFound(err)
			}
		}
		var seq int64
		if err := tx.Model(&schema.Step{}).Where("run_id = ?", in.RunID).Select("COALESCE(MAX(sequence),0)").Scan(&seq).Error; err != nil {
			return err
		}
		in.Sequence = seq + 1
		row := record(in)
		if err := tx.Omit("Run", "Parent").Create(&row).Error; err != nil {
			return err
		}
		out = in
		return nil
	})
	return out, err
}

func (r *Repository) SaveStep(ctx context.Context, in domain.Step) error {
	row := record(in)
	result := r.db.WithContext(ctx).Model(&schema.Step{}).Where("id = ? AND run_id = ? AND status IN ?", in.ID, in.RunID, []string{"pending", "running", "awaiting_approval", "awaiting_user_input"}).Select("Input", "Definition", "Status", "Output", "Usage", "ProviderRequestID", "ErrorKind", "ErrorMessage", "StartedAt", "UpdatedAt", "FinishedAt").Updates(&row)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *Repository) SaveMessage(ctx context.Context, id string, blocks []model.Block, status string) error {
	encoded, err := json.Marshal(blocks)
	if err != nil {
		return err
	}
	now := time.Now().UnixMilli()
	var completed *int64
	if status == domain.Completed {
		completed = &now
	}
	result := r.db.WithContext(ctx).Model(&schema.Message{}).Where("id = ? AND role = 'assistant' AND status IN ?", id, []string{"running", "awaiting_approval", "awaiting_user_input"}).Updates(map[string]any{"content": string(encoded), "status": status, "updated_at": now, "completed_at": completed})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *Repository) Finish(ctx context.Context, in domain.Run) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row := schema.Run{Status: in.Status, ErrorKind: in.ErrorKind, ErrorMessage: in.ErrorMessage, UpdatedAt: in.UpdatedAt, FinishedAt: in.FinishedAt}
		result := tx.Model(&schema.Run{}).Where("id = ? AND status IN ?", in.ID, []string{"running", "awaiting_approval", "awaiting_user_input"}).Select("Status", "ErrorKind", "ErrorMessage", "UpdatedAt", "FinishedAt").Updates(&row)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return domain.ErrNotFound
		}
		if err := cancelApprovals(tx, in.ID, in.UpdatedAt); err != nil {
			return err
		}
		if err := settleToolOutcomes(tx, in.ID); err != nil {
			return err
		}
		status := in.Status
		if status == domain.Completed {
			status = domain.Cancelled
		}
		if err := tx.Model(&schema.Step{}).Where("run_id = ? AND status IN ?", in.ID, []string{"pending", "running", "awaiting_approval", "awaiting_user_input"}).Updates(map[string]any{"status": status, "finished_at": in.FinishedAt, "updated_at": in.UpdatedAt}).Error; err != nil {
			return err
		}
		var completed *int64
		if in.Status == domain.Completed {
			completed = in.FinishedAt
		}
		return tx.Model(&schema.Message{}).Where("run_id = ? AND role = 'assistant' AND status IN ?", in.ID, []string{"running", "awaiting_approval", "awaiting_user_input"}).Updates(map[string]any{"status": in.Status, "completed_at": completed, "updated_at": in.UpdatedAt}).Error
	})
}

// Recover 标记中断、保留内容并确定新工具步骤的保守事实，不重试模型或工具操作。
// finished_at 为恢复时确认终态的时刻，不推测进程实际退出时间。
func (r *Repository) Recover(ctx context.Context) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now().UnixMilli()
		if err := cancelApprovals(tx, "", now); err != nil {
			return err
		}
		if err := settleToolOutcomes(tx, ""); err != nil {
			return err
		}
		active := []string{"pending", "running", "awaiting_approval", "awaiting_user_input"}
		for _, table := range []any{&schema.Step{}, &schema.Run{}} {
			if err := tx.Model(table).Where("status IN ?", active).Updates(map[string]any{"status": domain.Interrupted, "finished_at": now, "updated_at": now, "error_kind": "interrupted", "error_message": "服务退出导致执行中断"}).Error; err != nil {
				return err
			}
		}
		return tx.Model(&schema.Message{}).Where("status IN ?", active).Updates(map[string]any{"status": domain.Interrupted, "completed_at": nil, "updated_at": now}).Error
	})
}

// settleToolOutcomes 与执行终态在同一事务中落盘。只处理明确标记新契约的步骤，
// 不根据旧记录的状态推测副作用；分批读取，避免恢复时无界加载工具输出。
func settleToolOutcomes(tx *gorm.DB, runID string) error {
	var after string
	for {
		q := tx.Model(&schema.Step{}).Where("type='tool_call' AND status IN ? AND id>? AND json_extract(input,'$.operation_version')=?",
			[]string{domain.Pending, domain.Running, domain.AwaitingApproval, "awaiting_user_input"}, after, tool.OperationVersion)
		if runID != "" {
			q = q.Where("run_id=?", runID)
		}
		var rows []schema.Step
		if err := q.Select("id", "input", "output", "definition", "tool_call_id", "started_at").Order("id").Limit(64).Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		for _, row := range rows {
			output, err := recoveredToolOutput(row)
			if err != nil {
				return err
			}
			if err := tx.Model(&schema.Step{}).Where("id=?", row.ID).Update("output", string(output)).Error; err != nil {
				return err
			}
		}
		after = rows[len(rows)-1].ID
	}
}

func recoveredToolOutput(row schema.Step) (json.RawMessage, error) {
	if row.ToolCallID == nil || *row.ToolCallID == "" {
		return nil, model.ErrInvalidResponse
	}
	var input struct {
		RequestHash string `json:"request_hash"`
	}
	if err := json.Unmarshal(row.Input, &input); err != nil {
		return nil, err
	}
	fields := map[string]json.RawMessage{}
	if len(row.Output) > 0 && json.Unmarshal(row.Output, &fields) != nil || fields == nil {
		return nil, model.ErrInvalidResponse
	}
	facts := tool.OutcomeRecord{Version: tool.OperationVersion, Outcome: tool.OutcomeNotApplied, RequestHash: input.RequestHash, Recovered: true}
	if raw, ok := fields["operation"]; ok {
		if json.Unmarshal(raw, &facts) != nil || !facts.Valid() || facts.RequestHash != input.RequestHash {
			return nil, model.ErrInvalidResponse
		}
		facts.Recovered = true
	} else if row.StartedAt != nil {
		var definition tool.Definition
		if json.Unmarshal(row.Definition, &definition) != nil {
			return nil, model.ErrInvalidResponse
		}
		facts.Outcome = tool.UnconfirmedOutcome(definition.Effects)
	}
	var err error
	fields["operation"], err = json.Marshal(facts)
	if err != nil {
		return nil, err
	}
	if _, ok := fields["result"]; !ok {
		fields["result"] = json.RawMessage(`null`)
	}
	if _, ok := fields["error"]; !ok {
		kind, message := "result_unconfirmed", "操作返回结果未完整保存，请核实已记录的操作事实"
		if facts.Outcome == tool.OutcomeUnknown {
			kind, message = "outcome_unknown", "操作可能已经生效，请核实后再重试"
		} else if facts.Outcome == tool.OutcomeNotApplied {
			kind, message = "not_executed", "该操作尚未执行"
		} else if facts.Outcome == tool.OutcomeNone {
			message = "工具返回结果未完整保存，该工具不产生副作用"
		}
		fields["error"], err = json.Marshal(map[string]string{"kind": kind, "message": message})
		if err != nil {
			return nil, err
		}
	}
	delete(fields, "context_message")
	body, err := json.Marshal(fields)
	if err != nil {
		return nil, err
	}
	fields["context_message"], err = json.Marshal(model.Message{Role: model.RoleTool, Content: string(body), ToolCallID: *row.ToolCallID})
	if err != nil {
		return nil, err
	}
	return json.Marshal(fields)
}
