package schema

import "github.com/xiaoyu98628/callum/internal/model"

// FollowUp 保存队列和运行中的用户引导。已受理记录保留附件归属。
type FollowUp struct {
	ID             string        `gorm:"type:text;not null;primaryKey"`
	ConversationID string        `gorm:"type:text;not null;index:ix_followup_queue,priority:1"`
	ExpectedRunID  string        `gorm:"type:text;not null;index:ix_followup_run"`
	ModelID        string        `gorm:"type:text;not null"`
	Status         string        `gorm:"type:text;not null;index:ix_followup_queue,priority:2;check:ck_followup_status,status IN ('queued','steering','applied','sent','removed')"`
	Content        []model.Block `gorm:"type:text;not null;serializer:json;check:ck_followup_content,json_valid(content) AND json_type(content) = 'array'"`
	Position       int64         `gorm:"not null;index:ix_followup_queue,priority:3"`
	Revision       int64         `gorm:"not null"`
	AppliedStepID  *string       `gorm:"type:text"`
	CreatedAt      int64         `gorm:"not null;autoCreateTime:false"`
	Conversation   *Conversation `gorm:"foreignKey:ConversationID;references:ID;constraint:OnDelete:RESTRICT"`
	Run            *Run          `gorm:"foreignKey:ExpectedRunID,ConversationID;references:ID,ConversationID;constraint:OnDelete:RESTRICT"`
}

func (FollowUp) TableName() string { return "conversation_followups" }
