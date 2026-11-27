```go
package 迁移集测试

import (
	文件路径包 "path/filepath"
	字符串包 "strings"
	测试包 "testing"

	SQLite包 "github.com/xiaoyu98628/callum/internal/storage/sqlite"
)

func Test版本三个添加凭据格式无变化中旧版值集(测试器 *测试包.测试器) {
	路径 := 文件路径包.合并(测试器.创建临时目录(), "data", "callum.db")
	数据库, 错误 := SQLite包.打开(测试器.上下文(), 路径)
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	var 建表语句 string
	if 错误 := 数据库.数据库.原始("SELECT sql FROM sqlite_schema WHERE name='model_providers'").扫描(&建表语句).错误; 错误 != nil {
		测试器.报告致命错误(错误)
	}
	旧版 := 字符串包.替换(建表语句, "`credential_format` integer NOT NULL DEFAULT 1,", "", 1)
	旧版 = 字符串包.替换(旧版, ",CONSTRAINT `ck_provider_credential_format` CHECK (credential_format IN (0,1))", "", 1)
	if 字符串包.包含(旧版, "credential_format") {
		测试器.报告致命错误("无法构造完整 v3 表结构", 旧版)
	}
	for _, 查询 := range []string{
		"DROP TABLE model_providers",
		旧版,
		"INSERT INTO model_providers(id,name,protocol,base_url,api_key,created_at,updated_at) VALUES ('019a4f39-0000-7000-8000-000000000001','旧厂商','chatcompletions','https://example.invalid','legacy-private-value',1,1)",
		"PRAGMA user_version=3",
	} {
		if 错误 := 数据库.数据库.进程执行(查询).错误; 错误 != nil {
			测试器.报告致命错误(错误)
		}
	}
	if 错误 := 数据库.关闭(); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	已升级, 错误 := SQLite包.打开(测试器.上下文(), 路径)
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	defer 已升级.关闭()
	var 值 struct {
		API密钥 string
		凭据格式  int
	}
	if 错误 := 已升级.数据库.表("model_providers").选择("api_key,credential_format").取出(&值).错误; 错误 != nil || 值.API密钥 != "legacy-private-value" || 值.凭据格式 != 0 {
		测试器.报告致命错误("结构升级丢失旧值或错误标记密文", 错误)
	}
	if 错误 := 已升级.数据库.进程执行("UPDATE model_providers SET credential_format=2").错误; 错误 == nil {
		测试器.报告致命错误("密文格式缺少结构校验")
	}
}
```
