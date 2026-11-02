package schema

// Provider 映射 model_providers（模型厂商连接配置表），不直接作为通信返回值。
// 每条记录保存一组协议、地址和凭据，同一厂商可以配置多个连接。
type Provider struct {
	// ID 是由服务端生成的 UUIDv7 主键，以标准小写文本保存。
	ID string `gorm:"type:text;primaryKey;not null;check:ck_provider_id,id GLOB '????????-????-7???-[89ab]???-????????????' AND length(replace(id,'-','')) = 32 AND replace(id,'-','') NOT GLOB '*[^0-9a-f]*'"`
	// Name 是厂商连接的展示名称，不要求唯一。
	Name string `gorm:"type:text;not null;check:ck_provider_name,length(trim(name)) > 0"`
	// Protocol 指定连接使用的接口协议，例如 chatcompletions。
	Protocol string `gorm:"type:text;not null;check:ck_provider_protocol,length(trim(protocol)) > 0"`
	// BaseURL 是该连接的 API 根地址，由关联模型共享。
	BaseURL string `gorm:"type:text;not null;check:ck_provider_url,length(trim(base_url)) > 0"`
	// APIKey 保存调用凭据，空字符串表示未配置；不得作为查询结果或日志输出。
	APIKey string `gorm:"type:text;not null;default:''" json:"-"`
	// CreatedAt 是创建时间，使用 UTC Unix 毫秒时间戳。
	CreatedAt int64 `gorm:"not null;autoCreateTime:milli"`
	// UpdatedAt 是最近更新时间，使用 UTC Unix 毫秒时间戳。
	UpdatedAt int64 `gorm:"not null;autoUpdateTime:milli"`
}

// TableName 指定模型厂商连接配置表名，避免依赖 GORM 的默认命名推断。
func (Provider) TableName() string { return "model_providers" }
