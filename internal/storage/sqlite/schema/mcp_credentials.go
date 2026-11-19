package schema

// MCPCredential 映射 mcp_credentials（MCP 服务凭据表），与公开配置分开保存。
type MCPCredential struct {
	// ServerID 对应 app_settings 中 MCP 服务的稳定标识。
	ServerID string `gorm:"type:text;primaryKey;not null"`
	// Ciphertext 保存绑定到该服务实例的 AES-GCM 密文。
	Ciphertext string `gorm:"type:text;not null;check:ck_mcp_credential_ciphertext,length(ciphertext) > 0"`
	// UpdatedAt 保存本次凭据写入的 Unix 毫秒时间。
	UpdatedAt int64 `gorm:"not null;autoUpdateTime:false"`
}

func (MCPCredential) TableName() string { return "mcp_credentials" }
