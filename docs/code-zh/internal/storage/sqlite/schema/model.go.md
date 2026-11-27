```go
package 表结构

import 供应商包 "github.com/xiaoyu98628/callum/internal/model/provider"

// 模型 映射 模型集（模型配置表），与客户端展示结构分开。
type 模型 struct {
	// 标识 是客户端选择模型时使用的 UUIDv7 本地主键，由服务端生成。
	标识 string `gorm:"type:text;primaryKey;not null;check:ck_model_id,id GLOB '????????-????-7???-[89ab]???-????????????' AND length(replace(id,'-','')) = 32 AND replace(id,'-','') NOT GLOB '*[^0-9a-f]*'"`
	// 供应商标识 关联厂商连接，并与 远程模型标识 组成联合唯一索引。
	供应商标识 string `gorm:"type:text;not null;uniqueIndex:uq_provider_remote_model,priority:1"`
	// 显示名称 是客户端模型列表中展示的名称。
	显示名称 string `gorm:"type:text;not null;check:ck_model_name,length(trim(display_name)) > 0"`
	// 远程模型标识 是调用厂商 API 时传递的模型标识，同一连接下不得重复。
	远程模型标识 string `gorm:"type:text;not null;uniqueIndex:uq_provider_remote_model,priority:2;check:ck_remote_model,length(trim(remote_model_id)) > 0"`
	// 已启用 控制能否发起新调用；指针保留显式 false，避免被 GORM 替换成默认值 true。
	已启用 *bool `gorm:"type:integer;not null;default:true;check:ck_model_enabled,enabled IN (0, 1)"`
	// 上下文窗口 是上下文窗口的 词元 容量，空 表示未知，已知值必须大于零。
	上下文窗口 *int64 `gorm:"check:ck_context_window,context_window > 0"`
	// 最大输出词元 是模型允许的最大输出 词元 数，空 表示未知。
	最大输出词元 *int64 `gorm:"check:ck_max_output,max_output_tokens > 0"`
	// 能力集 以 JSON 对象保存模型拥有的能力；空 或缺少键表示未知，不作为用户启用开关。
	能力集 map[string]bool `gorm:"type:text;serializer:json;check:ck_capabilities,capabilities IS NULL OR (json_valid(capabilities) AND json_type(capabilities) = 'object')"`
	// 模型类型 保存用户选中的 文本、图片、音频、video 类型；包含 图片 时启用图片输入。
	模型类型 []string `gorm:"type:text;serializer:json;not null;default:('[]');check:ck_model_type,json_valid(model_type) AND json_type(model_type) = 'array'"`
	// 推理等级集 保存推理等级的展示名称和远端值；空 表示未知，空数组表示没有选项。
	推理等级集 []供应商包.推理级别 `gorm:"type:text;serializer:json;check:ck_reasoning_levels,reasoning_levels IS NULL OR (json_valid(reasoning_levels) AND json_type(reasoning_levels) = 'array')"`
	// 创建时间 是创建时间，使用 UTC Unix 毫秒时间戳。
	创建时间 int64 `gorm:"not null;autoCreateTime:milli"`
	// 更新时间 是最近更新时间，使用 UTC Unix 毫秒时间戳。
	更新时间 int64 `gorm:"not null;autoUpdateTime:milli"`
	// 供应商 定义外键关系；仍被模型引用的厂商连接不能删除，不随模型读写自动保存关联。
	供应商 *供应商 `gorm:"foreignKey:ProviderID;references:ID;constraint:OnDelete:RESTRICT" json:"-"`
}

// 表名称 指定模型配置表名。
func (模型) 表名称() string { return "models" }
```
