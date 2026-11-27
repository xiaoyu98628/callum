```go
package 表结构

import (
	JSON包 "encoding/json"

	模型包 "github.com/xiaoyu98628/callum/internal/model"
)

// 步骤 映射 会话执行步骤集（执行步骤表），只在存储层使用。
type 步骤 struct {
	标识      string         `gorm:"type:text;primaryKey;not null;uniqueIndex:uq_step_run,priority:1;uniqueIndex:uq_step_identity,priority:1;check:ck_step_id,id GLOB '????????-????-7???-[89ab]???-????????????' AND length(replace(id,'-','')) = 32 AND replace(id,'-','') NOT GLOB '*[^0-9a-f]*'"`
	会话标识    string         `gorm:"type:text;not null;uniqueIndex:uq_step_identity,priority:3"`
	执行标识    string         `gorm:"type:text;not null;uniqueIndex:uq_step_identity,priority:2;uniqueIndex:uq_step_run,priority:2;uniqueIndex:uq_step_sequence,priority:1"`
	父级步骤标识  *string        `gorm:"type:text;check:ck_step_parent,parent_step_id IS NULL OR parent_step_id <> id"`
	序号      int64          `gorm:"not null;uniqueIndex:uq_step_sequence,priority:2;check:ck_step_sequence,sequence > 0"`
	类型      string         `gorm:"type:text;not null;check:ck_step_type,type IN ('model_call','tool_call','user_input','context_compression')"`
	名称      string         `gorm:"type:text;not null;check:ck_step_name,length(trim(name)) > 0"`
	定义      JSON包.原始JSON消息 `gorm:"type:text;serializer:json;check:ck_step_definition,definition IS NULL OR json_valid(definition)"`
	状态      string         `gorm:"type:text;not null;check:ck_step_status,status IN ('pending','running','awaiting_approval','awaiting_user_input','completed','failed','cancelled','interrupted')"`
	输入      JSON包.原始JSON消息 `gorm:"type:text;serializer:json;not null;default:('{}');check:ck_step_input,json_valid(input) AND json_type(input) = 'object'"`
	输出      JSON包.原始JSON消息 `gorm:"type:text;serializer:json;check:ck_step_output,output IS NULL OR json_valid(output)"`
	用量      JSON包.原始JSON消息 `gorm:"type:text;serializer:json;check:ck_step_usage,usage IS NULL OR (json_valid(usage) AND json_type(usage) = 'object')"`
	模型      *模型包.快照        `gorm:"type:text;serializer:json;check:ck_step_model,model IS NULL OR (json_valid(model) AND json_type(model) = 'object')"`
	供应商请求标识 *string        `gorm:"type:text"`
	工具调用标识  *string        `gorm:"type:text"`
	错误类别    *string        `gorm:"type:text"`
	错误消息    *string        `gorm:"type:text"`
	创建时间    int64          `gorm:"not null;autoCreateTime:false"`
	开始时间    *int64
	更新时间    int64  `gorm:"not null;autoUpdateTime:false"`
	结束时间    *int64 `gorm:"check:ck_step_finished,(status IN ('pending','running','awaiting_approval','awaiting_user_input') AND finished_at IS NULL) OR (status IN ('completed','failed','cancelled','interrupted') AND finished_at IS NOT NULL)"`
	执行      *执行    `gorm:"foreignKey:RunID,ConversationID;references:ID,ConversationID;constraint:OnDelete:RESTRICT"`
	父级      *步骤    `gorm:"foreignKey:ParentStepID,RunID,ConversationID;references:ID,RunID,ConversationID;constraint:OnDelete:RESTRICT"`
}

func (步骤) 表名称() string { return "conversation_run_steps" }
```
