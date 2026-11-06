package schema

import "github.com/xiaoyu98628/callum/internal/tool"

// DirectoryGrant 映射会话目录授权，撤销保留审计，不修改来源审批。
type DirectoryGrant struct {
	ID             string              `gorm:"type:text;primaryKey;not null;check:ck_directory_grant_id,id GLOB '????????-????-7???-[89ab]???-????????????' AND length(replace(id,'-','')) = 32 AND replace(id,'-','') NOT GLOB '*[^0-9a-f]*'"`
	ConversationID string              `gorm:"type:text;not null;index:ix_directory_grant_conversation,priority:1"`
	ApprovalID     string              `gorm:"type:text;not null;uniqueIndex:uq_directory_grant_approval"`
	Scope          tool.DirectoryScope `gorm:"type:text;serializer:json;not null;check:ck_directory_grant_scope,json_valid(scope) AND json_type(scope) = 'object' AND json_type(scope,'$.root_path') IS 'text' AND length(json_extract(scope,'$.root_path')) > 0 AND json_extract(scope,'$.access') IN ('read','read_write')"`
	CreatedAt      int64               `gorm:"not null;autoCreateTime:false"`
	RevokedAt      *int64              `gorm:"index:ix_directory_grant_conversation,priority:2"`
	Conversation   *Conversation       `gorm:"foreignKey:ConversationID;references:ID;constraint:OnDelete:RESTRICT"`
	Approval       *Approval           `gorm:"foreignKey:ApprovalID;references:ID;constraint:OnDelete:RESTRICT"`
}

func (DirectoryGrant) TableName() string { return "conversation_directory_grants" }
