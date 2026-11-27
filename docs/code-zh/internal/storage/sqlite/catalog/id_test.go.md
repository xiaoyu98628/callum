```go
package 目录管理测试

import (
	文件路径包 "path/filepath"
	测试包 "testing"

	供应商包 "github.com/xiaoyu98628/callum/internal/model/provider"
	SQLite包 "github.com/xiaoyu98628/callum/internal/storage/sqlite"
)

func TestUUID主要键集并且约束集(测试器 *测试包.测试器) {
	数据库, 错误 := SQLite包.打开(测试器.上下文(), 文件路径包.合并(测试器.创建临时目录(), "data", "callum.db"))
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	defer func() {
		if 错误 := 数据库.关闭(); 错误 != nil {
			测试器.错误文本(错误)
		}
	}()
	存储, 错误 := 新建服务(数据库.数据库)
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	已见 := map[string]bool{}
	for range 8 {
		进程标识, 错误 := 存储.创建供应商(测试器.上下文(), 供应商包.供应商输入{名称: "厂商", 协议: "chatcompletions", 基础URL: "https://example.invalid"})
		if 错误 != nil {
			测试器.报告致命错误(错误)
		}
		标识, 错误 := 存储.创建模型(测试器.上下文(), 供应商包.模型输入{供应商标识: 进程标识, 显示名称: "模型", 远程模型标识: "remote", 已启用: true})
		if 错误 != nil {
			测试器.报告致命错误(错误)
		}
		for _, 值 := range []string{进程标识, 标识} {
			if !供应商包.有效标识(值) || 已见[值] {
				测试器.格式化报告致命错误("UUID 版本或唯一性错误: %s", 值)
			}
			已见[值] = true
		}
		if _, 错误 := 存储.解析(测试器.上下文(), 标识); 错误 != nil {
			测试器.格式化报告致命错误("UUID 关联失败: %v", 错误)
		}
	}
	for _, 无效 := range []string{"1", "", "01900000-0000-4000-8000-000000000001", "01900000-0000-7000-c000-000000000001", "01900000-0000-7000-8000-00000000000G"} {
		if 供应商包.有效标识(无效) {
			测试器.格式化报告致命错误("接受了非规范 UUIDv7: %q", 无效)
		}
		if 错误 := 数据库.数据库.进程执行("INSERT INTO model_providers(id,name,protocol,base_url,api_key,created_at,updated_at) VALUES(?,'p','chatcompletions','https://example.invalid','',0,0)", 无效).错误; 错误 == nil {
			测试器.格式化报告致命错误("数据库未约束 UUIDv7: %q", 无效)
		}
	}
}
```
