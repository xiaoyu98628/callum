```go
package MCP

import (
	上下文包 "context"
	测试包 "testing"
	时间包 "time"

	模型包 "github.com/xiaoyu98628/callum/internal/model"
	设置包 "github.com/xiaoyu98628/callum/internal/settings"
	工具包 "github.com/xiaoyu98628/callum/internal/tool"
)

func Test工具集快照保留已缓存工具当发现预算过期时间(测试器 *测试包.测试器) {
	缓慢 := &条目{执行上下文: 测试器.上下文(), 加锁: make(chan struct{}, 1), 启动配置: 设置包.MCP服务端{
		标识: "slow", 已启用: true, 允许工具集: []string{"slow"},
	}}
	就绪 := &条目{执行上下文: 测试器.上下文(), 加锁: make(chan struct{}, 1), 启动配置: 设置包.MCP服务端{
		标识: "ready", 已启用: true, 允许工具集: []string{"cached"},
	}}
	缓慢.加锁 <- struct{}{} // 模拟其他请求占用排在前面的服务。
	就绪.加锁 <- struct{}{} // 读取已发布的目录也不应等待工具执行。
	连接 := &连接{}
	已缓存 := &远程工具{名称: "cached", 定义: 工具包.定义{工具定义: 模型包.工具定义{名称: "cached"}}}
	发布工具集(连接, map[string]*远程工具{"cached": 已缓存})
	就绪.公开连接.存储(连接)
	当前状态 := &服务{修订号: 1, 启动配置: 设置包.MCP{服务集: []设置包.MCP服务端{缓慢.启动配置, 就绪.启动配置}}, 条目集: map[string]*条目{
		"slow": 缓慢, "ready": 就绪,
	}}

	执行上下文, 取消函数 := 上下文包.创建超时上下文(测试器.上下文(), 30*时间包.毫秒)
	defer 取消函数()
	实际值 := 当前状态.工具集快照(执行上下文, true, 1)
	if len(实际值) != 1 || 实际值[0] != 已缓存 {
		测试器.格式化报告致命错误("等待超时不应丢失另一服务已发布的工具: %v", 实际值)
	}
}
```
