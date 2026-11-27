```go
package 表结构

import (
	模型包 "github.com/xiaoyu98628/callum/internal/model"
	设置包 "github.com/xiaoyu98628/callum/internal/settings"
)

// 执行 映射 会话执行集（会话执行表），终态保留审计记录。
type 执行 struct {
	标识     string  `gorm:"type:text;primaryKey;not null;uniqueIndex:uq_run_identity,priority:1;check:ck_run_id,id GLOB '????????-????-7???-[89ab]???-????????????' AND length(replace(id,'-','')) = 32 AND replace(id,'-','') NOT GLOB '*[^0-9a-f]*'"`
	会话标识   string  `gorm:"type:text;not null;uniqueIndex:uq_run_identity,priority:2;uniqueIndex:uq_active_conversation,where:status = 'running' OR status = 'awaiting_approval' OR status = 'awaiting_user_input';index:ix_run_history,priority:1"`
	替换执行标识 *string `gorm:"type:text;check:ck_run_replaces,replaces_run_id IS NULL OR replaces_run_id <> id"`
	模型     模型包.快照  `gorm:"type:text;not null;serializer:json;check:ck_run_model,json_valid(model) AND json_type(model) = 'object'"`
	流      bool    `gorm:"type:integer;not null;check:ck_run_stream,stream IN (0,1)"`
	设置快照   *设置包.快照 `gorm:"type:text;serializer:json;check:ck_run_settings,settings_snapshot IS NULL OR (json_valid(settings_snapshot) AND json_type(settings_snapshot) = 'object' AND json_type(settings_snapshot,'$.revision') IS 'integer' AND json_extract(settings_snapshot,'$.revision') > 0 AND json_extract(settings_snapshot,'$.approval_policy') IN ('on-request','always','never') AND json_type(settings_snapshot,'$.approval_policy') IS 'text' AND COALESCE(json_extract(settings_snapshot,'$.approvals_reviewer') IN ('user','auto_review'),0))"`
	状态     string  `gorm:"type:text;not null;check:ck_run_status,status IN ('running','awaiting_approval','awaiting_user_input','completed','failed','cancelled','interrupted')"`
	错误类别   *string `gorm:"type:text"`
	错误消息   *string `gorm:"type:text"`
	创建时间   int64   `gorm:"not null;autoCreateTime:false;index:ix_run_history,priority:2,sort:desc"`
	更新时间   int64   `gorm:"not null;autoUpdateTime:false"`
	结束时间   *int64  `gorm:"check:ck_run_finished,(status IN ('running','awaiting_approval','awaiting_user_input') AND finished_at IS NULL) OR (status IN ('completed','failed','cancelled','interrupted') AND finished_at IS NOT NULL)"`
	会话     *会话     `gorm:"foreignKey:ConversationID;references:ID;constraint:OnDelete:RESTRICT"`
	替换     *执行     `gorm:"foreignKey:ReplacesRunID,ConversationID;references:ID,ConversationID;constraint:OnDelete:RESTRICT"`
}

func (执行) 表名称() string { return "conversation_runs" }
```
