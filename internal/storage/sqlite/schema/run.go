package schema

import (
	"github.com/xiaoyu98628/callum/internal/model"
	"github.com/xiaoyu98628/callum/internal/settings"
)

// Run 映射 conversation_runs（会话执行表），终态保留审计记录。
type Run struct {
	ID               string             `gorm:"type:text;primaryKey;not null;uniqueIndex:uq_run_identity,priority:1;check:ck_run_id,id GLOB '????????-????-7???-[89ab]???-????????????' AND length(replace(id,'-','')) = 32 AND replace(id,'-','') NOT GLOB '*[^0-9a-f]*'"`
	ConversationID   string             `gorm:"type:text;not null;uniqueIndex:uq_run_identity,priority:2;uniqueIndex:uq_active_conversation,where:status = 'running' OR status = 'awaiting_approval' OR status = 'awaiting_user_input';index:ix_run_history,priority:1"`
	ReplacesRunID    *string            `gorm:"type:text;check:ck_run_replaces,replaces_run_id IS NULL OR replaces_run_id <> id"`
	Model            model.Snapshot     `gorm:"type:text;not null;serializer:json;check:ck_run_model,json_valid(model) AND json_type(model) = 'object'"`
	Stream           bool               `gorm:"type:integer;not null;check:ck_run_stream,stream IN (0,1)"`
	SettingsSnapshot *settings.Snapshot `gorm:"type:text;serializer:json;check:ck_run_settings,settings_snapshot IS NULL OR (json_valid(settings_snapshot) AND json_type(settings_snapshot) = 'object' AND json_type(settings_snapshot,'$.revision') IS 'integer' AND json_extract(settings_snapshot,'$.revision') > 0 AND json_extract(settings_snapshot,'$.approval_policy') IN ('on-request','always','never') AND json_type(settings_snapshot,'$.approval_policy') IS 'text' AND COALESCE(json_extract(settings_snapshot,'$.approvals_reviewer') IN ('user','auto_review'),0))"`
	Status           string             `gorm:"type:text;not null;check:ck_run_status,status IN ('running','awaiting_approval','awaiting_user_input','completed','failed','cancelled','interrupted')"`
	ErrorKind        *string            `gorm:"type:text"`
	ErrorMessage     *string            `gorm:"type:text"`
	CreatedAt        int64              `gorm:"not null;autoCreateTime:false;index:ix_run_history,priority:2,sort:desc"`
	UpdatedAt        int64              `gorm:"not null;autoUpdateTime:false"`
	FinishedAt       *int64             `gorm:"check:ck_run_finished,(status IN ('running','awaiting_approval','awaiting_user_input') AND finished_at IS NULL) OR (status IN ('completed','failed','cancelled','interrupted') AND finished_at IS NOT NULL)"`
	Conversation     *Conversation      `gorm:"foreignKey:ConversationID;references:ID;constraint:OnDelete:RESTRICT"`
	Replaces         *Run               `gorm:"foreignKey:ReplacesRunID,ConversationID;references:ID,ConversationID;constraint:OnDelete:RESTRICT"`
}

func (Run) TableName() string { return "conversation_runs" }
