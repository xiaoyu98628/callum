```go
package 表结构

import JSON包 "encoding/json"

// 审批 映射 会话执行审批集（执行审批表），保留用户决定及操作快照。
type 审批 struct {
	标识    string         `gorm:"type:text;primaryKey;not null;check:ck_approval_id,id GLOB '????????-????-7???-[89ab]???-????????????' AND length(replace(id,'-','')) = 32 AND replace(id,'-','') NOT GLOB '*[^0-9a-f]*'"`
	执行标识  string         `gorm:"type:text;not null;index:ix_approval_run,priority:1"`
	步骤标识  string         `gorm:"type:text;not null;uniqueIndex:uq_approval_step"`
	请求    JSON包.原始JSON消息 `gorm:"type:text;serializer:json;not null;check:ck_approval_request,json_valid(request) AND json_type(request) = 'object'"`
	请求哈希  string         `gorm:"type:text;not null;check:ck_approval_hash,length(request_hash) = 64 AND request_hash NOT GLOB '*[^0-9a-f]*'"`
	状态    string         `gorm:"type:text;not null;default:pending;index:ix_approval_pending;check:ck_approval_status,status IN ('pending','approved','rejected','cancelled')"`
	已决定依据 *string        `gorm:"type:text"`
	决定说明  *string        `gorm:"type:text"`
	创建时间  int64          `gorm:"not null;autoCreateTime:false;index:ix_approval_run,priority:2"`
	更新时间  int64          `gorm:"not null;autoUpdateTime:false"`
	已决定位于 *int64         `gorm:"check:ck_approval_decision,(status = 'pending' AND decided_at IS NULL) OR (status <> 'pending' AND decided_at IS NOT NULL)"`
	执行    *执行            `gorm:"foreignKey:RunID;references:ID;constraint:OnDelete:RESTRICT"`
	步骤    *步骤            `gorm:"foreignKey:StepID,RunID;references:ID,RunID;constraint:OnDelete:RESTRICT"`
}

func (审批) 表名称() string { return "conversation_run_approvals" }
```
