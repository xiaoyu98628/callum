package schema

import (
	"encoding/json"

	"github.com/xiaoyu98628/callum/internal/conversation"
)

// ContextVersion 映射摘要版本；一个会话最多存在一个有效版本，原文不删除。
type ContextVersion struct {
	ID                string                       `gorm:"type:text;primaryKey;not null;check:ck_context_id,id GLOB '????????-????-7???-[89ab]???-????????????' AND length(replace(id,'-',''))=32 AND replace(id,'-','') NOT GLOB '*[^0-9a-f]*'"`
	ConversationID    string                       `gorm:"type:text;not null;uniqueIndex:uq_context_version,priority:1;uniqueIndex:uq_context_active,where:status = 'active';index:ix_context_history,priority:1"`
	Version           int64                        `gorm:"not null;uniqueIndex:uq_context_version,priority:2;check:ck_context_version,version > 0"`
	BaseVersionID     *string                      `gorm:"type:text;check:ck_context_base,base_version_id IS NULL OR base_version_id <> id"`
	GeneratedByStepID string                       `gorm:"type:text;not null"`
	ThroughSequence   int64                        `gorm:"not null;check:ck_context_sequence,through_sequence > 0"`
	Summary           string                       `gorm:"type:text;not null;check:ck_context_summary,length(CAST(summary AS BLOB)) <= 65536 AND (summary = '' OR (json_valid(summary) AND json_type(summary) = 'object'))"`
	Sources           []conversation.ContextSource `gorm:"column:source_manifest;type:text;not null;serializer:json;check:ck_context_sources,json_valid(source_manifest) AND json_type(source_manifest) = 'array' AND json_array_length(source_manifest) BETWEEN 1 AND 256 AND length(CAST(source_manifest AS BLOB)) <= 131072"`
	Stats             json.RawMessage              `gorm:"column:size_stats;type:text;serializer:json;check:ck_context_stats,size_stats IS NULL OR (json_valid(size_stats) AND json_type(size_stats) = 'object' AND length(CAST(size_stats AS BLOB)) <= 4096)"`
	Status            string                       `gorm:"type:text;not null;check:ck_context_status,status IN ('building','active','superseded','invalid','failed')"`
	CreatedAt         int64                        `gorm:"not null;autoCreateTime:false;index:ix_context_history,priority:2,sort:desc"`
	UpdatedAt         int64                        `gorm:"not null;autoUpdateTime:false"`
	ActivatedAt       *int64                       `gorm:"check:ck_context_activated,status NOT IN ('active','superseded') OR (activated_at IS NOT NULL AND length(trim(summary)) > 0)"`
	InvalidatedAt     *int64
	Conversation      *Conversation   `gorm:"foreignKey:ConversationID;references:ID;constraint:OnDelete:RESTRICT"`
	GeneratedByStep   *Step           `gorm:"foreignKey:GeneratedByStepID;references:ID;constraint:OnDelete:RESTRICT"`
	Base              *ContextVersion `gorm:"foreignKey:BaseVersionID;references:ID;constraint:OnDelete:RESTRICT"`
}

func (ContextVersion) TableName() string { return "conversation_context_versions" }
