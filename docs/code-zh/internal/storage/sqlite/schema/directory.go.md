```go
package 表结构

import 工具包 "github.com/xiaoyu98628/callum/internal/tool"

// 目录授权 映射会话目录授权，撤销保留审计，不修改来源审批。
type 目录授权 struct {
	标识   string   `gorm:"type:text;primaryKey;not null;check:ck_directory_grant_id,id GLOB '????????-????-7???-[89ab]???-????????????' AND length(replace(id,'-','')) = 32 AND replace(id,'-','') NOT GLOB '*[^0-9a-f]*'"`
	会话标识 string   `gorm:"type:text;not null;index:ix_directory_grant_conversation,priority:1"`
	审批标识 string   `gorm:"type:text;not null;uniqueIndex:uq_directory_grant_approval"`
	范围   工具包.目录范围 `gorm:"type:text;serializer:json;not null;check:ck_directory_grant_scope,json_valid(scope) AND json_type(scope) = 'object' AND json_type(scope,'$.root_path') IS 'text' AND length(json_extract(scope,'$.root_path')) > 0 AND json_extract(scope,'$.access') IN ('read','read_write')"`
	创建时间 int64    `gorm:"not null;autoCreateTime:false"`
	撤销时间 *int64   `gorm:"index:ix_directory_grant_conversation,priority:2"`
	会话   *会话      `gorm:"foreignKey:ConversationID;references:ID;constraint:OnDelete:RESTRICT"`
	审批   *审批      `gorm:"foreignKey:ApprovalID;references:ID;constraint:OnDelete:RESTRICT"`
}

func (目录授权) 表名称() string { return "conversation_directory_grants" }
```
