```go
package 表结构

import 模型包 "github.com/xiaoyu98628/callum/internal/model"

// 会话 映射 会话集（会话表），仅在首次提问时创建。
type 会话 struct {
	标识    string  `gorm:"type:text;primaryKey;not null;check:ck_conversation_id,id GLOB '????????-????-7???-[89ab]???-????????????' AND length(replace(id,'-','')) = 32 AND replace(id,'-','') NOT GLOB '*[^0-9a-f]*'"`
	工作区标识 *string `gorm:"type:text;index:ix_conversation_workspace"`
	标题    string  `gorm:"type:text;not null;check:ck_conversation_title,length(trim(title)) > 0"`
	标题来源  string  `gorm:"type:text;not null;default:default;check:ck_title_source,title_source IN ('default','manual','generated')"`
	模型    模型包.快照  `gorm:"type:text;not null;serializer:json;check:ck_conversation_model,json_valid(model) AND json_type(model) = 'object'"`
	置顶位于  *int64
	已归档位于 *int64
	删除时间  *int64 `gorm:"index:ix_conversation_listing,priority:1"`
	创建时间  int64  `gorm:"not null;autoCreateTime:false"`
	更新时间  int64  `gorm:"not null;autoUpdateTime:false;index:ix_conversation_listing,priority:2,sort:desc"`
	工作区   *工作区   `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnDelete:RESTRICT"`
}

func (会话) 表名称() string { return "conversations" }
```
