package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"time"

	"gorm.io/gorm"

	"github.com/xiaoyu98628/callum/internal/agent"
	"github.com/xiaoyu98628/callum/internal/storage/sqlite/schema"
)

var _ agent.ApprovalRepository = (*Repository)(nil)

func approval(row schema.Approval) agent.Approval {
	return agent.Approval{ID: row.ID, RunID: row.RunID, StepID: row.StepID, Request: row.Request, RequestHash: row.RequestHash, Status: row.Status, DecidedBy: row.DecidedBy, DecisionNote: row.DecisionNote, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, DecidedAt: row.DecidedAt}
}
func (r *Repository) CreateApproval(ctx context.Context, in agent.Approval) (out agent.Approval, err error) {
	sum := sha256.Sum256(in.Request)
	if hex.EncodeToString(sum[:]) != in.RequestHash {
		return out, agent.ErrApprovalInvalid
	}
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var step schema.Step
		if err := tx.Where("id=? AND run_id=? AND type='tool_call' AND status='pending'", in.StepID, in.RunID).Take(&step).Error; err != nil {
			return notFound(err)
		}
		if err := setStatus(tx, in.RunID, agent.AwaitingApproval); err != nil {
			return err
		}
		if err := tx.Model(&schema.Step{}).Where("id=? AND status='pending'", in.StepID).Updates(map[string]any{"status": agent.AwaitingApproval, "updated_at": in.UpdatedAt}).Error; err != nil {
			return err
		}
		row := schema.Approval{ID: in.ID, RunID: in.RunID, StepID: in.StepID, Request: in.Request, RequestHash: in.RequestHash, Status: agent.ApprovalPending, CreatedAt: in.CreatedAt, UpdatedAt: in.UpdatedAt}
		if err := tx.Omit("Run", "Step").Create(&row).Error; err != nil {
			return err
		}
		var err error
		out, err = decorateApproval(tx, row)
		return err
	})
	return
}
func (r *Repository) GetApproval(ctx context.Context, id string) (out agent.Approval, err error) {
	// 决定与目录授权在同一事务提交，读取也使用同一快照，避免返回混合状态。
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row schema.Approval
		if err := tx.Where("id=?", id).Take(&row).Error; err != nil {
			return notFound(err)
		}
		var err error
		out, err = decorateApproval(tx, row)
		return err
	})
	return
}

func (r *Repository) ListApprovals(ctx context.Context, f agent.ApprovalFilter) (out []agent.Approval, err error) {
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		q := tx
		if f.RunID != "" {
			q = q.Where("run_id=?", f.RunID)
		}
		if f.Status != "" {
			q = q.Where("status=?", f.Status)
		}
		var rows []schema.Approval
		if err := q.Order("created_at DESC").Order("id DESC").Limit(f.Limit).Offset(f.Offset).Find(&rows).Error; err != nil {
			return err
		}
		out = make([]agent.Approval, 0, len(rows))
		for _, row := range rows {
			value, err := decorateApproval(tx, row)
			if err != nil {
				return err
			}
			out = append(out, value)
		}
		return nil
	})
	return
}
func (r *Repository) DecideApproval(ctx context.Context, id, status string, by, note *string, scopes ...string) (out agent.Approval, err error) {
	scope := agent.ApprovalOnce
	if len(scopes) == 1 {
		scope = scopes[0]
	}
	if len(scopes) > 1 || (scope != agent.ApprovalOnce && scope != agent.ApprovalConversationDirectory) || (scope != agent.ApprovalOnce && status != agent.ApprovalApproved) || (status != agent.ApprovalApproved && status != agent.ApprovalRejected) {
		return out, agent.ErrApprovalInvalid
	}
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row schema.Approval
		if err := tx.Where("id=?", id).Take(&row).Error; err != nil {
			return notFound(err)
		}
		var err error
		out, err = decorateApproval(tx, row)
		if err != nil {
			return err
		}
		if row.Status == status {
			savedScope := out.DecisionScope
			if savedScope == "" {
				savedScope = agent.ApprovalOnce
			}
			if savedScope != scope {
				return agent.ErrConflict
			}
			return nil
		}
		if row.Status != agent.ApprovalPending {
			return agent.ErrConflict
		}
		var active int64
		if err := tx.Model(&schema.Run{}).Where("id=? AND status=?", row.RunID, agent.AwaitingApproval).Count(&active).Error; err != nil {
			return err
		}
		if active != 1 {
			return agent.ErrConflict
		}
		if scope == agent.ApprovalConversationDirectory {
			if err := saveDirectoryGrant(tx, row, out.DirectoryScope); err != nil {
				return err
			}
		}
		now := time.Now().UnixMilli()
		row.Status, row.DecidedBy, row.DecisionNote, row.DecidedAt, row.UpdatedAt = status, by, note, &now, now
		if err := tx.Model(&schema.Approval{}).Where("id=? AND status='pending'", id).Select("Status", "DecidedBy", "DecisionNote", "DecidedAt", "UpdatedAt").Updates(&row).Error; err != nil {
			return err
		}
		out, err = decorateApproval(tx, row)
		return err
	})
	return
}

// CancelApproval 只取消尚在等待的审批，已经作出的决定保留原值。
func (r *Repository) CancelApproval(ctx context.Context, id string) (out agent.Approval, err error) {
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row schema.Approval
		if err := tx.Where("id=?", id).Take(&row).Error; err != nil {
			return notFound(err)
		}
		if row.Status == agent.ApprovalPending {
			now := time.Now().UnixMilli()
			row.Status, row.UpdatedAt, row.DecidedAt = agent.ApprovalCancelled, now, &now
			if err := tx.Model(&schema.Approval{}).Where("id=? AND status='pending'", id).Select("Status", "UpdatedAt", "DecidedAt").Updates(&row).Error; err != nil {
				return err
			}
		}
		out = approval(row)
		return nil
	})
	return
}
func setStatus(tx *gorm.DB, id, status string) error {
	now := time.Now().UnixMilli()
	result := tx.Model(&schema.Run{}).Where("id=? AND status IN ?", id, []string{agent.Running, agent.AwaitingApproval}).Updates(map[string]any{"status": status, "updated_at": now})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return agent.ErrConflict
	}
	return tx.Model(&schema.Message{}).Where("run_id=? AND role='assistant' AND status IN ?", id, []string{agent.Running, agent.AwaitingApproval}).Updates(map[string]any{"status": status, "updated_at": now}).Error
}
func (r *Repository) Resume(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error { return setStatus(tx, id, agent.Running) })
}
func (r *Repository) StartTool(ctx context.Context, runID, stepID, approvalID, hash string) (out agent.Step, err error) {
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row schema.Step
		if err := tx.Where("id=? AND run_id=? AND type='tool_call' AND status IN ?", stepID, runID, []string{agent.Pending, agent.AwaitingApproval}).Take(&row).Error; err != nil {
			return notFound(err)
		}
		if approvalID != "" {
			var a schema.Approval
			if err := tx.Where("id=? AND run_id=? AND step_id=? AND status='approved' AND request_hash=?", approvalID, runID, stepID, hash).Take(&a).Error; err != nil {
				return agent.ErrConflict
			}
		}
		if row.Status == agent.AwaitingApproval && approvalID == "" {
			return agent.ErrConflict
		}
		if err := checkDirectoryClaim(tx, row); err != nil {
			return err
		}
		if err := setStatus(tx, runID, agent.Running); err != nil {
			return err
		}
		now := time.Now().UnixMilli()
		row.Status, row.StartedAt, row.UpdatedAt = agent.Running, &now, now
		result := tx.Model(&schema.Step{}).Where("id=? AND status IN ?", stepID, []string{agent.Pending, agent.AwaitingApproval}).Select("Status", "StartedAt", "UpdatedAt").Updates(&row)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return agent.ErrConflict
		}
		out = step(row)
		return nil
	})
	return
}

func cancelApprovals(tx *gorm.DB, runID string, now int64) error {
	q := tx.Model(&schema.Approval{}).Where("status='pending'")
	if runID != "" {
		q = q.Where("run_id=?", runID)
	}
	return q.Updates(map[string]any{"status": agent.ApprovalCancelled, "updated_at": now, "decided_at": now}).Error
}
