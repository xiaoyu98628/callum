```go
package 应用模块

import (
	上下文包 "context"
	JSON包 "encoding/json"
	格式化包 "fmt"
	输入输出包 "io"
	HTTP包 "net/http"
	HTTP测试包 "net/http/httptest"
	操作系统包 "os"
	文件路径包 "path/filepath"
	字符串包 "strings"
	测试包 "testing"

	SDK包 "github.com/modelcontextprotocol/go-sdk/mcp"
	智能体包 "github.com/xiaoyu98628/callum/internal/agent"
	配置模块包 "github.com/xiaoyu98628/callum/internal/config"
)

func TestMCP子级进程(测试器 *测试包.测试器) {
	标记 := 操作系统包.获取环境变量("CALLUM_MCP_FIXTURE_MARKER")
	if 标记 == "" {
		return
	}
	服务端 := SDK包.新建服务端(&SDK包.实现{名称: "审批测试服务", 版本: "1"}, nil)
	服务端.添加工具(&SDK包.工具{名称: "echo", 说明: "受控回显", 输入表结构: JSON包.原始JSON消息(`{"type":"object","properties":{"text":{"type":"string"}},"required":["text"],"additionalProperties":false}`)}, func(_ 上下文包.上下文, _ *SDK包.调用工具请求) (*SDK包.调用工具结果, error) {
		if 错误 := 操作系统包.写入文件(标记, []byte("called"), 0600); 错误 != nil {
			return nil, 错误
		}
		return &SDK包.调用工具结果{内容: []SDK包.内容{&SDK包.文本内容{文本: "mcp-returned"}}}, nil
	})
	if 错误 := 服务端.执行(测试器.上下文(), &SDK包.标准输入输出传输{}); 错误 != nil {
		操作系统包.退出进程(2)
	}
	操作系统包.退出进程(0)
}

func TestMCP标准输入输出审批通过公开协议(测试器 *测试包.测试器) {
	for _, 模式 := range []string{"approved", "rejected", "changed", "never"} {
		测试器.执行(模式, func(测试器 *测试包.测试器) {
			供应商 := HTTP测试包.新建服务端(HTTP包.处理器函数(func(当前响应写入器 HTTP包.响应写入器, 当前请求 *HTTP包.请求) {
				var 输入 struct {
					消息集 []struct {
						角色 string
						内容 string
					}
					工具集 []struct {
						函数 struct{ 名称, 说明 string }
					}
				}
				if JSON包.新建解码器(当前请求.正文).解码(&输入) != nil || len(输入.消息集) == 0 {
					测试器.错误文本("模型请求不完整")
					当前响应写入器.写入头部(400)
					return
				}
				当前响应写入器.头部().设置("Content-Type", "application/json")
				最后 := 输入.消息集[len(输入.消息集)-1]
				if 最后.角色 == "tool" {
					if 字符串包.包含(最后.内容, "mcp-returned") != (模式 == "approved") {
						测试器.格式化创建错误("工具结果和审批不匹配: %s", 最后.内容)
					}
					_, _ = 输入输出包.写入字符串(当前响应写入器, `{"choices":[{"index":0,"message":{"role":"assistant","content":"执行结束"},"finish_reason":"stop"}]}`)
					return
				}
				for _, 项 := range 输入.工具集 {
					if 字符串包.存在前缀(项.函数.名称, "mcp_") && 字符串包.包含(项.函数.说明, "受控回显") {
						_, _ = 格式化包.格式化输出(当前响应写入器, `{"choices":[{"index":0,"message":{"role":"assistant","content":"调用工具","tool_calls":[{"id":"mcp-test","type":"function","function":{"name":%q,"arguments":"{\"text\":\"示例\"}"}}]},"finish_reason":"tool_calls"}]}`, 项.函数.名称)
						return
					}
				}
				_, _ = 输入输出包.写入字符串(当前响应写入器, `{"choices":[{"index":0,"message":{"role":"assistant","content":"测试标题"},"finish_reason":"stop"}]}`)
			}))
			测试器.注册清理函数(供应商.关闭)
			启动配置, 错误 := 配置模块包.加载(文件路径包.合并(测试器.创建临时目录(), "data"))
			if 错误 != nil {
				测试器.报告致命错误(错误)
			}
			当前对象 := 开始应用(测试器, 启动配置, 输入输出包.丢弃)
			if 模式 == "never" {
				设置审批策略(测试器, 当前对象, "never")
			}
			可执行文件, 错误 := 操作系统包.可执行文件()
			if 错误 != nil {
				测试器.报告致命错误(错误)
			}
			标记 := 文件路径包.合并(测试器.创建临时目录(), "call")
			服务端 := map[string]any{
				"name": "回显 MCP", "transport": "stdio", "auth": "none", "enabled": true,
				"command": 可执行文件, "args": []string{"-test.run=^TestMCPChildProcess$"}, "allowed_tools": []string{"echo"},
				"env": map[string]string{"CALLUM_MCP_FIXTURE_MARKER": 标记},
			}
			未确认 := 当前对象.调用(测试器, "mcp.save", map[string]any{"revision": 1, "server": 服务端})
			if 未确认.错误 == nil {
				测试器.报告致命错误("未经确认启用本地进程")
			}
			标识 := 结果字符串(测试器, 当前对象.调用(测试器, "mcp.save", map[string]any{"revision": 1, "server": 服务端, "confirmed": true}), "id")
			if 当前对象.调用(测试器, "mcp.test", map[string]any{"id": 标识, "confirmed": true, "revision": 1}).错误 == nil {
				测试器.报告致命错误("允许用旧配置确认启动进程")
			}
			要求成功(测试器, 当前对象.调用(测试器, "mcp.test", map[string]any{"id": 标识, "confirmed": true, "revision": 2}))
			消息标识 := 初始化会话模型(测试器, 当前对象, 供应商.URL)
			已受理 := 当前对象.调用(测试器, "conversation.send", map[string]any{"model_id": 消息标识, "stream": false, "content": 消息内容块集("调用回显")})
			执行标识 := 结果字符串(测试器, 已受理, "run_id")
			if 模式 != "never" {
				var 载荷 struct {
					审批 智能体包.审批 `json:"approval"`
				}
				if 错误 := JSON包.反序列化(当前对象.事件(测试器, "approval.requested").参数, &载荷); 错误 != nil {
					测试器.报告致命错误(错误)
				}
				if !字符串包.包含(string(载荷.审批.请求), "回显 MCP") {
					测试器.报告致命错误("审批缺少工具来源")
				}
				if _, 错误 := 操作系统包.状态信息(标记); !操作系统包.是否不存在(错误) {
					测试器.报告致命错误("审批前调用了 MCP 工具")
				}
				决定 := 模式
				if 模式 == "changed" {
					服务端["id"] = 标识
					服务端["enabled"] = false
					要求成功(测试器, 当前对象.调用(测试器, "mcp.save", map[string]any{"revision": 2, "server": 服务端}))
					决定 = "approved"
				}
				要求成功(测试器, 当前对象.调用(测试器, "approval.decide", map[string]any{"approval_id": 载荷.审批.标识, "decision": 决定}))
			}
			当前对象.事件(测试器, "run.completed")
			_, 错误 = 操作系统包.状态信息(标记)
			if (错误 == nil) != (模式 == "approved") {
				测试器.格式化报告致命错误("审批后调用结果不符: %v", 错误)
			}
			结果 := 当前对象.调用(测试器, "step.list", map[string]any{"run_id": 执行标识, "limit": 20})
			要求成功(测试器, 结果)
			if !字符串包.包含(string(结果.结果), "回显 MCP") {
				测试器.格式化报告致命错误("执行记录缺少 MCP 来源: %s", 结果.结果)
			}
			当前对象.停止(测试器)
		})
	}
}
```
