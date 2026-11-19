package migrations

import (
	"fmt"

	"github.com/xiaoyu98628/callum/internal/settings"
	"github.com/xiaoyu98628/callum/internal/storage/sqlite/schema"
	"gorm.io/gorm"
)

func ensureMCP(tx *gorm.DB, version int) error {
	table := &schema.MCPCredential{}
	if version == 0 && tx.Migrator().HasTable(table) {
		return fmt.Errorf("数据库已包含 MCP 凭据表但缺少结构版本")
	}
	if version < 7 {
		// 中文注释写入 SQLite 建表语句，便于直接从 sqlite_schema 查看。
		if err := tx.Exec(`CREATE TABLE IF NOT EXISTS mcp_credentials (
	/* MCP 服务凭据表，保存独立加密的令牌与环境变量。 */
	server_id TEXT PRIMARY KEY NOT NULL, /* 对应应用设置中的 MCP 服务标识。 */
	ciphertext TEXT NOT NULL CONSTRAINT ck_mcp_credential_ciphertext CHECK (length(ciphertext) > 0), /* AES-GCM 密文。 */
	updated_at INTEGER NOT NULL /* 凭据更新时间，Unix 毫秒。 */
) STRICT`).Error; err != nil {
			return fmt.Errorf("创建 MCP 凭据表: %w", err)
		}
	}
	if err := validateCoreTable(tx, table); err != nil {
		return err
	}
	var row schema.Settings
	if err := tx.Where("key = ?", settings.MCPKey).Limit(1).Find(&row).Error; err != nil {
		return err
	}
	if row.ID == "" && version < 7 {
		return createSetting(tx, settings.MCPKey, settings.DefaultMCP())
	}
	_, err := settings.DecodeMCPRecord(settings.Record{Key: row.Key, Value: row.Value, Revision: row.Revision})
	return err
}
