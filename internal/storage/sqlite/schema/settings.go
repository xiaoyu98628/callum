package schema

import "encoding/json"

// Settings 映射 app_settings（应用设置表），一个配置组使用唯一 key 保存 JSON 内容。
// Revision 用于并发更新校验；增加配置内容不要求新增数据库字段。
type Settings struct {
	ID        string          `gorm:"type:text;primaryKey;not null;check:ck_settings_id,id GLOB '????????-????-7???-[89ab]???-????????????' AND length(replace(id,'-','')) = 32 AND replace(id,'-','') NOT GLOB '*[^0-9a-f]*'"`
	Key       string          `gorm:"type:text;not null;uniqueIndex:uq_settings_key;check:ck_settings_key,length(key) BETWEEN 1 AND 64 AND length(trim(key)) = length(key)"`
	Value     json.RawMessage `gorm:"type:text;not null;serializer:json;check:ck_settings_value,json_valid(value) AND json_type(value) = 'object' AND length(CAST(value AS BLOB)) <= 16384"`
	Revision  int64           `gorm:"not null;default:1;check:ck_settings_revision,revision > 0"`
	CreatedAt int64           `gorm:"not null;autoCreateTime:false"`
	UpdatedAt int64           `gorm:"not null;autoUpdateTime:false"`
}

func (Settings) TableName() string { return "app_settings" }
