package conversation

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"gorm.io/gorm"

	"github.com/xiaoyu98628/callum/internal/agent"
	domain "github.com/xiaoyu98628/callum/internal/conversation"
	"github.com/xiaoyu98628/callum/internal/conversation/compaction"
	"github.com/xiaoyu98628/callum/internal/model"
	agentstore "github.com/xiaoyu98628/callum/internal/storage/sqlite/agent"
	"github.com/xiaoyu98628/callum/internal/storage/sqlite/schema"
)

func contextValue(v schema.ContextVersion) domain.ContextVersion {
	return domain.ContextVersion{ID: v.ID, ConversationID: v.ConversationID, Version: v.Version,
		BaseVersionID: v.BaseVersionID, GeneratedByStepID: v.GeneratedByStepID, ThroughSequence: v.ThroughSequence,
		Summary: v.Summary, Sources: v.Sources, Stats: v.Stats, Status: v.Status,
		CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt, ActivatedAt: v.ActivatedAt, InvalidatedAt: v.InvalidatedAt}
}

func activeContext(tx *gorm.DB, id string) (*domain.ContextVersion, error) {
	var row schema.ContextVersion
	err := tx.Where("conversation_id=? AND status='active'", id).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	value := contextValue(row)
	if value.Summary == "" || value.ThroughSequence < 1 || len(value.Sources) == 0 || value.ActivatedAt == nil {
		return nil, model.ErrInvalidResponse
	}
	canonical, err := compaction.Validate(value.Summary)
	if err != nil || canonical != value.Summary {
		return nil, model.ErrInvalidResponse
	}
	var last int64
	for _, source := range value.Sources {
		if source.Sequence <= last || source.MessageID == "" || source.RunID == "" || len(source.Fingerprint) != 64 {
			return nil, model.ErrInvalidResponse
		}
		if _, err := hex.DecodeString(source.Fingerprint); err != nil {
			return nil, model.ErrInvalidResponse
		}
		last = source.Sequence
	}
	if last != value.ThroughSequence {
		return nil, model.ErrInvalidResponse
	}
	return &value, nil
}

func (r *Repository) ActiveContext(ctx context.Context, id string) (*domain.ContextVersion, error) {
	return activeContext(r.db.WithContext(ctx), id)
}

func (r *Repository) ContextVersion(ctx context.Context, conversationID, id string) (domain.ContextVersion, error) {
	var row schema.ContextVersion
	err := r.db.WithContext(ctx).Where("id=? AND conversation_id=?", id, conversationID).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.ContextVersion{}, domain.ErrNotFound
	}
	return contextValue(row), err
}

func (r *Repository) ContextVersions(ctx context.Context, id string, limit, offset int) ([]domain.ContextVersion, error) {
	if _, err := r.Get(ctx, id); err != nil {
		return nil, err
	}
	var rows []schema.ContextVersion
	// 列表只读元数据，避免一页携带多份摘要和来源清单。
	if err := r.db.WithContext(ctx).Omit("Summary", "Sources", "Stats").Where("conversation_id=?", id).
		Order("version DESC").Limit(limit).Offset(offset).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]domain.ContextVersion, len(rows))
	for i, row := range rows {
		out[i] = contextValue(row)
	}
	return out, nil
}

func (r *Repository) CreateContext(ctx context.Context, in domain.ContextVersion) (out domain.ContextVersion, err error) {
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := checkContextSource(tx, in); err != nil {
			return err
		}
		var version int64
		if err := tx.Model(&schema.ContextVersion{}).Where("conversation_id=?", in.ConversationID).Select("COALESCE(MAX(version),0)").Scan(&version).Error; err != nil {
			return err
		}
		now := time.Now().UnixMilli()
		row := schema.ContextVersion{ID: in.ID, ConversationID: in.ConversationID, Version: version + 1,
			BaseVersionID: in.BaseVersionID, GeneratedByStepID: in.GeneratedByStepID, ThroughSequence: in.ThroughSequence,
			Sources: in.Sources, Summary: "", Status: "building", CreatedAt: now, UpdatedAt: now}
		if err := tx.Omit("Conversation", "GeneratedByStep", "Base").Create(&row).Error; err != nil {
			return err
		}
		out = contextValue(row)
		return nil
	})
	return out, err
}

// checkContextSource 在激活前重查固定来源及所属活动执行；网络请求不持有该事务。
func checkContextSource(tx *gorm.DB, in domain.ContextVersion) error {
	if len(in.Sources) == 0 || len(in.Sources) > 256 || in.ThroughSequence < 1 {
		return domain.ErrInvalid
	}
	var step schema.Step
	if err := tx.Joins("JOIN conversation_runs r ON r.id=conversation_run_steps.run_id").
		Where("conversation_run_steps.id=? AND conversation_run_steps.conversation_id=? AND conversation_run_steps.type='context_compression' AND r.status='running'", in.GeneratedByStepID, in.ConversationID).Take(&step).Error; err != nil {
		return err
	}
	current, err := activeContext(tx, in.ConversationID)
	if err != nil {
		return err
	}
	floor := int64(0)
	if current != nil {
		floor = current.ThroughSequence
	}
	if (current == nil) != (in.BaseVersionID == nil) || (current != nil && current.ID != *in.BaseVersionID) || in.ThroughSequence <= floor {
		return domain.ErrConflict
	}
	var rows []schema.Message
	if err := tx.Where("conversation_id=? AND sequence>? AND sequence<=? AND (role='user' OR status='completed')", in.ConversationID, floor, in.ThroughSequence).Order("sequence").Limit(257).Find(&rows).Error; err != nil {
		return err
	}
	if len(rows) != len(in.Sources) {
		return domain.ErrConflict
	}
	repository, err := agentstore.NewRepository(tx)
	if err != nil {
		return err
	}
	traces, err := repository.ContextTranscript(tx.Statement.Context, in.ConversationID, floor)
	if err != nil {
		return err
	}
	for i, row := range rows {
		source := in.Sources[i]
		var trace []model.Message
		if row.Role == "user" {
			trace = traces[row.RunID]
		}
		hash, err := compaction.Fingerprint(message(row), trace)
		if err != nil {
			return err
		}
		if source.MessageID != row.ID || source.RunID != row.RunID || source.Sequence != row.Sequence || source.Fingerprint != hash {
			return domain.ErrConflict
		}
	}
	return nil
}

func (r *Repository) ActivateContext(ctx context.Context, in domain.ContextVersion) error {
	validated, err := compaction.Validate(in.Summary)
	if err != nil || len(in.Stats) > 4096 || !json.Valid(in.Stats) {
		return domain.ErrInvalid
	}
	in.Summary = validated
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := checkContextSource(tx, in); err != nil {
			return err
		}
		var row schema.ContextVersion
		if err := tx.Where("id=? AND status='building' AND conversation_id=?", in.ID, in.ConversationID).Take(&row).Error; err != nil {
			return err
		}
		// 摘要大小与原始来源固定在创建的版本上，不能借激活更换来源或父版本。
		if row.GeneratedByStepID != in.GeneratedByStepID || row.ThroughSequence != in.ThroughSequence || !slices.Equal(row.Sources, in.Sources) || (row.BaseVersionID == nil) != (in.BaseVersionID == nil) || (row.BaseVersionID != nil && *row.BaseVersionID != *in.BaseVersionID) {
			return domain.ErrConflict
		}
		now := time.Now().UnixMilli()
		if err := tx.Model(&schema.ContextVersion{}).Where("conversation_id=? AND status='active'", in.ConversationID).
			Updates(map[string]any{"status": "superseded", "updated_at": now}).Error; err != nil {
			return err
		}
		result := tx.Model(&schema.ContextVersion{}).Where("id=? AND status='building'", in.ID).
			Updates(map[string]any{"summary": in.Summary, "size_stats": string(in.Stats), "status": "active", "activated_at": now, "updated_at": now})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return domain.ErrConflict
		}
		output, err := json.Marshal(map[string]any{"compressed": true, "context_version_id": in.ID, "size_stats": in.Stats})
		if err != nil {
			return err
		}
		finished := tx.Model(&schema.Step{}).Where("id=? AND status='running'", in.GeneratedByStepID).Updates(map[string]any{"status": "completed", "output": string(output), "finished_at": now, "updated_at": now})
		if finished.Error != nil {
			return finished.Error
		}
		if finished.RowsAffected != 1 {
			return domain.ErrConflict
		}
		return nil
	})
}

func (r *Repository) FailContext(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Model(&schema.ContextVersion{}).Where("id=? AND status='building'", id).
		Updates(map[string]any{"status": "failed", "updated_at": time.Now().UnixMilli()}).Error
}

// loadContextHistory 仅读取未被有效摘要覆盖的消息，历史总量不再限制会话寿命。
func loadContextHistory(tx *gorm.DB, id string, before int64) ([]domain.Message, *domain.ContextVersion, error) {
	current, err := activeContext(tx, id)
	if err != nil {
		return nil, nil, err
	}
	floor := int64(0)
	if current != nil {
		floor = current.ThroughSequence
	}
	q := tx.Model(&schema.Message{}).Where("conversation_id=? AND sequence>? AND (role='user' OR status='completed')", id, floor)
	if before > 0 {
		q = q.Where("sequence<?", before)
	}
	var bounds struct{ Size, Count int64 }
	if err := q.Select("COALESCE(SUM(length(CAST(content AS BLOB))),0) AS size,COUNT(*) AS count").Scan(&bounds).Error; err != nil {
		return nil, nil, err
	}
	if bounds.Size > 16<<20 || bounds.Count > 511 {
		return nil, nil, agent.ErrOutputLimit
	}
	var rows []schema.Message
	if err := q.Select("conversation_messages.*").Order("sequence").Find(&rows).Error; err != nil {
		return nil, nil, err
	}
	out := make([]domain.Message, len(rows))
	for i, row := range rows {
		out[i] = message(row)
	}
	return out, current, nil
}

// invalidateContexts 与重发事务一起清理失效摘要及其模型结果，不删除操作审计和实际 usage。
func invalidateContexts(tx *gorm.DB, id string, sequence, now int64) error {
	parents := tx.Model(&schema.ContextVersion{}).Select("generated_by_step_id").Where("conversation_id=? AND through_sequence>=? AND status IN ?", id, sequence, []string{"active", "superseded", "building"})
	if err := tx.Model(&schema.Step{}).Where("type='model_call' AND parent_step_id IN (?)", parents).Update("output", nil).Error; err != nil {
		return err
	}
	return tx.Model(&schema.ContextVersion{}).Where("conversation_id=? AND through_sequence>=? AND status IN ?", id, sequence, []string{"active", "superseded", "building"}).Updates(map[string]any{"status": "invalid", "invalidated_at": now, "updated_at": now, "summary": ""}).Error
}
