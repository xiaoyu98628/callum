```go
package 标准输入输出

import (
	上下文包 "context"
	JSON包 "encoding/json"
	字符串包 "strings"
	时间包 "time"

	协议包 "github.com/xiaoyu98628/callum/internal/stdio/protocol"
	传输模块包 "github.com/xiaoyu98628/callum/internal/stdio/transport"
)

func (当前服务端 *服务端) 句柄(当前连接 *传输模块包.连接, 待处理 *网络请求集, 行 []byte) (bool, error) {
	请求, 解析错误 := 协议包.解析请求(行)
	if 解析错误 != nil {
		return false, 当前连接.发送(协议包.响应{JSONRPC: "2.0", 标识: JSON包.原始JSON消息("null"), 错误: 解析错误})
	}
	// 配置操作有独立短期超时；不能将它作为异步模型执行的父上下文。
	执行上下文, 取消函数 := 上下文包.创建超时上下文(当前连接.上下文(), 5*时间包.秒)
	defer 取消函数()
	回复 := func(结果 any, RPC错误 *协议包.错误) error {
		if RPC错误 != nil && RPC错误.代码 == -32603 {
			当前服务端.日志记录器.记录错误("请求处理失败", "module", "stdio", "method", 请求.方法, "request_id", string(请求.标识))
		}
		return 当前连接.回复(请求, 结果, RPC错误)
	}
	if 字符串包.存在前缀(请求.方法, "mcp.") && 当前服务端.MCP != nil {
		if 请求.方法 == "mcp.test" || 请求.方法 == "mcp.oauth.start" {
			select {
			case 待处理.槽位集 <- struct{}{}:
				待处理.等待组.Go(func() {
					defer func() { <-待处理.槽位集 }()
					执行上下文, 取消函数 := 上下文包.创建超时上下文(待处理.执行上下文, 12*时间包.秒)
					defer 取消函数()
					值, 错误 := 当前服务端.MCP.句柄(执行上下文, 待处理.执行上下文, 请求)
					if 回复错误 := 回复(值, 错误); 回复错误 != nil {
						当前连接.失败(回复错误)
					}
				})
				return false, nil
			default:
				return false, 回复(nil, &协议包.错误{代码: 3017, 消息: "多个 MCP 连接操作正在进行，请稍后重试"})
			}
		}
		值, 错误 := 当前服务端.MCP.句柄(执行上下文, 待处理.执行上下文, 请求)
		return false, 回复(值, 错误)
	}
	if 请求.方法 == "model.invoke" {
		return false, 当前服务端.执行.调用(执行上下文, 请求, 回复)
	}
	if 字符串包.存在前缀(请求.方法, "conversation.") && 当前服务端.会话 != nil {
		return false, 当前服务端.会话.句柄(执行上下文, 请求, 回复)
	}
	if 字符串包.存在前缀(请求.方法, "approval.") && 当前服务端.审批集 != nil {
		值, 错误 := 当前服务端.审批集.句柄(执行上下文, 请求)
		return false, 回复(值, 错误)
	}
	if 字符串包.存在前缀(请求.方法, "user_input.") && 当前服务端.输入集 != nil {
		值, 错误 := 当前服务端.输入集.句柄(执行上下文, 请求)
		return false, 回复(值, 错误)
	}
	if 字符串包.存在前缀(请求.方法, "settings.") && 当前服务端.设置 != nil {
		值, 错误 := 当前服务端.设置.句柄(执行上下文, 请求)
		return false, 回复(值, 错误)
	}
	if 请求.方法 == "skills.list" && 当前服务端.技能 != nil {
		值, 错误 := 当前服务端.技能.列表(执行上下文, 请求)
		return false, 回复(值, 错误)
	}
	var 结果 any
	var RPC错误 *协议包.错误
	停止 := false
	switch 请求.方法 {
	case "system.info", "system.shutdown":
		RPC错误 = 协议包.解码参数(请求.参数, &struct{}{})
		if RPC错误 == nil {
			结果 = 当前服务端.信息()
			停止 = 请求.方法 == "system.shutdown"
		}
	case "run.cancel":
		结果, RPC错误 = 当前服务端.执行.取消执行(请求)
	case "run.get", "run.list", "step.get", "step.list", "step.data":
		结果, RPC错误 = 当前服务端.执行.查询(执行上下文, 请求)
	default:
		if 字符串包.存在前缀(请求.方法, "workspace.") && 当前服务端.工作区 != nil {
			结果, RPC错误 = 当前服务端.工作区.句柄(执行上下文, 请求)
		} else {
			结果, RPC错误 = 当前服务端.目录管理.句柄(执行上下文, 请求)
		}
	}
	return 停止, 回复(结果, RPC错误)
}
```
