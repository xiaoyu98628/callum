```go
package 表结构

// 供应商 映射 模型供应商集（模型厂商连接配置表），不直接作为通信返回值。
// 每条记录保存一组协议、地址和凭据，同一厂商可以配置多个连接。
type 供应商 struct {
	// 标识 是由服务端生成的 UUIDv7 主键，以标准小写文本保存。
	标识 string `gorm:"type:text;primaryKey;not null;check:ck_provider_id,id GLOB '????????-????-7???-[89ab]???-????????????' AND length(replace(id,'-','')) = 32 AND replace(id,'-','') NOT GLOB '*[^0-9a-f]*'"`
	// 名称 是厂商连接的展示名称，不要求唯一。
	名称 string `gorm:"type:text;not null;check:ck_provider_name,length(trim(name)) > 0"`
	// 协议 指定连接使用的接口协议，例如 聊天补全。
	协议 string `gorm:"type:text;not null;check:ck_provider_protocol,length(trim(protocol)) > 0"`
	// 基础URL 是该连接的 API 根地址，由关联模型共享。
	基础URL string `gorm:"type:text;not null;check:ck_provider_url,length(trim(base_url)) > 0"`
	// API密钥 保存 AES-GCM 密文，空字符串表示未配置；不得作为查询结果或日志输出。
	API密钥 string `gorm:"type:text;not null;default:''" json:"-"`
	// 凭据格式 的 0 仅用于待迁移的旧记录，1 表示加密或空凭据。
	凭据格式 int `gorm:"type:integer;not null;default:1;check:ck_provider_credential_format,credential_format IN (0,1)"`
	// 创建时间 是创建时间，使用 UTC Unix 毫秒时间戳。
	创建时间 int64 `gorm:"not null;autoCreateTime:milli"`
	// 更新时间 是最近更新时间，使用 UTC Unix 毫秒时间戳。
	更新时间 int64 `gorm:"not null;autoUpdateTime:milli"`
}

// 表名称 指定模型厂商连接配置表名，避免依赖 GORM 的默认命名推断。
func (供应商) 表名称() string { return "model_providers" }
```
