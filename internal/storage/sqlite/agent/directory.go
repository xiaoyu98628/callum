package agent

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/xiaoyu98628/callum/internal/agent"
	"github.com/xiaoyu98628/callum/internal/storage/sqlite/schema"
	"github.com/xiaoyu98628/callum/internal/tool"
	"github.com/xiaoyu98628/callum/internal/tool/policy"
)

type directoryRequest struct {
	Tool   tool.Definition `json:"tool"`
	Target *tool.Target    `json:"target"`
	Plan   *tool.Plan      `json:"plan"`
}

func (r directoryRequest) targets() []tool.Target {
	var targets []tool.Target
	if r.Target != nil {
		targets = append(targets, *r.Target)
	}
	if r.Plan != nil {
		targets = append(targets, r.Plan.Targets...)
	}
	return targets
}

func directoryGrant(row schema.DirectoryGrant) agent.DirectoryGrant {
	return agent.DirectoryGrant{ID: row.ID, ConversationID: row.ConversationID, ApprovalID: row.ApprovalID, Scope: row.Scope, CreatedAt: row.CreatedAt, RevokedAt: row.RevokedAt}
}

// approvalDirectory 从服务端步骤与固定快照推导选项，不改变旧审批的决定。
func approvalDirectory(tx *gorm.DB, a schema.Approval) (*tool.DirectoryScope, error) {
	var row schema.Step
	if err := tx.Select("input").Where("id=? AND run_id=?", a.StepID, a.RunID).Take(&row).Error; err != nil {
		return nil, err
	}
	var input struct {
		Policy        policy.ApprovalPolicy `json:"approval_policy"`
		Authorization policy.Decision       `json:"authorization"`
	}
	var request directoryRequest
	if json.Unmarshal(row.Input, &input) != nil || json.Unmarshal(a.Request, &request) != nil {
		return nil, agent.ErrApprovalInvalid
	}
	if input.Policy != policy.OnRequest || input.Authorization.Action != policy.Ask || input.Authorization.Reason != "outside_workspace" {
		return nil, nil
	}
	return policy.DirectoryOption(request.targets(), request.Tool.Effects), nil
}

func decorateApproval(tx *gorm.DB, row schema.Approval) (agent.Approval, error) {
	out := approval(row)
	var err error
	out.DirectoryScope, err = approvalDirectory(tx, row)
	if err != nil {
		return out, err
	}
	var grant schema.DirectoryGrant
	err = tx.Where("approval_id=?", row.ID).Take(&grant).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return out, nil
	}
	if err == nil {
		out.DecisionScope = agent.ApprovalConversationDirectory
		out.DirectoryGrant = new(directoryGrant(grant))
	}
	return out, err
}

func saveDirectoryGrant(tx *gorm.DB, row schema.Approval, scope *tool.DirectoryScope) error {
	if scope == nil {
		return agent.ErrApprovalInvalid
	}
	var run schema.Run
	if err := tx.Where("id=?", row.RunID).Take(&run).Error; err != nil {
		return err
	}
	if run.SettingsSnapshot == nil || !run.SettingsSnapshot.Valid() || run.SettingsSnapshot.ApprovalPolicy != policy.OnRequest {
		return agent.ErrApprovalInvalid
	}
	id, err := uuid.NewV7()
	if err != nil {
		return err
	}
	grant := schema.DirectoryGrant{ID: id.String(), ConversationID: run.ConversationID, ApprovalID: row.ID, Scope: *scope, CreatedAt: time.Now().UnixMilli()}
	return tx.Omit("Conversation", "Approval").Create(&grant).Error
}

func (r *Repository) ListDirectoryGrants(ctx context.Context, conversationID string, limit, offset int) ([]agent.DirectoryGrant, error) {
	var rows []schema.DirectoryGrant
	if err := r.db.WithContext(ctx).Where("conversation_id=?", conversationID).Order("created_at DESC").Order("id DESC").Limit(limit).Offset(offset).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]agent.DirectoryGrant, 0, len(rows))
	for _, row := range rows {
		out = append(out, directoryGrant(row))
	}
	return out, nil
}

// RevokeDirectoryGrant 与工具认领共用事务边界，已经开始的操作不被宣称为撤销。
func (r *Repository) RevokeDirectoryGrant(ctx context.Context, conversationID, id string) (out agent.DirectoryGrant, err error) {
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row schema.DirectoryGrant
		if err := tx.Where("id=? AND conversation_id=?", id, conversationID).Take(&row).Error; err != nil {
			return notFound(err)
		}
		if row.RevokedAt == nil {
			row.RevokedAt = new(time.Now().UnixMilli())
			if err := tx.Model(&schema.DirectoryGrant{}).Where("id=? AND revoked_at IS NULL", id).Update("revoked_at", row.RevokedAt).Error; err != nil {
				return err
			}
		}
		out = directoryGrant(row)
		return nil
	})
	return
}

func checkDirectoryClaim(tx *gorm.DB, row schema.Step) error {
	var input struct {
		IDs           []string              `json:"directory_grant_ids"`
		Policy        policy.ApprovalPolicy `json:"approval_policy"`
		Authorization policy.Decision       `json:"authorization"`
		Request       directoryRequest      `json:"request"`
	}
	if json.Unmarshal(row.Input, &input) != nil {
		return tool.ErrDenied
	}
	if len(input.IDs) == 0 {
		if input.Authorization.Reason == "conversation_directory_grant" {
			return tool.ErrDenied
		}
		return nil
	}
	if input.Policy != policy.OnRequest || input.Authorization.Action != policy.Allow || input.Authorization.Reason != "conversation_directory_grant" {
		return tool.ErrDenied
	}
	var grants []schema.DirectoryGrant
	if err := tx.Where("id IN ? AND conversation_id=? AND revoked_at IS NULL", input.IDs, row.ConversationID).Find(&grants).Error; err != nil {
		return err
	}
	scopes := make([]tool.DirectoryScope, 0, len(grants))
	for _, grant := range grants {
		scopes = append(scopes, grant.Scope)
	}
	if !policy.DirectoriesCover(scopes, input.Request.targets(), input.Request.Tool.Effects) {
		return tool.ErrDenied
	}
	return nil
}
