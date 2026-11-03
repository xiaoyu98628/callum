package schema

import "github.com/xiaoyu98628/callum/internal/model"

// Message 映射 conversation_messages（会话消息表），使用 sequence 维护线性顺序。
type Message struct {
	ID             string        `gorm:"type:text;primaryKey;not null;check:ck_message_id,id GLOB '????????-????-7???-[89ab]???-????????????' AND length(replace(id,'-','')) = 32 AND replace(id,'-','') NOT GLOB '*[^0-9a-f]*'"`
	ConversationID string        `gorm:"type:text;not null;uniqueIndex:uq_message_sequence,priority:1"`
	RunID          string        `gorm:"type:text;not null;uniqueIndex:uq_message_run_role,priority:1"`
	Sequence       int64         `gorm:"not null;uniqueIndex:uq_message_sequence,priority:2;check:ck_message_sequence,sequence > 0"`
	Role           string        `gorm:"type:text;not null;uniqueIndex:uq_message_run_role,priority:2;check:ck_message_role,role IN ('user','assistant')"`
	Content        []model.Block `gorm:"type:text;serializer:json;not null;default:('[]');check:ck_message_content,json_valid(content) AND json_type(content) = 'array'"`
	Status         string        `gorm:"type:text;not null;check:ck_message_status,status IN ('running','awaiting_approval','awaiting_user_input','completed','failed','cancelled','interrupted')"`
	CreatedAt      int64         `gorm:"not null;autoCreateTime:false"`
	UpdatedAt      int64         `gorm:"not null;autoUpdateTime:false"`
	CompletedAt    *int64        `gorm:"check:ck_message_completed,(status = 'completed' AND completed_at IS NOT NULL) OR (status <> 'completed' AND completed_at IS NULL)"`
	Run            *Run          `gorm:"foreignKey:RunID,ConversationID;references:ID,ConversationID;constraint:OnDelete:RESTRICT"`
}

func (Message) TableName() string { return "conversation_messages" }
