```go
package 表结构

import JSON包 "encoding/json"

// 设置 映射 应用设置（应用设置表），一个配置组使用唯一 键 保存 JSON 内容。
// 修订号 用于并发更新校验；增加配置内容不要求新增数据库字段。
type 设置 struct {
	标识   string         `gorm:"type:text;primaryKey;not null;check:ck_settings_id,id GLOB '????????-????-7???-[89ab]???-????????????' AND length(replace(id,'-','')) = 32 AND replace(id,'-','') NOT GLOB '*[^0-9a-f]*'"`
	键    string         `gorm:"type:text;not null;uniqueIndex:uq_settings_key;check:ck_settings_key,length(key) BETWEEN 1 AND 64 AND length(trim(key)) = length(key)"`
	值    JSON包.原始JSON消息 `gorm:"type:text;not null;serializer:json;check:ck_settings_value,json_valid(value) AND json_type(value) = 'object' AND length(CAST(value AS BLOB)) <= 16384"`
	修订号  int64          `gorm:"not null;default:1;check:ck_settings_revision,revision > 0"`
	创建时间 int64          `gorm:"not null;autoCreateTime:false"`
	更新时间 int64          `gorm:"not null;autoUpdateTime:false"`
}

func (设置) 表名称() string { return "app_settings" }
```
