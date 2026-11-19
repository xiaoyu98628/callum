package conversation

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/xiaoyu98628/callum/internal/agent"
	domain "github.com/xiaoyu98628/callum/internal/conversation"
	"github.com/xiaoyu98628/callum/internal/model"
	"github.com/xiaoyu98628/callum/internal/storage/sqlite/schema"
)

var activeFollowUpRuns = []string{agent.Running, agent.AwaitingApproval, agent.AwaitingUserInput}

func followUpView(row schema.FollowUp) domain.FollowUp {
	return domain.FollowUp{ID: row.ID, ConversationID: row.ConversationID, ExpectedRunID: row.ExpectedRunID,
		ModelID: row.ModelID, Status: row.Status, Content: row.Content, Position: row.Position,
		Revision: row.Revision, AppliedStepID: row.AppliedStepID, CreatedAt: row.CreatedAt}
}

type steeringRow struct {
	schema.FollowUp
	AnchorSequence int64
	AfterStep      int64
	AppliedAt      *int64
}

func (row steeringRow) message() domain.Message {
	status := agent.Running
	var updated int64 = row.CreatedAt
	if row.Status == domain.FollowUpApplied {
		status = agent.Completed
		if row.AppliedAt != nil {
			updated = *row.AppliedAt
		}
	}
	return domain.Message{ID: row.ID, ConversationID: row.ConversationID, RunID: row.ExpectedRunID,
		Sequence: row.AnchorSequence, SteeringOrder: row.Position, SteeringAfterStep: new(row.AfterStep),
		Role: "user", Content: row.Content,
		Status: status, CreatedAt: row.CreatedAt, UpdatedAt: updated, CompletedAt: row.AppliedAt}
}

func steeringMessagesQuery(tx *gorm.DB) *gorm.DB {
	// 受理位置优先。旧记录仍可按首次携带引导的请求或实际采用引导的步骤恢复。
	return tx.Table("conversation_followups AS f").
		Select(`f.*, m.sequence AS anchor_sequence, s.finished_at AS applied_at,
			COALESCE(
				(SELECT value FROM json_each(root.input, '$.steering_receipts') WHERE key=f.id),
				(SELECT MIN(p.sequence) - 1 FROM conversation_run_steps p
				 WHERE p.run_id=f.expected_run_id AND p.type='model_call'
				 AND EXISTS (SELECT 1 FROM json_each(p.input, '$.steering_ids') WHERE value=f.id)),
				s.sequence - 1,
				(SELECT COALESCE(MAX(p.sequence), 0) FROM conversation_run_steps p WHERE p.run_id=f.expected_run_id)
			) AS after_step`).
		Joins("JOIN conversation_messages m ON m.run_id=f.expected_run_id AND m.conversation_id=f.conversation_id AND m.role='assistant'").
		Joins("LEFT JOIN conversation_run_steps root ON root.run_id=f.expected_run_id AND root.sequence=1").
		Joins("LEFT JOIN conversation_run_steps s ON s.id=f.applied_step_id AND s.run_id=f.expected_run_id").
		Where("f.status IN ?", []string{domain.FollowUpSteering, domain.FollowUpApplied})
}

// acceptSteeringMessage 在受理事务中把当时的过程位置写入首个步骤。
// 待启动的步骤可能直接采用引导，应排在气泡之后。
func acceptSteeringMessage(tx *gorm.DB, row schema.FollowUp) (domain.Message, error) {
	result := tx.Model(&schema.Step{}).
		Where("run_id=? AND conversation_id=? AND sequence=1", row.ExpectedRunID, row.ConversationID).
		UpdateColumn("input", gorm.Expr(`json_set(input, '$.steering_receipts',
			json_patch(COALESCE(json_extract(input, '$.steering_receipts'), '{}'), json_object(?,
				(SELECT COALESCE(MAX(p.sequence), 0) FROM conversation_run_steps p WHERE p.run_id=? AND p.status<>'pending')
			)))`, row.ID, row.ExpectedRunID))
	if result.Error != nil {
		return domain.Message{}, result.Error
	}
	if result.RowsAffected != 1 {
		return domain.Message{}, domain.ErrFollowUpChanged
	}
	return steeringMessage(tx, row.ConversationID, row.ID)
}

func steeringMessage(tx *gorm.DB, conversationID, followUpID string) (domain.Message, error) {
	var row steeringRow
	err := steeringMessagesQuery(tx).
		Where("f.conversation_id=? AND f.id=?", conversationID, followUpID).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.Message{}, domain.ErrNotFound
	}
	return row.message(), err
}

func followUpRows(q *gorm.DB) ([]domain.FollowUp, error) {
	var rows []schema.FollowUp
	if err := q.Limit(domain.FollowUpLimit + 1).Find(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) > domain.FollowUpLimit {
		return nil, domain.ErrFollowUpLimit
	}
	out := make([]domain.FollowUp, 0, len(rows))
	for _, row := range rows {
		out = append(out, followUpView(row))
	}
	return out, nil
}

func (r *Repository) FollowUps(ctx context.Context, id string) ([]domain.FollowUp, error) {
	return followUpRows(r.db.WithContext(ctx).Model(&schema.FollowUp{}).
		Where("conversation_id=? AND status=?", id, domain.FollowUpQueued).Order("position, id"))
}

func (r *Repository) FollowUp(ctx context.Context, id, itemID string) (domain.FollowUp, error) {
	var row schema.FollowUp
	err := r.db.WithContext(ctx).Where("conversation_id=? AND id=?", id, itemID).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = domain.ErrNotFound
	}
	return followUpView(row), err
}

func checkFollowUpRun(tx *gorm.DB, id, runID string, steer bool) error {
	var count int64
	q := tx.Model(&schema.Run{}).Where("id=? AND conversation_id=? AND status IN ?", runID, id, activeFollowUpRuns).
		Where("EXISTS (SELECT 1 FROM conversations c WHERE c.id=? AND c.archived_at IS NULL AND c.deleted_at IS NULL)", id)
	if steer {
		q = q.Where("status=?", agent.Running).Where("EXISTS (SELECT 1 FROM conversation_run_steps s WHERE s.run_id=conversation_runs.id AND s.sequence=1 AND s.type='model_call')")
	}
	if err := q.Count(&count).Error; err != nil {
		return err
	}
	if count != 1 {
		return domain.ErrFollowUpChanged
	}
	return nil
}

// checkFollowUpContent keeps queue snapshots and per-run steering requests bounded.
func checkFollowUpContent(tx *gorm.DB, row schema.FollowUp, writing bool) error {
	base := tx.Model(&schema.FollowUp{}).Where("conversation_id=? AND id<>?", row.ConversationID, row.ID)
	check := func(q *gorm.DB, bound int64) error {
		var totals struct{ Bytes, Count int64 }
		if err := q.Select("COALESCE(SUM(length(CAST(content AS BLOB))),0) AS bytes, COUNT(*) AS count").Scan(&totals).Error; err != nil {
			return err
		}
		raw, err := json.Marshal(row.Content)
		if err != nil {
			return err
		}
		if totals.Count >= domain.FollowUpLimit || totals.Bytes+int64(len(raw)) > bound {
			return domain.ErrFollowUpLimit
		}
		return nil
	}
	if writing {
		if err := check(base.Session(&gorm.Session{}).Where("status IN ('queued','steering')"), 3<<20); err != nil {
			return err
		}
	}
	if row.Status == domain.FollowUpSteering {
		return check(base.Where("expected_run_id=? AND status IN ('steering','applied')", row.ExpectedRunID), 2<<20)
	}
	return nil
}

func (r *Repository) CreateFollowUp(ctx context.Context, in domain.FollowUp) (domain.FollowUp, error) {
	var receipt *domain.Message
	row := schema.FollowUp{ID: in.ID, ConversationID: in.ConversationID, ExpectedRunID: in.ExpectedRunID,
		ModelID: in.ModelID, Status: in.Status, Content: in.Content, Revision: 1, CreatedAt: time.Now().UnixMilli()}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := checkFollowUpRun(tx, row.ConversationID, row.ExpectedRunID, row.Status == domain.FollowUpSteering); err != nil {
			return err
		}
		if err := checkFollowUpContent(tx, row, true); err != nil {
			return err
		}
		var position int64
		if err := tx.Model(&schema.FollowUp{}).Where("conversation_id=? AND (status IN ('queued','steering') OR (expected_run_id=? AND status='applied'))", row.ConversationID, row.ExpectedRunID).
			Select("COALESCE(MAX(position), 0)").Scan(&position).Error; err != nil {
			return err
		}
		row.Position = position + 1
		if err := tx.Omit("Conversation", "Run").Create(&row).Error; err != nil {
			return err
		}
		if row.Status == domain.FollowUpSteering {
			message, err := acceptSteeringMessage(tx, row)
			if err != nil {
				return err
			}
			receipt = &message
		}
		return nil
	})
	out := followUpView(row)
	out.Receipt = receipt
	return out, err
}

func (r *Repository) ChangeFollowUp(ctx context.Context, in domain.FollowUpChange, content []model.Block) (out domain.FollowUp, err error) {
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row schema.FollowUp
		if err := tx.Where("conversation_id=? AND id=? AND revision=? AND status=?", in.ConversationID, in.ID, in.Revision, domain.FollowUpQueued).Take(&row).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return domain.ErrFollowUpChanged
			}
			return err
		}
		switch in.Action {
		case "steer":
			if err := checkFollowUpRun(tx, row.ConversationID, in.ExpectedRunID, true); err != nil {
				return err
			}
			var tail int64
			if err := tx.Model(&schema.FollowUp{}).Where("conversation_id=? AND (status IN ('queued','steering') OR (expected_run_id=? AND status='applied'))", row.ConversationID, in.ExpectedRunID).
				Select("COALESCE(MAX(position),0)").Scan(&tail).Error; err != nil {
				return err
			}
			row.Status, row.ExpectedRunID, row.Position = domain.FollowUpSteering, in.ExpectedRunID, tail+1
		case "remove":
			row.Status = domain.FollowUpRemoved
			row.Content = []model.Block{}
		case "edit":
			row.Content = content
		case "up", "down":
			var other schema.FollowUp
			q := tx.Where("conversation_id=? AND status=?", row.ConversationID, domain.FollowUpQueued)
			if in.Action == "up" {
				q = q.Where("position<?", row.Position).Order("position DESC")
			} else {
				q = q.Where("position>?", row.Position).Order("position")
			}
			if err := q.Take(&other).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return domain.ErrFollowUpChanged
				}
				return err
			}
			row.Position, other.Position = other.Position, row.Position
			other.Revision++
			if err := tx.Model(&other).Select("Position", "Revision").Updates(&other).Error; err != nil {
				return err
			}
		default:
			return domain.ErrInvalid
		}
		if in.Action == "edit" || in.Action == "steer" {
			if err := checkFollowUpContent(tx, row, in.Action == "edit"); err != nil {
				return err
			}
		}
		row.Revision++
		out = followUpView(row)
		if err := tx.Model(&row).Omit("Conversation", "Run").Select("Status", "Content", "Revision", "Position", "ExpectedRunID").Updates(&row).Error; err != nil {
			return err
		}
		if in.Action == "steer" {
			message, err := acceptSteeringMessage(tx, row)
			if err != nil {
				return err
			}
			out.Receipt = &message
		}
		return nil
	})
	return
}

// PendingSteering 按受理顺序提取还未用于实际模型请求的消息。
func (r *Repository) PendingSteering(ctx context.Context, runID string) ([]domain.FollowUp, error) {
	return followUpRows(r.db.WithContext(ctx).Model(&schema.FollowUp{}).
		Where("expected_run_id=? AND status=?", runID, domain.FollowUpSteering).Order("position, id"))
}

func (r *Repository) ApplySteering(ctx context.Context, runID, stepID string, ids []string) ([]domain.Message, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	messages := make([]domain.Message, 0, len(ids))
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&schema.FollowUp{}).Where("expected_run_id=? AND status=? AND id IN ?", runID, domain.FollowUpSteering, ids).
			Updates(map[string]any{"status": domain.FollowUpApplied, "applied_step_id": stepID, "revision": gorm.Expr("revision+1")})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != int64(len(ids)) {
			return domain.ErrFollowUpChanged
		}
		var rows []steeringRow
		if err := steeringMessagesQuery(tx).
			Where("f.expected_run_id=? AND f.id IN ?", runID, ids).Order("f.position, f.id").Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) != len(ids) {
			return domain.ErrFollowUpChanged
		}
		for _, row := range rows {
			messages = append(messages, row.message())
		}
		return nil
	})
	return messages, err
}

func consumeFollowUp(tx *gorm.DB, in domain.Begin) error {
	request := in.FollowUp
	var head schema.FollowUp
	err := tx.Where("conversation_id=? AND status=?", in.ConversationID, domain.FollowUpQueued).
		Order("position, id").Take(&head).Error
	if errors.Is(err, gorm.ErrRecordNotFound) || (err == nil && (head.ID != request.ID || head.Revision != request.Revision)) {
		return domain.ErrFollowUpChanged
	}
	if err != nil {
		return err
	}
	if request.AfterRunID != "" {
		var previous schema.Run
		if err := tx.Where("conversation_id=?", in.ConversationID).Order("created_at DESC, id DESC").Take(&previous).Error; err != nil {
			return err
		}
		if previous.ID != request.AfterRunID || previous.Status != agent.Completed || head.ExpectedRunID != previous.ID {
			return domain.ErrFollowUpChanged
		}
	}
	if err := tx.Model(&head).Updates(map[string]any{"status": domain.FollowUpSent, "content": "[]", "revision": gorm.Expr("revision+1")}).Error; err != nil {
		return err
	}
	return nil
}

// bindFollowUpQueue links the remaining queue to the new run after it exists.
func bindFollowUpQueue(tx *gorm.DB, in domain.Begin) error {
	if in.FollowUp == nil {
		return nil
	}
	return tx.Model(&schema.FollowUp{}).Where("conversation_id=? AND status=?", in.ConversationID, domain.FollowUpQueued).
		Updates(map[string]any{"expected_run_id": in.RunID, "revision": gorm.Expr("revision+1")}).Error
}
