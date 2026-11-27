```go
package MCP

import (
	上下文包 "context"
	错误判断包 "errors"

	凭据包 "github.com/xiaoyu98628/callum/internal/credentials"
	服务包 "github.com/xiaoyu98628/callum/internal/mcp"
	设置包 "github.com/xiaoyu98628/callum/internal/settings"
	协议包 "github.com/xiaoyu98628/callum/internal/stdio/protocol"
)

type 处理器 struct{ 服务 *服务包.服务 }

func 新建(当前服务 *服务包.服务) *处理器 { return &处理器{服务包: 当前服务} }

func (当前处理器 *处理器) 句柄(执行上下文, 所属对象 上下文包.上下文, 请求 协议包.请求) (any, *协议包.错误) {
	var 结果 any
	var 错误 error
	switch 请求.方法 {
	case "mcp.list":
		if 当前条目 := 协议包.解码参数(请求.参数, &struct{}{}); 当前条目 != nil {
			return nil, 当前条目
		}
		结果, 错误 = 当前处理器.服务.列表(执行上下文)
	case "mcp.save":
		var 输入 struct {
			修订号  int64      `json:"revision"`
			服务端  设置包.MCP服务端 `json:"server"`
			密钥集  服务包.密钥输入   `json:"secrets"`
			已确认  bool       `json:"confirmed"`
			清空范围 bool       `json:"clear_scope,omitempty"`
		}
		if 当前条目 := 协议包.解码参数(请求.参数, &输入); 当前条目 != nil {
			return nil, 当前条目
		}
		var 标识 string
		标识, 错误 = 当前处理器.服务.保存(执行上下文, 输入.修订号, 输入.服务端, 输入.密钥集, 服务包.保存选项集{
			已确认: 输入.已确认, 清空范围: 输入.清空范围,
		})
		结果 = map[string]string{"id": 标识}
	case "mcp.delete":
		var 输入 struct {
			标识  string `json:"id"`
			修订号 int64  `json:"revision"`
		}
		if 当前条目 := 协议包.解码参数(请求.参数, &输入); 当前条目 != nil {
			return nil, 当前条目
		}
		错误 = 当前处理器.服务.删除(执行上下文, 输入.修订号, 输入.标识)
		结果 = true
	case "mcp.test":
		var 输入 struct {
			修订号 int64  `json:"revision"`
			标识  string `json:"id"`
			已确认 bool   `json:"confirmed"`
		}
		if 当前条目 := 协议包.解码参数(请求.参数, &输入); 当前条目 != nil {
			return nil, 当前条目
		}
		结果, 错误 = 当前处理器.服务.测试(执行上下文, 输入.标识, 输入.已确认, 输入.修订号)
	case "mcp.oauth.start", "mcp.oauth.status", "mcp.oauth.cancel", "mcp.disconnect":
		var 输入 struct {
			标识 string `json:"id"`
		}
		if 当前条目 := 协议包.解码参数(请求.参数, &输入); 当前条目 != nil {
			return nil, 当前条目
		}
		switch 请求.方法 {
		case "mcp.oauth.start":
			结果, 错误 = 当前处理器.服务.开始OAuth(执行上下文, 所属对象, 输入.标识)
		case "mcp.oauth.status", "mcp.oauth.cancel":
			结果, 错误 = 当前处理器.服务.OAuth状态(输入.标识, 请求.方法 == "mcp.oauth.cancel")
		case "mcp.disconnect":
			错误 = 当前处理器.服务.断开连接(执行上下文, 输入.标识)
			结果 = true
		}
	default:
		return nil, 协议包.方法未找到()
	}
	if 错误 != nil {
		return nil, 公开错误(错误)
	}
	return 结果, nil
}

func 公开错误(错误 error) *协议包.错误 {
	for _, 项 := range []struct {
		类别 error
		代码 int
	}{
		{设置包.无效错误, -32602}, {设置包.冲突错误, 3010}, {服务包.未找到错误, 3011},
		{服务包.授权错误, 3012}, {服务包.回调错误, 3013}, {服务包.OAuth错误, 3014},
		{服务包.变化通知错误, 3015}, {凭据包.不可用错误, 3016}, {服务包.连接错误, 3017},
		{服务包.客户端注册错误, 3018},
		{服务包.OAuth范围错误, 3019}, {服务包.OAuth范围不匹配错误, 3020},
	} {
		if 错误判断包.是否属于(错误, 项.类别) {
			return &协议包.错误{代码: 项.代码, 消息: 项.类别.错误文本()}
		}
	}
	if 错误判断包.是否属于(错误, 上下文包.超出截止时间错误) {
		return &协议包.错误{代码: 3017, 消息: "MCP 请求超时"}
	}
	return &协议包.错误{代码: -32603, 消息: "MCP 操作失败"}
}
```
