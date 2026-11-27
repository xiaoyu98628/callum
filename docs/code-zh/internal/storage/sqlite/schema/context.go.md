```go
package 表结构

import (
	JSON包 "encoding/json"

	会话包 "github.com/xiaoyu98628/callum/internal/conversation"
)

// 上下文版本 映射摘要版本；一个会话最多存在一个有效版本，原文不删除。
type 上下文版本 struct {
	标识        string         `gorm:"type:text;primaryKey;not null;check:ck_context_id,id GLOB '????????-????-7???-[89ab]???-????????????' AND length(replace(id,'-',''))=32 AND replace(id,'-','') NOT GLOB '*[^0-9a-f]*'"`
	会话标识      string         `gorm:"type:text;not null;uniqueIndex:uq_context_version,priority:1;uniqueIndex:uq_context_active,where:status = 'active';index:ix_context_history,priority:1"`
	版本        int64          `gorm:"not null;uniqueIndex:uq_context_version,priority:2;check:ck_context_version,version > 0"`
	基准版本标识    *string        `gorm:"type:text;check:ck_context_base,base_version_id IS NULL OR base_version_id <> id"`
	已生成依据步骤标识 string         `gorm:"type:text;not null"`
	通过序号      int64          `gorm:"not null;check:ck_context_sequence,through_sequence > 0"`
	摘要        string         `gorm:"type:text;not null;check:ck_context_summary_text,length(CAST(summary AS BLOB)) <= 65536"`
	来源集       []会话包.上下文来源    `gorm:"column:source_manifest;type:text;not null;serializer:json;check:ck_context_sources,json_valid(source_manifest) AND json_type(source_manifest) = 'array' AND json_array_length(source_manifest) BETWEEN 1 AND 256 AND length(CAST(source_manifest AS BLOB)) <= 131072"`
	统计        JSON包.原始JSON消息 `gorm:"column:size_stats;type:text;serializer:json;check:ck_context_stats,size_stats IS NULL OR (json_valid(size_stats) AND json_type(size_stats) = 'object' AND length(CAST(size_stats AS BLOB)) <= 4096)"`
	状态        string         `gorm:"type:text;not null;check:ck_context_status,status IN ('building','active','superseded','invalid','failed')"`
	创建时间      int64          `gorm:"not null;autoCreateTime:false;index:ix_context_history,priority:2,sort:desc"`
	更新时间      int64          `gorm:"not null;autoUpdateTime:false"`
	已激活位于     *int64         `gorm:"check:ck_context_activated,status NOT IN ('active','superseded') OR (activated_at IS NOT NULL AND length(trim(summary)) > 0)"`
	已失效位于     *int64
	会话        *会话    `gorm:"foreignKey:ConversationID;references:ID;constraint:OnDelete:RESTRICT"`
	已生成依据步骤   *步骤    `gorm:"foreignKey:GeneratedByStepID;references:ID;constraint:OnDelete:RESTRICT"`
	基准        *上下文版本 `gorm:"foreignKey:BaseVersionID;references:ID;constraint:OnDelete:RESTRICT"`
}

func (上下文版本) 表名称() string { return "conversation_context_versions" }
```
