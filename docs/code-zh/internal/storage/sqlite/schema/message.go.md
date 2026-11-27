```go
package 表结构

import 模型包 "github.com/xiaoyu98628/callum/internal/model"

// 消息 映射 会话消息集（会话消息表），使用 序号 维护线性顺序。
type 消息 struct {
	标识    string    `gorm:"type:text;primaryKey;not null;check:ck_message_id,id GLOB '????????-????-7???-[89ab]???-????????????' AND length(replace(id,'-','')) = 32 AND replace(id,'-','') NOT GLOB '*[^0-9a-f]*'"`
	会话标识  string    `gorm:"type:text;not null;uniqueIndex:uq_message_sequence,priority:1"`
	执行标识  string    `gorm:"type:text;not null;uniqueIndex:uq_message_run_role,priority:1"`
	序号    int64     `gorm:"not null;uniqueIndex:uq_message_sequence,priority:2;check:ck_message_sequence,sequence > 0"`
	角色    string    `gorm:"type:text;not null;uniqueIndex:uq_message_run_role,priority:2;check:ck_message_role,role IN ('user','assistant')"`
	内容    []模型包.内容块 `gorm:"type:text;serializer:json;not null;default:('[]');check:ck_message_content,json_valid(content) AND json_type(content) = 'array'"`
	状态    string    `gorm:"type:text;not null;check:ck_message_status,status IN ('running','awaiting_approval','awaiting_user_input','completed','failed','cancelled','interrupted')"`
	创建时间  int64     `gorm:"not null;autoCreateTime:false"`
	更新时间  int64     `gorm:"not null;autoUpdateTime:false"`
	已完成位于 *int64    `gorm:"check:ck_message_completed,(status = 'completed' AND completed_at IS NOT NULL) OR (status <> 'completed' AND completed_at IS NULL)"`
	执行    *执行       `gorm:"foreignKey:RunID,ConversationID;references:ID,ConversationID;constraint:OnDelete:RESTRICT"`
}

func (消息) 表名称() string { return "conversation_messages" }
```
