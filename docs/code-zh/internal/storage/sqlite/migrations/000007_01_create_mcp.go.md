```go
package 迁移集

import (
	格式化包 "fmt"

	设置包 "github.com/xiaoyu98628/callum/internal/settings"
	表结构包 "github.com/xiaoyu98628/callum/internal/storage/sqlite/schema"
	GORM包 "gorm.io/gorm"
)

func 确保MCP(事务 *GORM包.数据库, 版本 int) error {
	表 := &表结构包.MCP凭据{}
	if 版本 == 0 && 事务.迁移器().存在表(表) {
		return 格式化包.格式化创建错误("数据库已包含 MCP 凭据表但缺少结构版本")
	}
	if 版本 < 7 {
		// 中文注释写入 SQLite 建表语句，便于直接从 SQLite表结构 查看。
		if 错误 := 事务.进程执行(`CREATE TABLE IF NOT EXISTS mcp_credentials (
	/* MCP 服务凭据表，保存独立加密的令牌与环境变量。 */
	server_id TEXT PRIMARY KEY NOT NULL, /* 对应应用设置中的 MCP 服务标识。 */
	ciphertext TEXT NOT NULL CONSTRAINT ck_mcp_credential_ciphertext CHECK (length(ciphertext) > 0), /* AES-GCM 密文。 */
	updated_at INTEGER NOT NULL /* 凭据更新时间，Unix 毫秒。 */
) STRICT`).错误; 错误 != nil {
			return 格式化包.格式化创建错误("创建 MCP 凭据表: %w", 错误)
		}
	}
	if 错误 := 校验核心表(事务, 表); 错误 != nil {
		return 错误
	}
	var 行 表结构包.设置
	if 错误 := 事务.条件("key = ?", 设置包.MCP键).上限(1).查找(&行).错误; 错误 != nil {
		return 错误
	}
	if 行.标识 == "" && 版本 < 7 {
		return 创建设置(事务, 设置包.MCP键, 设置包.默认MCP())
	}
	_, 错误 := 设置包.解码MCP记录(设置包.记录{键: 行.键, 值: 行.值, 修订号: 行.修订号})
	return 错误
}
```
