package schema

import "github.com/xiaoyu98628/callum/internal/model"

// FollowUp 映射 conversation_followups（后续消息表），保存待发消息、取出的草稿和引导。
type FollowUp struct {
	ID             string `gorm:"type:text;not null;primaryKey"`                         // 后续消息标识，同时作为私有附件目录的归属标识。
	ConversationID string `gorm:"type:text;not null;index:ix_followup_queue,priority:1"` // 所属会话。
	ExpectedRunID  string `gorm:"type:text;not null;index:ix_followup_run"`              // 队列等待的前序执行，或接收引导的当前执行。
	ModelID        string `gorm:"type:text;not null"`                                    // 将该条消息单独发送时使用的模型。
	// queued 待发，draft 已取出编辑，steering 待模型采用，applied 已采用，sent 已单独发送，removed 已移除。
	Status        string        `gorm:"type:text;not null;index:ix_followup_queue,priority:2;check:ck_followup_status,status IN ('queued','draft','steering','applied','sent','removed')"`
	Content       []model.Block `gorm:"type:text;not null;serializer:json;check:ck_followup_content,json_valid(content) AND json_type(content) = 'array'"` // 正文、技能和受服务端保管的附件引用。
	Position      int64         `gorm:"not null;index:ix_followup_queue,priority:3"`                                                                       // 待发顺序或引导受理顺序，升序排列。
	Revision      int64         `gorm:"not null"`                                                                                                          // 取出、移动和发送时的并发校验版本。
	AppliedStepID *string       `gorm:"type:text"`                                                                                                         // 实际采用引导的模型步骤，尚未采用时为空。
	CreatedAt     int64         `gorm:"not null;autoCreateTime:false"`                                                                                     // 创建时的 Unix 毫秒时间戳。
	Conversation  *Conversation `gorm:"foreignKey:ConversationID;references:ID;constraint:OnDelete:RESTRICT"`
	Run           *Run          `gorm:"foreignKey:ExpectedRunID,ConversationID;references:ID,ConversationID;constraint:OnDelete:RESTRICT"`
}

func (FollowUp) TableName() string { return "conversation_followups" }
