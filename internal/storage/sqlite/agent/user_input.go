package agent

import (
	"context"
	"reflect"
	"time"

	domain "github.com/xiaoyu98628/callum/internal/agent"
	"github.com/xiaoyu98628/callum/internal/storage/sqlite/schema"
	"gorm.io/gorm"
)

var _ domain.UserInputRepository = (*Repository)(nil)

// CreateUserInput 在同一事务中冻结问题并将执行、回答标记为等待输入。
func (r *Repository) CreateUserInput(ctx context.Context, in domain.Step) (out domain.Step, err error) {
	if in.Type != "user_input" || in.Status != domain.AwaitingUserInput {
		return out, domain.ErrInvalid
	}
	if _, err := domain.ParseInputQuestion(in.Input); err != nil {
		return out, err
	}
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var parent schema.Step
		if in.ParentStepID == nil || in.ToolCallID == nil {
			return domain.ErrInvalid
		}
		if err := tx.Where("id=? AND run_id=? AND conversation_id=?", *in.ParentStepID, in.RunID, in.ConversationID).Take(&parent).Error; err != nil {
			return notFound(err)
		}
		result := tx.Model(&schema.Run{}).Where("id=? AND conversation_id=? AND status='running'", in.RunID, in.ConversationID).Updates(map[string]any{"status": domain.AwaitingUserInput, "updated_at": in.UpdatedAt})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return domain.ErrConflict
		}
		result = tx.Model(&schema.Message{}).Where("run_id=? AND role='assistant' AND status='running'", in.RunID).Updates(map[string]any{"status": domain.AwaitingUserInput, "updated_at": in.UpdatedAt})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return domain.ErrConflict
		}
		var sequence int64
		if err := tx.Model(&schema.Step{}).Where("run_id=?", in.RunID).Select("COALESCE(MAX(sequence),0)").Scan(&sequence).Error; err != nil {
			return err
		}
		in.Sequence = sequence + 1
		row := record(in)
		if err := tx.Omit("Run", "Parent").Create(&row).Error; err != nil {
			return err
		}
		out = in
		return nil
	})
	return
}

func (r *Repository) UserInputSteps(ctx context.Context, runID string, pending bool, limit, offset int) ([]domain.Step, error) {
	if limit < 1 || limit > 200 || offset < 0 {
		return nil, domain.ErrInvalid
	}
	if _, err := r.Get(ctx, runID); err != nil {
		return nil, err
	}
	q := r.db.WithContext(ctx).Where("run_id=? AND type='user_input'", runID)
	if pending {
		q = q.Where("status=?", domain.AwaitingUserInput)
	}
	var rows []schema.Step
	if err := q.Order("sequence DESC").Limit(limit).Offset(offset).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]domain.Step, 0, len(rows))
	for _, row := range rows {
		out = append(out, step(row))
	}
	return out, nil
}

// AnswerUserInput 与取消事务竞争同一等待状态。相同回复幂等，不覆盖已有答案。
func (r *Repository) AnswerUserInput(ctx context.Context, id, runID string, answer domain.InputAnswer) (out domain.Step, changed bool, err error) {
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row schema.Step
		if err := tx.Where("id=? AND run_id=? AND type='user_input'", id, runID).Take(&row).Error; err != nil {
			return notFound(err)
		}
		out = step(row)
		question, err := domain.ParseInputQuestion(row.Input)
		if err != nil {
			return err
		}
		if err := domain.ValidateInputAnswer(question, answer); err != nil {
			return err
		}
		if row.Status == domain.Completed {
			value, err := domain.InputFromStep(out)
			if err != nil {
				return err
			}
			if reflect.DeepEqual(value.Answer, &answer) {
				return nil
			}
			return domain.ErrConflict
		}
		if row.Status != domain.AwaitingUserInput {
			return domain.ErrConflict
		}
		now := time.Now().UnixMilli()
		result := tx.Model(&schema.Run{}).Where("id=? AND status=?", runID, domain.AwaitingUserInput).Updates(map[string]any{"status": domain.Running, "updated_at": now})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return domain.ErrConflict
		}
		result = tx.Model(&schema.Message{}).Where("run_id=? AND role='assistant' AND status=?", runID, domain.AwaitingUserInput).Updates(map[string]any{"status": domain.Running, "updated_at": now})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return domain.ErrConflict
		}
		output, err := domain.InputAnswerOutput(out, answer)
		if err != nil {
			return err
		}
		out.Status, out.Output, out.UpdatedAt, out.FinishedAt = domain.Completed, output, now, &now
		result = tx.Model(&schema.Step{}).Where("id=? AND status=?", id, domain.AwaitingUserInput).Updates(map[string]any{"status": domain.Completed, "output": string(output), "updated_at": now, "finished_at": now})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return domain.ErrConflict
		}
		changed = true
		return nil
	})
	return
}

func (r *Repository) CancelUserInput(ctx context.Context, id string) (out domain.Step, err error) {
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row schema.Step
		if err := tx.Where("id=? AND type='user_input'", id).Take(&row).Error; err != nil {
			return notFound(err)
		}
		if row.Status == domain.AwaitingUserInput {
			now := time.Now().UnixMilli()
			row.Status, row.UpdatedAt, row.FinishedAt = domain.Cancelled, now, &now
			if err := tx.Model(&schema.Step{}).Where("id=? AND status=?", id, domain.AwaitingUserInput).Updates(map[string]any{"status": row.Status, "updated_at": now, "finished_at": now}).Error; err != nil {
				return err
			}
		}
		out = step(row)
		return nil
	})
	return
}
