```go
package 应用模块

import (
	JSON包 "encoding/json"
	文件路径包 "path/filepath"
	测试包 "testing"

	设置包 "github.com/xiaoyu98628/callum/internal/settings"
	SQLite包 "github.com/xiaoyu98628/callum/internal/storage/sqlite"
)

func Test常规设置通过标准输入输出并且重启(测试器 *测试包.测试器) {
	运行时, 启动配置 := 运行时测试夹具(测试器, 2)
	首个, 秒 := 共享通信对端(测试器, 运行时), 共享通信对端(测试器, 运行时)
	读取 := func(当前应用通信对端 *应用通信对端) 设置包.记录 {
		测试器.标记测试辅助函数()
		结果 := 当前应用通信对端.调用(测试器, "settings.get", map[string]any{"key": 设置包.常规键})
		要求成功(测试器, 结果)
		var 值 设置包.记录
		if 错误 := JSON包.反序列化(结果.结果, &值); 错误 != nil {
			测试器.报告致命错误(错误)
		}
		return 值
	}
	初始 := 读取(首个)
	if 值, 错误 := 设置包.解码常规记录(初始); 错误 != nil || 值 != 设置包.默认常规() || 初始.修订号 != 1 {
		测试器.格式化报告致命错误("常规默认值异常: %s, %v", 初始.值, 错误)
	}
	过时 := 读取(秒)
	for _, 原始 := range []string{
		`{}`, `{"follow_up_mode":"unknown"}`, `{"follow_up_mode":null}`,
		`{"follow_up_mode":"queue","follow_up_mode":"steer"}`,
		`{"follow_up_mode":"queue","extra":true}`,
	} {
		结果 := 首个.调用(测试器, "settings.update", map[string]any{"key": 设置包.常规键, "revision": 初始.修订号, "value": JSON包.原始JSON消息(原始)})
		if 结果.错误 == nil || 结果.错误.代码 != -32602 {
			测试器.格式化报告致命错误("接受了非法常规设置 %s: %+v", 原始, 结果)
		}
	}
	更新 := func(当前应用通信对端 *应用通信对端, 修订号 int64, 模式 string) 通信帧 {
		测试器.标记测试辅助函数()
		return 当前应用通信对端.调用(测试器, "settings.update", map[string]any{"key": 设置包.常规键, "revision": 修订号, "value": map[string]string{"follow_up_mode": 模式}})
	}
	要求成功(测试器, 更新(首个, 初始.修订号, "steer"))
	if 结果 := 更新(秒, 过时.修订号, "queue"); 结果.错误 == nil || 结果.错误.代码 != 1003 {
		测试器.格式化报告致命错误("旧版本写入未报告冲突: %+v", 结果)
	}
	首个.停止(测试器)
	秒.停止(测试器)
	if 错误 := 运行时.关闭(); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	数据库, 错误 := SQLite包.打开(测试器.上下文(), 文件路径包.合并(启动配置.数据目录, "callum.db"))
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	defer func() {
		if 错误 := 数据库.关闭(); 错误 != nil {
			测试器.错误文本(错误)
		}
	}()
	var 已存储 struct {
		值   string
		修订号 int64
	}
	if 错误 := 数据库.数据库.带有上下文(测试器.上下文()).表("app_settings").条件("key=?", 设置包.常规键).取出(&已存储).错误; 错误 != nil ||
		已存储.值 != `{"follow_up_mode":"steer"}` || 已存储.修订号 != 初始.修订号+1 {
		测试器.格式化报告致命错误("重启后常规设置未保留: %+v, %v", 已存储, 错误)
	}
}
```
