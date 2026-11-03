package schema

import "github.com/xiaoyu98628/callum/internal/model"

// Conversation 映射 conversations（会话表），仅在首次提问时创建。
type Conversation struct {
	ID          string         `gorm:"type:text;primaryKey;not null;check:ck_conversation_id,id GLOB '????????-????-7???-[89ab]???-????????????' AND length(replace(id,'-','')) = 32 AND replace(id,'-','') NOT GLOB '*[^0-9a-f]*'"`
	WorkspaceID *string        `gorm:"type:text;index:ix_conversation_workspace"`
	Title       string         `gorm:"type:text;not null;check:ck_conversation_title,length(trim(title)) > 0"`
	TitleSource string         `gorm:"type:text;not null;default:default;check:ck_title_source,title_source IN ('default','manual','generated')"`
	Model       model.Snapshot `gorm:"type:text;not null;serializer:json;check:ck_conversation_model,json_valid(model) AND json_type(model) = 'object'"`
	PinnedAt    *int64
	ArchivedAt  *int64
	DeletedAt   *int64     `gorm:"index:ix_conversation_listing,priority:1"`
	CreatedAt   int64      `gorm:"not null;autoCreateTime:false"`
	UpdatedAt   int64      `gorm:"not null;autoUpdateTime:false;index:ix_conversation_listing,priority:2,sort:desc"`
	Workspace   *Workspace `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnDelete:RESTRICT"`
}

func (Conversation) TableName() string { return "conversations" }
