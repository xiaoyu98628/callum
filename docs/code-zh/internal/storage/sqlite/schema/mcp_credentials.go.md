```go
package 表结构

// MCP凭据 映射 MCP凭据（MCP 服务凭据表），与公开配置分开保存。
type MCP凭据 struct {
	// 服务端标识 对应 应用设置 中 MCP 服务的稳定标识。
	服务端标识 string `gorm:"type:text;primaryKey;not null"`
	// 密文 保存绑定到该服务实例的 AES-GCM 密文。
	密文 string `gorm:"type:text;not null;check:ck_mcp_credential_ciphertext,length(ciphertext) > 0"`
	// 更新时间 保存本次凭据写入的 Unix 毫秒时间。
	更新时间 int64 `gorm:"not null;autoUpdateTime:false"`
}

func (MCP凭据) 表名称() string { return "mcp_credentials" }
```
