```go
package 表结构

import 模型包 "github.com/xiaoyu98628/callum/internal/model"

// 后续消息 映射 conversation_followups（后续消息表），保存待发消息、取出的草稿和引导。
type 后续消息 struct {
	标识     string `gorm:"type:text;not null;primaryKey"`                         // 后续消息标识，同时作为私有附件目录的归属标识。
	会话标识   string `gorm:"type:text;not null;index:ix_followup_queue,priority:1"` // 所属会话。
	预期执行标识 string `gorm:"type:text;not null;index:ix_followup_run"`              // 队列等待的前序执行，或接收引导的当前执行。
	模型标识   string `gorm:"type:text;not null"`                                    // 将该条消息单独发送时使用的模型。
	// 已排队 待发，草稿 已取出编辑，引导 待模型采用，已生效 已采用，已发送 已单独发送，已移除 已移除。
	状态      string    `gorm:"type:text;not null;index:ix_followup_queue,priority:2;check:ck_followup_status,status IN ('queued','draft','steering','applied','sent','removed')"`
	内容      []模型包.内容块 `gorm:"type:text;not null;serializer:json;check:ck_followup_content,json_valid(content) AND json_type(content) = 'array'"` // 正文、技能和受服务端保管的附件引用。
	位置      int64     `gorm:"not null;index:ix_followup_queue,priority:3"`                                                                       // 待发顺序或引导受理顺序，升序排列。
	修订号     int64     `gorm:"not null"`                                                                                                          // 取出、移动和发送时的并发校验版本。
	已生效步骤标识 *string   `gorm:"type:text"`                                                                                                         // 实际采用引导的模型步骤，尚未采用时为空。
	创建时间    int64     `gorm:"not null;autoCreateTime:false"`                                                                                     // 创建时的 Unix 毫秒时间戳。
	会话      *会话       `gorm:"foreignKey:ConversationID;references:ID;constraint:OnDelete:RESTRICT"`
	执行      *执行       `gorm:"foreignKey:ExpectedRunID,ConversationID;references:ID,ConversationID;constraint:OnDelete:RESTRICT"`
}

func (后续消息) 表名称() string { return "conversation_followups" }
```
