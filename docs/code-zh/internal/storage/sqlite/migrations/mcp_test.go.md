```go
package 迁移集测试

import (
	字符串包 "strings"
	测试包 "testing"

	SQLite包 "github.com/xiaoyu98628/callum/internal/storage/sqlite"
)

func Test版本六个添加已注释MCP存储(测试器 *测试包.测试器) {
	路径, 池 := 测试夹具数据库(测试器)
	执行上下文 := 测试器.上下文()
	数据库, 错误 := SQLite包.打开(执行上下文, 路径)
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	if 错误 := 数据库.关闭(); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	for _, 语句 := range []string{
		"DROP TABLE mcp_credentials",
		"DELETE FROM app_settings WHERE key='mcp'",
		"UPDATE app_settings SET revision=8 WHERE key='general'",
		"PRAGMA user_version=6",
	} {
		if _, 错误 := 池.进程执行上下文(执行上下文, 语句); 错误 != nil {
			测试器.报告致命错误(错误)
		}
	}
	数据库, 错误 = SQLite包.打开(执行上下文, 路径)
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	defer 数据库.关闭()
	var 建表语句 string
	if 错误 := 池.查询行上下文(执行上下文, "SELECT sql FROM sqlite_schema WHERE name='mcp_credentials'").扫描(&建表语句); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	for _, 注释 := range []string{"MCP 服务凭据表", "服务标识", "AES-GCM 密文", "Unix 毫秒"} {
		if !字符串包.包含(建表语句, 注释) {
			测试器.格式化报告致命错误("建表语句缺少中文注释 %q", 注释)
		}
	}
	var 版本, 修订号 int
	var 启动配置 string
	if 错误 := 池.查询行上下文(执行上下文, "SELECT value FROM app_settings WHERE key='mcp'").扫描(&启动配置); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	if 错误 := 池.查询行上下文(执行上下文, "SELECT revision FROM app_settings WHERE key='general'").扫描(&修订号); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	if 错误 := 池.查询行上下文(执行上下文, "PRAGMA user_version").扫描(&版本); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	if 启动配置 != `{"servers":[]}` || 版本 != 8 || 修订号 != 8 {
		测试器.格式化报告致命错误("版本 6 升级未完整迁移或覆盖了其他设置: %q %d %d", 启动配置, 版本, 修订号)
	}
}

func Test版本七个保留MCP范围期间升级(测试器 *测试包.测试器) {
	路径, 池 := 测试夹具数据库(测试器)
	执行上下文 := 测试器.上下文()
	数据库, 错误 := SQLite包.打开(执行上下文, 路径)
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	if 错误 := 数据库.关闭(); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	标识 := "019a4f39-0000-7000-8000-000000000001"
	值 := `{"servers":[{"id":"` + 标识 + `","name":"OAuth","enabled":false,"transport":"streamable_http","auth":"oauth","url":"https://example.invalid/mcp","scope":"read","allowed_tools":[]}]}`
	for _, 进程执行 := range []struct {
		查询 string
		参数 []any
	}{
		{"UPDATE app_settings SET value=?,revision=3 WHERE key='mcp'", []any{值}},
		{"INSERT INTO mcp_credentials(server_id,ciphertext,updated_at) VALUES(?,?,1)", []any{标识, "sealed-fixture"}},
		{"PRAGMA user_version=7", nil},
	} {
		if _, 错误 := 池.进程执行上下文(执行上下文, 进程执行.查询, 进程执行.参数...); 错误 != nil {
			测试器.报告致命错误(错误)
		}
	}
	数据库, 错误 = SQLite包.打开(执行上下文, 路径)
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	if 错误 := 数据库.关闭(); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	var 版本, 修订号 int
	var 已保存, 密钥 string
	if 错误 := 池.查询行上下文(执行上下文, "PRAGMA user_version").扫描(&版本); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	if 错误 := 池.查询行上下文(执行上下文, "SELECT value,revision FROM app_settings WHERE key='mcp'").扫描(&已保存, &修订号); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	if 错误 := 池.查询行上下文(执行上下文, "SELECT ciphertext FROM mcp_credentials WHERE server_id=?", 标识).扫描(&密钥); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	if 版本 != 8 || 已保存 != 值 || 修订号 != 3 || 密钥 != "sealed-fixture" {
		测试器.格式化报告致命错误("v7 升级未保留 MCP 设置和凭据: version=%d revision=%d", 版本, 修订号)
	}
}
```
