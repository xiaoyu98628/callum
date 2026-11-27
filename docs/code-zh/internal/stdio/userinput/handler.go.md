```go
package 用户输入

import (
	上下文包 "context"
	错误判断包 "errors"
	智能体包 "github.com/xiaoyu98628/callum/internal/agent"
	服务包 "github.com/xiaoyu98628/callum/internal/agent/service"
	协议包 "github.com/xiaoyu98628/callum/internal/stdio/protocol"
)

type 处理器 struct{ 引擎 *服务包.引擎 }

func 新建(引擎 *服务包.引擎) *处理器 { return &处理器{引擎: 引擎} }
func (当前处理器 *处理器) 句柄(执行上下文 上下文包.上下文, 请求 协议包.请求) (any, *协议包.错误) {
	var 值 any
	var 错误 error
	switch 请求.方法 {
	case "user_input.get":
		var 当前对象 struct {
			标识 string `json:"id"`
		}
		if 失败 := 协议包.解码参数(请求.参数, &当前对象); 失败 != nil {
			return nil, 失败
		}
		值, 错误 = 当前处理器.引擎.获取输入(执行上下文, 当前对象.标识)
	case "user_input.list":
		var 当前对象 struct {
			执行标识 string `json:"run_id"`
			状态   string `json:"status"`
			上限   int    `json:"limit"`
			偏移   int    `json:"offset"`
		}
		if 失败 := 协议包.解码参数(请求.参数, &当前对象); 失败 != nil {
			return nil, 失败
		}
		if 当前对象.上限 == 0 {
			当前对象.上限 = 100
		}
		if 当前对象.状态 != "" && 当前对象.状态 != "pending" {
			return nil, 协议包.无效参数()
		}
		值, 错误 = 当前处理器.引擎.输入集(执行上下文, 当前对象.执行标识, 当前对象.状态 == "pending", 当前对象.上限, 当前对象.偏移)
	case "user_input.submit":
		var 当前对象 struct {
			标识   string    `json:"input_id"`
			执行标识 string    `json:"run_id"`
			回答   智能体包.输入回答 `json:"answer"`
		}
		if 失败 := 协议包.解码参数(请求.参数, &当前对象); 失败 != nil {
			return nil, 失败
		}
		值, 错误 = 当前处理器.引擎.提交输入(执行上下文, 当前对象.标识, 当前对象.执行标识, 当前对象.回答)
	default:
		return nil, 协议包.方法未找到()
	}
	if 错误 == nil {
		return 值, nil
	}
	switch {
	case 错误判断包.是否属于(错误, 智能体包.无效错误):
		return nil, 协议包.无效参数()
	case 错误判断包.是否属于(错误, 智能体包.未找到错误):
		return nil, &协议包.错误{代码: 1001, 消息: "待回复问题不存在"}
	case 错误判断包.是否属于(错误, 智能体包.冲突错误):
		return nil, &协议包.错误{代码: 1003, 消息: "问题状态已变化，请重新核对"}
	default:
		return nil, &协议包.错误{代码: -32603, 消息: "用户回复处理失败"}
	}
}
```
